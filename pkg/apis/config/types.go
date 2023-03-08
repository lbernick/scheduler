package config

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OneNodePerPipelineRunArgs holds configuration for the OneNodePerPipelineRun plugin
type OneNodePerPipelineRunArgs struct {
	metav1.TypeMeta
	IsolationStrategy string `json:"isolationStrategy,omitempty"`
}

const (
	StrategyColocateAlways    = "colocateAlways"
	StrategyColocateCompleted = "colocateCompleted"
	StrategyIsolateAlways     = "isolateAlways"
)

var ValidIsolationStrategies = sets.NewString(StrategyColocateAlways, StrategyColocateCompleted, StrategyIsolateAlways)

func (args OneNodePerPipelineRunArgs) Validate() error {
	if ValidIsolationStrategies.Has(args.IsolationStrategy) {
		return nil
	}
	return fmt.Errorf("invalid isolationStrategy %s", args.IsolationStrategy)
}

func (args *OneNodePerPipelineRunArgs) SetDefaults() {
	if args.IsolationStrategy == "" {
		args.IsolationStrategy = StrategyColocateAlways
	}
}
