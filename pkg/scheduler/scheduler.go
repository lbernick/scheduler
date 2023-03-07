package scheduler

import (
	"k8s.io/apimachinery/pkg/runtime"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const SchedulerName = "OneNodePerPipelineRun"

type Scheduler struct {
	podLister corelisters.PodLister
}

func (*Scheduler) Name() string {
	return SchedulerName
}

// NewScheduler initializes a new plugin and returns it.
func NewScheduler(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Scheduler{podLister: handle.SharedInformerFactory().Core().V1().Pods().Lister()}, nil
}
