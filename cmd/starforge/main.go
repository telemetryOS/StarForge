package main

import (
	"fmt"
	"os"

	"github.com/telemetryos/starforge/commands"
	"github.com/telemetryos/starforge/worker"
)

func main() {
	if os.Getpid() == 1 {
		if err := worker.RunInit(); err != nil {
			fmt.Fprintf(os.Stderr, "StarForge worker failed: %s\n", err)
			os.Exit(1)
		}
		return
	}
	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
