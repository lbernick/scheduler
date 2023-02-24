package main

import (
	"os"

	"github.com/lbernick/scheduler/pkg/scheduler"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	s := scheduler.Scheduler{}
	command := app.NewSchedulerCommand(
		app.WithPlugin(s.Name(), scheduler.NewScheduler),
	)
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
