package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/telemetryos/starforge/installer/client"
)

func main() {
	server := flag.String("server", "http://localhost:8100", "installer server URL")
	unattended := flag.Bool("unattended", false, "run in unattended mode")
	flag.Parse()

	if err := client.RunTUI(*server, *unattended); err != nil {
		log.Fatal(fmt.Errorf("installer TUI: %w", err))
	}
}
