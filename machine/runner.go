// Package machine boots sealed StarForge artifacts for interactive testing.
package machine

import (
	"context"

	"github.com/telemetryos/starforge/artifact"
	"github.com/telemetryos/starforge/config"
)

// RunRequest describes a target virtual machine invocation.
type RunRequest struct {
	Artifact *artifact.Receipt
	Disk     string
	BuildDir string
	Config   *config.QEMUConfig
	Serial   bool
}

// Runner starts a target machine without owning or mutating its base artifact.
type Runner interface {
	Run(context.Context, RunRequest) error
}
