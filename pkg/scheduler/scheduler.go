package scheduler

import (
	"context"
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const SchedulerName = "OneNodePerPipelineRun"

type Scheduler struct {
	podLister corelisters.PodLister
	podClient v1.PodInterface
}

var _ framework.PreFilterPlugin = &Scheduler{}
var _ framework.FilterPlugin = &Scheduler{}

func (*Scheduler) Name() string {
	return SchedulerName
}

// TODO: See if there's a way to determine expected resource usage of the PipelineRun
// and score or filter nodes based on which are expected to fit a PipelineRun

// NewScheduler initializes a new plugin and returns it.
func NewScheduler(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Scheduler{
		podLister: handle.SharedInformerFactory().Core().V1().Pods().Lister(),
		podClient: handle.ClientSet().CoreV1().Pods(""),
	}, nil
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

// Filter filters out nodes that already have pods from other PipelineRuns on them.
// If there are other pods from the same PipelineRun running on a node, filters out all nodes but that one.
//
// Fails if there are no available nodes. In this case, the cluster autoscaler is expected to create a new node
// for the pods that cannot go on available nodes.
//
// This scheduler plugin should be used with an architecture where PipelineRuns are deleted a short time after they
// finish executing. (Otherwise, the number of nodes will only continue to grow.)
// TODO: Should the node be deleted once the PipelineRun is deleted from it?
// (This is probably required in order to meet SLSA L3 build isolation requirements.)
// Or can new PipelineRuns be scheduled onto the node where previous PipelineRuns ran but were deleted?
//
// TODO: How to handle the situation where there are pods from different PipelineRuns in the queue,
// and both may be assigned before one is bound?
// Use app group strategy or similar: https://kccnceu2022.sched.com/event/ytlA/network-aware-scheduling-in-kubernetes-jose-santos-ghent-university
func (s *Scheduler) Filter(
	ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	prName, ok := pod.ObjectMeta.Labels[pipeline.PipelineRunLabelKey]
	klog.V(3).Infof("filter pod %s associated with PipelineRun %s", pod.Name, prName)
	if !ok {
		// Pod is not related to a PipelineRun, so it can be scheduled on this node.
		return framework.NewStatus(framework.Success, fmt.Sprintf("pod %s not associated with a PipelineRun", pod.Name))
	}

	existingPRNames := sets.Set[string]{}
	nodeName := nodeInfo.Node().Name

	// List all the pods associated with the node, rather than using nodeInfo.Pods,
	// since nodeInfo.Pods does not contain completed pods.
	// TODO: This probably has the same scaling problems as pod anti-affinity-- explore other options.
	pods, err := s.podClient.List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})

	if err != nil {
		return framework.NewStatus(framework.Success, fmt.Sprintf("error listing pods for node %s", nodeName))
	}

	for _, pod := range pods.Items {
		prNameForPod, ok := pod.ObjectMeta.Labels[pipeline.PipelineRunLabelKey]
		if ok {
			existingPRNames.Insert(prNameForPod)
		}
	}

	if len(existingPRNames) > 1 {
		// This node has multiple PipelineRuns' pods on it
		// TODO: How best to handle this error case?
		// Should we fail to schedule any more pods for these PipelineRuns?
		klog.V(1).Infof("found pods associated with multiple PipelineRuns %s", existingPRNames)
		return framework.NewStatus(framework.Error, fmt.Sprintf("found pods for multiple PipelineRuns running on the same node: %s", existingPRNames.UnsortedList()))
	}

	if existingPRNames.Len() == 0 {
		klog.V(3).Infof("no pods associated with PipelineRun %s on node", prName)
		return framework.NewStatus(framework.Success, "no pods associated with a PipelineRun running on node")
	}

	if existingPRNames.Has(prName) {
		klog.V(3).Infof("existing pods associated with PipelineRun %s on node", prName)
		return framework.NewStatus(framework.Success, fmt.Sprintf("can schedule pod %s onto node with pods from same PipelineRun %s", pod.Name, prName))
	}
	existingPr := existingPRNames.UnsortedList()[0]
	klog.V(3).Infof("existing pods associated with different PipelineRun %s on node", existingPr)
	// Unschedulable = cannot schedule, but that might change with preemption
	// UnschedulableAndUnresolvable would also work here
	return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("node is already running PipelineRun %s but pod is associated with PipelineRun %s", existingPr, prName))
}
