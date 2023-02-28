package scheduler_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lbernick/scheduler/pkg/scheduler"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	testClientSet "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
)

type fakeSharedLister struct{}

func (f *fakeSharedLister) StorageInfos() framework.StorageInfoLister {
	return nil
}

func (f *fakeSharedLister) NodeInfos() framework.NodeInfoLister {
	return nil
}

func setUpTestData(ctx context.Context, t *testing.T, pods []*corev1.Pod) *scheduler.Scheduler {
	cs := testClientSet.NewSimpleClientset()

	informerFactory := informers.NewSharedInformerFactory(cs, 0 /* defaultResync */)
	podInformer := informerFactory.Core().V1().Pods()
	for _, pod := range pods {
		podInformer.Informer().GetStore().Add(pod)
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
	s, err := scheduler.NewScheduler(nil, fh)
	if err != nil {
		t.Fatalf("error creating new scheduler: %s", err)
	}
	return s.(*scheduler.Scheduler)
}

func TestNewScheduler(t *testing.T) {
	s := setUpTestData(context.Background(), t, nil)
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
			s := setUpTestData(ctx, t, tc.existingPods)
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
