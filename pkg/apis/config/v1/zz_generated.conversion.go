//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

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
// Code generated by conversion-gen. DO NOT EDIT.

package v1

import (
	config "github.com/lbernick/scheduler/pkg/apis/config"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*OneNodePerPipelineRunArgs)(nil), (*config.OneNodePerPipelineRunArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_OneNodePerPipelineRunArgs_To_config_OneNodePerPipelineRunArgs(a.(*OneNodePerPipelineRunArgs), b.(*config.OneNodePerPipelineRunArgs), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*config.OneNodePerPipelineRunArgs)(nil), (*OneNodePerPipelineRunArgs)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_config_OneNodePerPipelineRunArgs_To_v1_OneNodePerPipelineRunArgs(a.(*config.OneNodePerPipelineRunArgs), b.(*OneNodePerPipelineRunArgs), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1_OneNodePerPipelineRunArgs_To_config_OneNodePerPipelineRunArgs(in *OneNodePerPipelineRunArgs, out *config.OneNodePerPipelineRunArgs, s conversion.Scope) error {
	out.IsolationStrategy = in.IsolationStrategy
	return nil
}

// Convert_v1_OneNodePerPipelineRunArgs_To_config_OneNodePerPipelineRunArgs is an autogenerated conversion function.
func Convert_v1_OneNodePerPipelineRunArgs_To_config_OneNodePerPipelineRunArgs(in *OneNodePerPipelineRunArgs, out *config.OneNodePerPipelineRunArgs, s conversion.Scope) error {
	return autoConvert_v1_OneNodePerPipelineRunArgs_To_config_OneNodePerPipelineRunArgs(in, out, s)
}

func autoConvert_config_OneNodePerPipelineRunArgs_To_v1_OneNodePerPipelineRunArgs(in *config.OneNodePerPipelineRunArgs, out *OneNodePerPipelineRunArgs, s conversion.Scope) error {
	out.IsolationStrategy = in.IsolationStrategy
	return nil
}

// Convert_config_OneNodePerPipelineRunArgs_To_v1_OneNodePerPipelineRunArgs is an autogenerated conversion function.
func Convert_config_OneNodePerPipelineRunArgs_To_v1_OneNodePerPipelineRunArgs(in *config.OneNodePerPipelineRunArgs, out *OneNodePerPipelineRunArgs, s conversion.Scope) error {
	return autoConvert_config_OneNodePerPipelineRunArgs_To_v1_OneNodePerPipelineRunArgs(in, out, s)
}