package scheduler

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const SchedulerName = "foo"

type Scheduler struct{}

var _ framework.QueueSortPlugin = &Scheduler{}

func (*Scheduler) Name() string {
	return SchedulerName
}

// NewScheduler initializes a new plugin and returns it.
func NewScheduler(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Scheduler{}, nil
}

func (*Scheduler) Less(*framework.QueuedPodInfo, *framework.QueuedPodInfo) bool {
	return true
}
