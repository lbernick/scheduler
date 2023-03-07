package scheduler_test

import (
	"context"
	"testing"

	"github.com/lbernick/scheduler/pkg/scheduler"
	corev1 "k8s.io/api/core/v1"
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
