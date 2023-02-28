This Kubernetes scheduler plugin runs in a deployment alongside the kubernetes "default-scheduler".
It ensures that PipelineRun's TaskRuns' pods are all scheduled to that node,
so that parallel TaskRuns using the same PVC-backed Workspace can actually run in parallel.

## Installation

[Install Tekton Pipelines](https://github.com/tektoncd/pipeline/blob/main/docs/install.md)
and [disable the affinity assistant](https://github.com/tektoncd/pipeline/blob/main/docs/additional-configs.md#customizing-the-pipelines-controller-behavior).

Build and install from source with [ko](https://ko.build/):

```sh
ko apply -f config
```

This scheduler has only been tested on GKE version 1.25.

## How it works

This scheduler implements the PreFilter extension point of the [Kubernetes scheduler framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/)
and is run as a second scheduler (see ["Configuring multiple schedulers"](https://kubernetes.io/docs/tasks/extend-kubernetes/configure-multiple-schedulers)).

The PreFilter extension point determines what nodes the pod could be placed on by selecting pods with the same value for the label
"tekton.dev/pipelineRun" and considering only the node those pods are running on (if any).

## Example usage

List existing nodes:

```sh
$ kubectl get node
NAME                                       STATUS   ROLES    AGE   VERSION
gke-scheduler-default-pool-906ef80f-xsg1   Ready    <none>   10d   v1.25.6-gke.1000
gke-scheduler-default-pool-ac08710a-04wh   Ready    <none>   10d   v1.25.6-gke.1000
gke-scheduler-default-pool-e7e66a24-c7rf   Ready    <none>   10d   v1.25.6-gke.1000
```

Create PipelineRun:

```sh
$ kubectl create -f examples/pipelinerun.yaml
pipelinerun.tekton.dev/sample-pipelinerun-phbpb created
```

Get its pods and their nodes:
```sh
$ PIPELINERUN_NAME=sample-pipelinerun-phbpb
$ kubectl get pods -l tekton.dev/pipelineRun=$PIPELINERUN_NAME -o=custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
NAME                                      NODE
sample-pipelinerun-phbpb-first-pod        gke-scheduler-default-pool-e7e66a24-c7rf
sample-pipelinerun-phbpb-last-pod         gke-scheduler-default-pool-e7e66a24-c7rf
sample-pipelinerun-phbpb-parallel-1-pod   gke-scheduler-default-pool-e7e66a24-c7rf
sample-pipelinerun-phbpb-parallel-2-pod   gke-scheduler-default-pool-e7e66a24-c7rf
```

Get events (including scheduler events) for one of the involved pods:
```sh
$ POD_NAME=sample-pipelinerun-phbpb-parallel-1-pod
$ kubectl get events -n default --field-selector involvedObject.name=$POD_NAME
LAST SEEN   TYPE     REASON      OBJECT                                        MESSAGE
88s         Normal   Scheduled   pod/sample-pipelinerun-phbpb-parallel-1-pod   Successfully assigned default/sample-pipelinerun-phbpb-parallel-1-pod to gke-scheduler-default-pool-e7e66a24-c7rf
...
```
