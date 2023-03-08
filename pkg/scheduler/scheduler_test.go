package scheduler_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lbernick/scheduler/pkg/apis/config"
	"github.com/lbernick/scheduler/pkg/scheduler"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	testClientSet "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	fakeframework "k8s.io/kubernetes/pkg/scheduler/framework/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
)

type fakeSharedLister struct {
	nodes []*framework.NodeInfo
}

var _ framework.SharedLister = &fakeSharedLister{}

func (f *fakeSharedLister) StorageInfos() framework.StorageInfoLister {
	return nil
}

func (f *fakeSharedLister) NodeInfos() framework.NodeInfoLister {
	return fakeframework.NodeInfoLister(f.nodes)
}

func setUpTestData(ctx context.Context, t *testing.T, args config.OneNodePerPipelineRunArgs, pods []*corev1.Pod) *scheduler.Scheduler {
	cs := testClientSet.NewSimpleClientset()

	informerFactory := informers.NewSharedInformerFactory(cs, 0 /* defaultResync */)
	podInformer := informerFactory.Core().V1().Pods()
	for _, pod := range pods {
		podInformer.Informer().GetStore().Add(pod)
		if _, err := cs.CoreV1().Pods("").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Failed to create pod %q: %v", pod.Name, err)
		}
	}

	// Use the default plugins for queue sorting and binding,
	// since scheduler.Scheduler does not handle that functionality
	registeredPlugins := []st.RegisterPluginFunc{
		st.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		st.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	fakeSharedLister := &fakeSharedLister{}

	fh, err := st.NewFramework(
		registeredPlugins,
		"one-node-per-pipelineRun", // profileName
		wait.NeverStop,
		runtime.WithClientSet(cs),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithSnapshotSharedLister(fakeSharedLister))

	if err != nil {
		t.Fatalf("error creating new framework handle: %s", err)
	}
	s, err := scheduler.NewScheduler(&args, fh)
	if err != nil {
		t.Fatalf("error creating new scheduler: %s", err)
	}
	return s.(*scheduler.Scheduler)
}

func makeNodeInfo(name string, pods []*corev1.Pod) *framework.NodeInfo {
	ni := framework.NewNodeInfo()
	ni.SetNode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}})
	for _, pod := range pods {
		ni.Pods = append(ni.Pods, &framework.PodInfo{Pod: pod})
	}
	return ni
}

func TestNewSchedulerValidConfig(t *testing.T) {
	s := setUpTestData(context.Background(), t, config.OneNodePerPipelineRunArgs{IsolationStrategy: config.StrategyIsolateAlways}, nil)
	if s.Name() != scheduler.SchedulerName {
		t.Errorf("wrong name: %s", s.Name())
	}
}

func TestPreFilter(t *testing.T) {
	tcs := []struct {
		name          string
		podToSchedule *corev1.Pod
		existingPods  []*corev1.Pod
		wantResult    *framework.PreFilterResult
		wantCode      framework.Code
	}{{
		name: "pod not related to a PipelineRun",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"foo": "bar"},
		}},
		wantResult: nil,
		wantCode:   framework.Success,
	}, {
		name: "no existing pods for any PR",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		wantResult: nil,
		wantCode:   framework.Success,
	}, {
		name: "existing pods for different PR",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		existingPods: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"},
			}, Spec: corev1.PodSpec{NodeName: "node-1"}},
		},
		wantResult: nil,
		wantCode:   framework.Success,
	}, {
		name: "existing pods for same PR, not yet scheduled",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		existingPods: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-for-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
			}},
		},
		wantResult: nil,
		wantCode:   framework.Success,
	}, {
		name: "existing pods for same PR, all scheduled to same node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		existingPods: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-for-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
			}, Spec: corev1.PodSpec{NodeName: "node-1"},
		}, {
			ObjectMeta: metav1.ObjectMeta{Name: "another-pod-for-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
			}, Spec: corev1.PodSpec{NodeName: "node-1"}},
		},
		wantResult: &framework.PreFilterResult{NodeNames: sets.NewString("node-1")},
		wantCode:   framework.Success,
	}, {
		name: "existing pods for same PR, scheduled to different nodes",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		existingPods: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-for-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
			}, Spec: corev1.PodSpec{NodeName: "node-1"},
		}, {
			ObjectMeta: metav1.ObjectMeta{Name: "another-pod-for-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
			}, Spec: corev1.PodSpec{NodeName: "node-2"}},
		},
		wantResult: nil,
		wantCode:   framework.Error,
	}}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := setUpTestData(ctx, t, config.OneNodePerPipelineRunArgs{}, tc.existingPods)
			gotResult, gotStatus := s.PreFilter(ctx, framework.NewCycleState(), tc.podToSchedule)
			if d := cmp.Diff(tc.wantResult, gotResult); d != "" {
				t.Errorf("wrong result: %s", d)
			}
			if d := cmp.Diff(tc.wantCode, gotStatus.Code()); d != "" {
				t.Errorf("wrong status code: %s", d)
			}
		})
	}
}

