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

package main

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/options"
	"k8s.io/test-infra/prow/sidecar"
	"os"
)

func main() {
	o := sidecar.NewOptions()

	if err := options.Load(o); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "msidecar"}),
	)

	if err := o.Run(); err != nil {
		logrus.WithError(err).Fatal("Failed to initialize job")
	}

	var err error
	var kc *kube.Client

	kc, err = kube.NewClientInCluster("pods")
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kubernetes client.")
	}

	jobSpecEnv, _ := os.LookupEnv("JOB_SPEC")
	if jobSpecEnv != "" {
		jobSpec := downwardapi.JobSpec{}
		json.Unmarshal([]byte(jobSpecEnv), &jobSpec)
		err := kc.DeletePod(fmt.Sprintf("pod-%s", jobSpec.ProwJobID))
		if err != nil {
			logrus.WithError(err)
		} else {
			logrus.Infof("Deleting the pod named pod-%s", jobSpec.ProwJobID)
		}
	}
}
