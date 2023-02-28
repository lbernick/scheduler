package scheduler

import (
	"context"
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const SchedulerName = "OneNodePerPipelineRun"

type Scheduler struct {
	podLister corelisters.PodLister
}

var _ framework.PreFilterPlugin = &Scheduler{}

func (*Scheduler) Name() string {
	return SchedulerName
}

// NewScheduler initializes a new plugin and returns it.
func NewScheduler(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Scheduler{podLister: handle.SharedInformerFactory().Core().V1().Pods().Lister()}, nil
}

// PreFilter determines if there are any nodes running pods associated with the same PipelineRun
// as the pod to be scheduled. (There should be at most one node.) If so, returns this node.
func (s *Scheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	prName, ok := pod.ObjectMeta.Labels[pipeline.PipelineRunLabelKey]
	klog.V(3).Infof("prefilter pod %s associated with PipelineRun %s", pod.Name, prName)
	if !ok {
		return nil, framework.NewStatus(framework.Success, fmt.Sprintf("pod %s not associated with a PipelineRun", pod.Name))
	}

	selector := labels.SelectorFromSet(map[string]string{pipeline.PipelineRunLabelKey: prName})
	pods, err := s.podLister.List(selector)
	if err != nil {
		// TODO: This error would likely be transient. Not clear what PreFilter should return since this should likely be retried.
		return nil, framework.NewStatus(framework.Success, fmt.Sprintf("error listing pods for PipelineRun %s", prName))
	}
	if pods == nil {
		klog.V(3).Infof("no existing pods associated with PipelineRun %s for pod %s", prName, pod.Name)
		return nil, framework.NewStatus(framework.Success, fmt.Sprintf("no existing pods related to PipelineRun %s for pod %s", prName, pod.Name))
	}

	existingNodes := sets.String{}
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			existingNodes.Insert(pod.Spec.NodeName)
		}
	}

	if len(existingNodes) > 1 {
		klog.V(1).Infof("pods associated with PipelineRun %s found on multiple nodes %s", prName, existingNodes)
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("pods for PipelineRun %s found on multiple nodes", prName))
	}
	if len(existingNodes) == 0 {
		klog.V(3).Infof("no scheduled pods associated with PipelineRun %s for pod %s", prName, pod.Name)
		return nil, framework.NewStatus(framework.Success, fmt.Sprintf("no scheduled pods related to PipelineRun %s for pod %s", prName, pod.Name))
	}

	klog.V(3).Infof("scheduled pods associated with PipelineRun %s for pod %s on node %s", prName, pod.Name, existingNodes)
	return &framework.PreFilterResult{NodeNames: existingNodes}, framework.NewStatus(framework.Success, fmt.Sprintf("found scheduled pods related to PipelineRun %s for pod %s", prName, pod.Name))
}

// PreFilterExtensions is needed to implement the PreFilterPlugin interface but is not used
func (s *Scheduler) PreFilterExtensions() framework.PreFilterExtensions { return s }

func (*Scheduler) AddPod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, podToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	return framework.NewStatus(framework.Success, "")
}

func (*Scheduler) RemovePod(ctx context.Context, cycleState *framework.CycleState, podToSchedule *corev1.Pod, podToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	return framework.NewStatus(framework.Success, "")
}
