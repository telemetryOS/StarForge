package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/telemetryos/starforge/installer/client"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "installer server URL")
	flag.Parse()

	if err := client.RunTUI(*server); err != nil {
		log.Fatal(fmt.Errorf("installer TUI: %w", err))
	}
}
