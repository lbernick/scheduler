Running this binary with `go run ./cmd` requires supplying a kubeconfig.
See https://kubernetes.io/docs/reference/command-line-tools-reference/kube-scheduler/
for a reference of the arguments accepted by this binary.
When this binary is run in a deployment in a Kubernetes cluster, it uses the in-cluster
kubeconfig and "just works".