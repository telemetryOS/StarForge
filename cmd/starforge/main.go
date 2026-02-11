package main

import (
	"fmt"
	"os"

	"github.com/telemetryos/starforge/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

