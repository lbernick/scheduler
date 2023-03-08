package main

import (
	"os"

	"github.com/lbernick/scheduler/pkg/scheduler"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	// Ensure scheme package is initialized.
	_ "github.com/lbernick/scheduler/pkg/apis/config/scheme"
)

func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(scheduler.SchedulerName, scheduler.NewScheduler),
	)
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
