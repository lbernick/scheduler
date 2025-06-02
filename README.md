# Overview

This project has an [accompanying blog post](https://www.leebernick.com/posts/scheduler-plugin/) with more info on what it does and why I built it.

This Kubernetes scheduler plugin runs in a deployment alongside the kubernetes "default-scheduler".
It ensures that PipelineRun's TaskRuns' pods are all scheduled to that node,
so that parallel TaskRuns using the same PVC-backed Workspace can actually run in parallel.

It can also optionally ensure that no other PipelineRuns may run on the same node (see [Configuration](#configuration) for more info).
This improves the security boundary between PipelineRuns, but has the drawback of poor bin-packing,
since a build TaskRun will likely need much more resources than other TaskRuns in the PipelineRun.

## Installation

[Install Tekton Pipelines](https://github.com/tektoncd/pipeline/blob/main/docs/install.md)
and [disable the affinity assistant](https://github.com/tektoncd/pipeline/blob/main/docs/additional-configs.md#customizing-the-pipelines-controller-behavior).

Build and install from source with [ko](https://ko.build/):

```sh
ko apply -f config
```

This scheduler has only been tested on GKE version 1.25.

## Configuration

The scheduler can be configured with three `isolationStrategies`:

- `colocateAlways`: Multiple PipelineRuns' pods may be scheduled to the same node.
- `colocateCompleted`: Pods may be scheduled to a node if there are no pods from other PipelineRuns
currently running on it (completed PipelineRuns are OK).
- `isolateAlways`: Pods will not be scheduled to a node that has other PipelineRuns on it, regardless of whether they are running or completed.
This option requires an architecture that deletes PipelineRuns after completion,
or creates new nodes when a PipelineRun cannot be run on existing ones.

These options can be configured in [`scheduler-config.yaml`](./config/201-configmap.yaml).

## How it works

This scheduler implements the PreFilter and Filter extension points of the
[Kubernetes scheduler framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/)
and is run as a second scheduler (see ["Configuring multiple schedulers"](https://kubernetes.io/docs/tasks/extend-kubernetes/configure-multiple-schedulers)).

The PreFilter extension point determines what nodes the pod could be placed on by selecting pods with the same value for the label
"tekton.dev/pipelineRun" and considering only the node those pods are running on (if any).

The Filter extension points determines whether a pod can run on a given node by listing all pods associated with the node and rejecting the node if there
are pods for other PipelineRuns on it.

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
pipelinerun.tekton.dev/sample-pipelinerun-52sfx created
```

Get its pods and their nodes:
```sh
$ PIPELINERUN_NAME=sample-pipelinerun-52sfx
$ kubectl get pods -l tekton.dev/pipelineRun=$PIPELINERUN_NAME -o=custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
NAME                                      NODE
sample-pipelinerun-52sfx-first-pod        gke-scheduler-default-pool-906ef80f-xsg1
sample-pipelinerun-52sfx-last-pod         gke-scheduler-default-pool-906ef80f-xsg1
sample-pipelinerun-52sfx-parallel-1-pod   gke-scheduler-default-pool-906ef80f-xsg1
sample-pipelinerun-52sfx-parallel-2-pod   gke-scheduler-default-pool-906ef80f-xsg1
```

Get events (including scheduler events) for one of the involved pods:
```sh
$ POD_NAME=sample-pipelinerun-52sfx-parallel-1-pod
$ kubectl get events -n default --field-selector involvedObject.name=$POD_NAME
LAST SEEN   TYPE     REASON      OBJECT                                        MESSAGE
15m         Normal   Scheduled   pod/sample-pipelinerun-52sfx-parallel-1-pod   Successfully assigned default/sample-pipelinerun-52sfx-parallel-1-pod to gke-scheduler-default-pool-906ef80f-xsg1
...
```

### Example with IsolateAlways

Now, create as many PipelineRuns as the cluster has nodes:

```sh
$ kubectl create -f examples/pipelinerun.yaml
pipelinerun.tekton.dev/sample-pipelinerun-nwp7n created


$ kubectl create -f examples/pipelinerun.yaml
pipelinerun.tekton.dev/sample-pipelinerun-z6m9v created
```

Each PipelineRun runs on its own node:

```sh
$ kubectl get pods -l tekton.dev/pipelineRun=sample-pipelinerun-nwp7n -o=custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
NAME                                      NODE
sample-pipelinerun-nwp7n-first-pod        gke-scheduler-default-pool-e7e66a24-c7rf
sample-pipelinerun-nwp7n-last-pod         gke-scheduler-default-pool-e7e66a24-c7rf
sample-pipelinerun-nwp7n-parallel-1-pod   gke-scheduler-default-pool-e7e66a24-c7rf
sample-pipelinerun-nwp7n-parallel-2-pod   gke-scheduler-default-pool-e7e66a24-c7rf

$ kubectl get pods -l tekton.dev/pipelineRun=sample-pipelinerun-z6m9v -o=custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
NAME                                      NODE
sample-pipelinerun-z6m9v-first-pod        gke-scheduler-default-pool-ac08710a-04wh
sample-pipelinerun-z6m9v-last-pod         gke-scheduler-default-pool-ac08710a-04wh
sample-pipelinerun-z6m9v-parallel-1-pod   gke-scheduler-default-pool-ac08710a-04wh
sample-pipelinerun-z6m9v-parallel-2-pod   gke-scheduler-default-pool-ac08710a-04wh
```

Now, create one more PipelineRun. It has no node to run on, so it remains pending:
```sh
$ kubectl create -f examples/pipelinerun.yaml
pipelinerun.tekton.dev/sample-pipelinerun-t2lq4 created

$ kubectl get po -l tekton.dev/pipelineRun=sample-pipelinerun-t2lq4 -w
NAME                                 READY   STATUS    RESTARTS   AGE
sample-pipelinerun-t2lq4-first-pod   0/1     Pending   0          12s

$ kubectl describe po sample-pipelinerun-t2lq4-first-pod
...
Events:
  Type     Reason            Age   From                      Message
  ----     ------            ----  ----                      -------
  Warning  FailedScheduling  76s   one-node-per-pipelineRun  0/3 nodes are available: 1 node is already running PipelineRun sample-pipelinerun-52sfx but pod is associated with PipelineRun sample-pipelinerun-t2lq4, 1 node is already running PipelineRun sample-pipelinerun-nwp7n but pod is associated with PipelineRun sample-pipelinerun-t2lq4, 1 node is already running PipelineRun sample-pipelinerun-z6m9v but pod is associated with PipelineRun sample-pipelinerun-t2lq4. preemption: 0/3 nodes are available: 3 No preemption victims found for incoming pod..
```

With the current implementation, the GKE cluster autoscaler will not scale up the node pool.
