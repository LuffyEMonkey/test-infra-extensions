/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pod_launcher

import (
	"boscoin.io/test-infra-extensions/pkg/node-sidecar"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
	"os"
	"path"
	"sort"
	"strings"
	"text/template"
	"time"
)

const (
	logMountName            = "logs"
	logMountPath            = "/logs"
	artifactsPath           = logMountPath + "/artifacts"
	toolsMountName          = "tools"
	toolsMountPath          = "/tools"
	gcsCredentialsMountName = "gcs-credentials"
	gcsCredentialsMountPath = "/secrets/gcs"
)

var terminationGracePeriodSeconds = int64(1200)

func (o *Options) Run() error {
	var err error
	var kc *kube.Client

	kc, err = kube.NewClientInCluster("pods")
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kubernetes client.")
	}

	var dockerImage string
	logPath, _ := os.LookupEnv("LOG_MOUNT_PATH")
	clusterSpec, _ := os.LookupEnv("CLUSTER_SPEC")
	clusterInfo, _ := os.LookupEnv("CLUSTER_INFO")
	jobId, _ := os.LookupEnv("PROW_JOB_ID")
	jobSpec, _ := os.LookupEnv(downwardapi.JobSpecEnv)
	gcsCredentialsName, _ := os.LookupEnv("GCS_CREDENTIALS_NAME")

	if buf, err := ioutil.ReadFile(fmt.Sprintf("%s/docker_image", logPath)); err == nil {
		dockerImage = strings.Trim(string(buf), " \n")
	} else {
		logrus.WithError(err).Fatal("Error opening file")
	}

	buf := bytes.Buffer{}
	template.Must(template.New("clusterSpec").Parse(clusterSpec)).Execute(&buf, map[string]string{
		"Host":           fmt.Sprintf("pod-%s", jobId),
		"ContainerImage": dockerImage,
		"DataDir":        logMountPath,
	})
	sebak := buf.String()

	if sebak != "" && jobId != "" {
		sebakSpec := &SebakJob{}
		if err := json.Unmarshal([]byte(sebak), sebakSpec); err != nil {
			fmt.Println(fmt.Errorf("malformed $%s: %v", sebak, err))
		}

		podLabels := make(map[string]string)
		spec := *sebakSpec.PodSpec.DeepCopy()

		logMount := v1.VolumeMount{
			Name:      logMountName,
			MountPath: logMountPath,
		}
		logVolume := kube.Volume{
			Name: logMountName,
			VolumeSource: kube.VolumeSource{
				EmptyDir: &kube.EmptyDirVolumeSource{},
			},
		}
		toolsMount := v1.VolumeMount{
			Name:      toolsMountName,
			MountPath: toolsMountPath,
		}
		toolsVolume := kube.Volume{
			Name: toolsMountName,
			VolumeSource: kube.VolumeSource{
				EmptyDir: &kube.EmptyDirVolumeSource{},
			},
		}
		gcsCredentialsMount := v1.VolumeMount{
			Name:      gcsCredentialsMountName,
			MountPath: gcsCredentialsMountPath,
		}
		gcsCredentialsVolume := kube.Volume{
			Name: gcsCredentialsMountName,
			VolumeSource: kube.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: gcsCredentialsName,
				},
			},
		}

		entrypointLocation := fmt.Sprintf("%s/entrypoint", toolsMountPath)

		spec.InitContainers = append(spec.InitContainers, kube.Container{
			Name:         "place-tools",
			Image:        "registry.svc.ci.openshift.org/ci/entrypoint:latest",
			Command:      []string{"/bin/cp"},
			Args:         []string{"/entrypoint", entrypointLocation},
			VolumeMounts: []kube.VolumeMount{toolsMount},
		})

		for i, _ := range spec.Containers {
			container := spec.Containers[i]
			environments := make(map[string]string)
			if err != nil {
				return fmt.Errorf("could not encode entrypoint configuration as JSON: %v", err)
			}

			if env, err := entrypointConfigEnv(&spec, &container, i); err == nil {
				environments[entrypoint.JSONConfigEnvVar] = env
			} else {
				return err
			}

			container.Command = []string{entrypointLocation}
			container.Args = []string{}
			container.Env = append(container.Env, kubeEnv(environments)...)
			container.VolumeMounts = append(container.VolumeMounts, logMount, toolsMount)
			spec.Containers[i] = container
		}
		spec.TerminationGracePeriodSeconds = &terminationGracePeriodSeconds
		spec.RestartPolicy = v1.RestartPolicyNever
		spec.Volumes = append(spec.Volumes, logVolume, toolsVolume, gcsCredentialsVolume)

		gcsOptions := gcsupload.Options{
			GCSConfiguration: &kube.GCSConfiguration{
				Bucket:       "bos-e2e-test",
				PathStrategy: "legacy",
				DefaultOrg:   "owlchain",
				DefaultRepo:  "sebak",
			},
			GcsCredentialsFile: fmt.Sprintf("%s/service-account.json", gcsCredentialsMountPath),
			DryRun:             false,
		}

		sidecarEnv, _ := json.Marshal(&sidecar.Options{
			GcsOptions: &gcsOptions,
			NodeCount:  len(spec.Containers),
			BaseDir:    logMountPath,
		})

		spec.Containers = append(spec.Containers, kube.Container{
			Name:    "node-sidecar",
			Image:   "gcr.io/devenv-205606/node-sidecar:latest",
			Command: []string{"/node-sidecar"},
			Args:    []string{},
			Env: kubeEnv(map[string]string{
				downwardapi.JobSpecEnv:   jobSpec,
				sidecar.JSONConfigEnvVar: string(sidecarEnv),
			}),
			VolumeMounts: []kube.VolumeMount{logMount, gcsCredentialsMount},
		})

		podName := fmt.Sprintf("pod-%s", jobId)
		pod, err := kc.CreatePod(v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: podLabels,
			},
			Spec: spec,
		})

		var logData []byte
		if buf, err := json.MarshalIndent(pod, "", "\t"); err == nil {
			logData = buf
		}

		if err != nil {
			logData = append(logData, '\n', '\n')
			logData = append(logData, err.Error()...)
			logrus.WithError(err)
		} else {
			logrus.Infof("Create a pod named %s", pod.Name)
			if podIp, err := waitReady(kc, podName); err == nil {
				buf := bytes.Buffer{}
				template.Must(template.New("clusterInfo").Parse(clusterInfo)).Execute(&buf, map[string]string{
					"Host": podIp,
				})

				logData = append(logData, '\n', '\n')
				logData = append(logData, buf.Bytes()...)

				ioutil.WriteFile(fmt.Sprintf("%s/cluster_info.json", logMountPath), buf.Bytes(), 0755)
			}
		}

		logfile := fmt.Sprintf("%s/initials/pod-launcher.txt", logMountPath)
		os.MkdirAll(path.Dir(logfile), 0755)
		if err := ioutil.WriteFile(logfile, logData, 0755); err != nil {
			return fmt.Errorf("failed to write clone records: %v", err)
		}

		return nil
	}
	return nil
}

