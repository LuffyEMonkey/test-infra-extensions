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

package sidecar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	"k8s.io/test-infra/prow/pod-utils/gcs"
	"path"
)

// Run will watch for the process being wrapped to exit
// and then post the status of that process and any artifacts
// to cloud storage.
func (o Options) Run() error {
	spec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		return fmt.Errorf("could not resolve job spec: %v", err)
	}

	// If we are being asked to terminate by the kubelet but we have
	// NOT seen the test process exit cleanly, we need a to start
	// uploading artifacts to GCS immediately. If we notice the process
	// exit while doing this best-effort upload, we can race with the
	// second upload but we can tolerate this as we'd rather get SOME
	// data into GCS than attempt to cancel these uploads and get none.
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case s := <-interrupt:
			logrus.Errorf("Received an interrupt: %s", s)
			o.doUpload(spec, false, true)
		}
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("could not begin fsnotify watch: %v", err)
	}
	defer watcher.Close()

	ticker := time.NewTicker(30 * time.Second)
	group := sync.WaitGroup{}
	group.Add(o.NodeCount)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if isMarkerFile(event.Name) && event.Op&fsnotify.Create == fsnotify.Create {
					group.Done()
				}
			case err := <-watcher.Errors:
				logrus.WithError(err).Info("Encountered an error during fsnotify watch")
			case <-ticker.C:
				if o.isCompleted() {
					for i := 0; i < o.NodeCount; i++ {
						group.Done()
					}
				}
			}
		}
	}()

	if err := watcher.Add(o.BaseDir); err != nil {
		return fmt.Errorf("could not add to fsnotify watch: %v", err)
	}
	group.Wait()
	ticker.Stop()

	// If we are being asked to terminate by the kubelet but we have
	// seen the test process exit cleanly, we need a chance to upload
	// artifacts to GCS. The only valid way for this program to exit
	// after a SIGINT or SIGTERM in this situation is to finish]
	// uploading, so we ignore the signals.
	signal.Ignore(os.Interrupt, syscall.SIGTERM)

	passed, aborted := o.returnCode()
	return o.doUpload(spec, passed, aborted)
}

func (o Options) isCompleted() bool {
	completed := 0
	filepath.Walk(o.BaseDir, func(fspath string, info os.FileInfo, err error) error {
		if info.IsDir() && fspath != o.BaseDir {
			return filepath.SkipDir
		}

		if isMarkerFile(fspath) {
			completed += 1
		}

		return nil
	})

	return completed == o.NodeCount
}

func (o Options) returnCode() (bool, bool) {
	passed := true
	aborted := false

	filepath.Walk(o.BaseDir, func(fspath string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return filepath.SkipDir
		}

		if isMarkerFile(fspath) {
			returnCodeData, err := ioutil.ReadFile(fspath)
			if err != nil {
				logrus.WithError(err).Warn("Could not read return code from marker file")
			} else {
				returnCode, err := strconv.Atoi(strings.TrimSpace(string(returnCodeData)))
				if err != nil {
					logrus.WithError(err).Warn("Failed to parse process return code")
				}

				if returnCode != 0 || err != nil {
					passed = false
				}

				if returnCode == 130 {
					aborted = true
				}
			}
		}
		return nil
	})

	return passed, aborted
}

func isMarkerFile(fullname string) bool {
	filename := path.Base(fullname)
	return strings.HasPrefix(filename, "marker-file-")
}

func isProcessLog(fullname string) bool {
	filename := path.Base(fullname)
	return strings.HasPrefix(filename, "process-log-")
}

func nodeIndex(fullname string) (int, error) {
	filename := path.Base(fullname)
	endIndex := len(filename) - 4
	return strconv.Atoi(filename[12:endIndex])
}

func (o Options) doUpload(spec *downwardapi.JobSpec, passed, aborted bool) error {
	uploadTargets := map[string]gcs.UploadFunc{}
	filepath.Walk(o.BaseDir, func(fspath string, info os.FileInfo, err error) error {
		if isProcessLog(fspath) {
			if index, err := nodeIndex(fspath); err == nil {
				uploadTargets[fmt.Sprintf("process-logs/node-%d.txt", index)] = gcs.FileUpload(fspath)
			}
		}
		return nil
	})

	var result string
	switch {
	case passed:
		result = "SUCCESS"
	case aborted:
		result = "ABORTED"
	default:
		result = "FAILURE"
	}

	finished := struct {
		Timestamp int64  `json:"timestamp"`
		Passed    bool   `json:"passed"`
		Result    string `json:"result"`
	}{
		Timestamp: time.Now().Unix(),
		Passed:    passed,
		Result:    result,
	}
	finishedData, err := json.Marshal(&finished)
	if err != nil {
		logrus.WithError(err).Warn("Could not marshal finishing data")
	} else {
		uploadTargets["node-finished.json"] = gcs.DataUpload(bytes.NewBuffer(finishedData))
	}

	if err := o.GcsOptions.Run(spec, uploadTargets); err != nil {
		return fmt.Errorf("failed to upload to GCS: %v", err)
	}

	return nil
}
