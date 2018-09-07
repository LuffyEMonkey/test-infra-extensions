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
	"flag"
	"k8s.io/api/core/v1"
)

type Options struct {
	configPath    string
	jobConfigPath string
}

func (o *Options) ConfigVar() string {
	return ""
}

func (o *Options) LoadConfig(config string) error {
	return nil
}

func (o *Options) BindOptions(flags *flag.FlagSet) {
}

func (o *Options) Complete(args []string) {
}

func (o *Options) Validate() error {
	return nil
}

type SebakJob struct {
	PodSpec *v1.PodSpec `json:"spec,omitempty"`
}