func waitReady(kc *kube.Client, podName string) (string, error) {
	retries := 5
	backOff := time.Second * 3
	timer := time.NewTimer(backOff)

	var podIp string
loop:
	for {
		select {
		case <-timer.C:
			retries -= 1
			pod, err := kc.GetPod(podName)

			if err == nil && pod.Status.Phase == kube.PodRunning {
				ready := true
				for _, status := range pod.Status.ContainerStatuses {
					logrus.Infof("container status %s : %v", status.Name, status.Ready)
					if !status.Ready {
						ready = false
						break
					}
				}

				if ready || retries < 1 {
					podIp = pod.Status.PodIP
					break loop
				}
			}

			backOff *= 2
			timer.Reset(backOff)

			logrus.Infof("Waiting until Pod is ready. Will check the status after %d seconds.", backOff)
		}
	}

	if podIp != "" {
		return podIp, nil
	} else {
		return "", errors.New("Unreachable host")
	}
}

func entrypointConfigEnv(spec *v1.PodSpec, container *v1.Container, i int) (string, error) {
	timeout := 7200000000000
	gracePeriod := 15000000000

	wrapperOptions := wrapper.Options{
		ProcessLog: fmt.Sprintf("%s/process-log-%d.txt", logMountPath, i),
		MarkerFile: fmt.Sprintf("%s/marker-file-%d.txt", logMountPath, i),
	}
	env, err := entrypoint.Encode(entrypoint.Options{
		Args:        append(spec.Containers[i].Command, spec.Containers[i].Args...),
		Options:     &wrapperOptions,
		Timeout:     time.Duration(timeout),
		GracePeriod: time.Duration(gracePeriod),
		ArtifactDir: fmt.Sprintf("%s/node-%d", artifactsPath, i),
	})
	return env, err
}

func kubeEnv(environment map[string]string) []v1.EnvVar {
	var keys []string
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var kubeEnvironment []v1.EnvVar
	for _, key := range keys {
		kubeEnvironment = append(kubeEnvironment, v1.EnvVar{
			Name:  key,
			Value: environment[key],
		})
	}

	return kubeEnvironment
}
