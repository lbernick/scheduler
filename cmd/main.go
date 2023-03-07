package main

import (
	"os"

	"github.com/lbernick/scheduler/pkg/scheduler"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(scheduler.SchedulerName, scheduler.NewScheduler),
	)
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
