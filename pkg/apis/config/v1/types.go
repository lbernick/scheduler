package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OneNodePerPipelineRunArgs holds configuration for the OneNodePerPipelineRun plugin
type OneNodePerPipelineRunArgs struct {
	metav1.TypeMeta
	IsolationStrategy string `json:"isolationStrategy,omitempty"`
}