func TestFilterIsolateAlways(t *testing.T) {
	tcs := []struct {
		name                string
		podToSchedule       *corev1.Pod
		runningPodsOnNode   []*corev1.Pod
		completedPodsOnNode []*corev1.Pod
		wantCode            framework.Code
	}{{
		name: "no pods running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: nil,
		wantCode:          framework.Success,
	}, {
		name: "no pods for other PipelineRuns on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode:   []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "random-other-pod"}}},
		completedPodsOnNode: []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "random-other-pod-2"}}},
		wantCode:            framework.Success,
	}, {
		name: "pod from same PipelineRun running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-from-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod from same PipelineRun completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-from-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod for other PipelineRun running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "another-pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Unschedulable,
	}, {
		name: "pod for other PipelineRun completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "another-pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Unschedulable,
	}, {
		name: "pods from multiple other PipelineRuns running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Error,
	}, {
		name: "pods from multiple other PipelineRuns completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Error,
	}, {
		name: "pods from multiple other PipelineRuns running and completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Error,
	}, {
		name: "pod to schedule not associated with a PipelineRun",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"foo": "bar"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr-2",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Success,
	}}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			args := config.OneNodePerPipelineRunArgs{IsolationStrategy: config.StrategyIsolateAlways}
			nodeName := "node-1"
			var allPods []*corev1.Pod
			for _, pod := range tc.runningPodsOnNode {
				pod.Spec.NodeName = nodeName
				allPods = append(allPods, pod)
			}
			for _, pod := range tc.completedPodsOnNode {
				pod.Spec.NodeName = nodeName
				allPods = append(allPods, pod)
			}
			s := setUpTestData(ctx, t, args, allPods)
			node := makeNodeInfo(nodeName, tc.runningPodsOnNode)
			gotStatus := s.Filter(ctx, framework.NewCycleState(), tc.podToSchedule, node)
			if d := cmp.Diff(tc.wantCode, gotStatus.Code()); d != "" {
				t.Errorf("wrong status code: %s", d)
			}
		})
	}
}

func TestFilterColocateCompleted(t *testing.T) {
	tcs := []struct {
		name                string
		podToSchedule       *corev1.Pod
		runningPodsOnNode   []*corev1.Pod
		completedPodsOnNode []*corev1.Pod
		wantCode            framework.Code
	}{{
		name: "no pods running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: nil,
		wantCode:          framework.Success,
	}, {
		name: "no pods for other PipelineRuns on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode:   []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "random-other-pod"}}},
		completedPodsOnNode: []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "random-other-pod-2"}}},
		wantCode:            framework.Success,
	}, {
		name: "pod from same PipelineRun running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-from-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod from same PipelineRun completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-from-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod for other PipelineRun running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "another-pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Unschedulable,
	}, {
		name: "pod for other PipelineRun completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "another-pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pods from multiple other PipelineRuns running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Error,
	}, {
		name: "pods from multiple other PipelineRuns completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pods from multiple other PipelineRuns running and completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Unschedulable,
	}, {
		name: "pod to schedule not associated with a PipelineRun",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"foo": "bar"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr-2",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Success,
	}}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			args := config.OneNodePerPipelineRunArgs{IsolationStrategy: config.StrategyColocateCompleted}
			nodeName := "node-1"
			var allPods []*corev1.Pod
			for _, pod := range tc.runningPodsOnNode {
				pod.Spec.NodeName = nodeName
				allPods = append(allPods, pod)
			}
			for _, pod := range tc.completedPodsOnNode {
				pod.Spec.NodeName = nodeName
				allPods = append(allPods, pod)
			}
			s := setUpTestData(ctx, t, args, allPods)
			node := makeNodeInfo(nodeName, tc.runningPodsOnNode)
			gotStatus := s.Filter(ctx, framework.NewCycleState(), tc.podToSchedule, node)
			if d := cmp.Diff(tc.wantCode, gotStatus.Code()); d != "" {
				t.Errorf("wrong status code: %s", d)
			}
		})
	}
}

func TestFilterColocateAlways(t *testing.T) {
	tcs := []struct {
		name                string
		podToSchedule       *corev1.Pod
		runningPodsOnNode   []*corev1.Pod
		completedPodsOnNode []*corev1.Pod
		wantCode            framework.Code
	}{{
		name: "no pods running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: nil,
		wantCode:          framework.Success,
	}, {
		name: "no pods for other PipelineRuns on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode:   []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "random-other-pod"}}},
		completedPodsOnNode: []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "random-other-pod-2"}}},
		wantCode:            framework.Success,
	}, {
		name: "pod from same PipelineRun running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-from-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod from same PipelineRun completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-from-same-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod for other PipelineRun running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "another-pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod for other PipelineRun completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "another-pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pods from multiple other PipelineRuns running on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pods from multiple other PipelineRuns completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pods from multiple other PipelineRuns running and completed on node",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-1"},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-different-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-3"}},
		}},
		wantCode: framework.Success,
	}, {
		name: "pod to schedule not associated with a PipelineRun",
		podToSchedule: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"foo": "bar"},
		}},
		runningPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		completedPodsOnNode: []*corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pod-for-other-pr-2",
				Labels: map[string]string{pipeline.PipelineRunLabelKey: "pr-2"}},
		}},
		wantCode: framework.Success,
	}}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			args := config.OneNodePerPipelineRunArgs{IsolationStrategy: config.StrategyColocateAlways}
			nodeName := "node-1"
			var allPods []*corev1.Pod
			for _, pod := range tc.runningPodsOnNode {
				pod.Spec.NodeName = nodeName
				allPods = append(allPods, pod)
			}
			for _, pod := range tc.completedPodsOnNode {
				pod.Spec.NodeName = nodeName
				allPods = append(allPods, pod)
			}
			s := setUpTestData(ctx, t, args, allPods)
			node := makeNodeInfo(nodeName, tc.runningPodsOnNode)
			gotStatus := s.Filter(ctx, framework.NewCycleState(), tc.podToSchedule, node)
			if d := cmp.Diff(tc.wantCode, gotStatus.Code()); d != "" {
				t.Errorf("wrong status code: %s", d)
			}
		})
	}
}
