package actions

import (
	"fmt"
	"sync"

	"github.com/telemetryos/starforge/config"
)

// Action is the interface that all build actions implement.
// Actions are declarative: they populate the BuildContext during the Collect
// phase and never execute side effects directly.
type Action interface {
	Name() string
	Execute(step config.Step, layerDir string, ctx *BuildContext) error
}

var (
	registry   = make(map[string]Action)
	registryMu sync.RWMutex
)

// Register adds an action to the global registry. Called from init() in each action file.
func Register(a Action) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[a.Name()] = a
}

// Get retrieves an action by name from the registry.
func Get(name string) (Action, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown action: %q", name)
	}
	return a, nil
}

func init() {
	// Package management
	Register(&PacmanAdd{})
	Register(&PacmanRemove{})

	// File operations
	Register(&FileCreate{})
	Register(&FileEdit{})
	Register(&FileCopy{})
	Register(&FileMove{})
	Register(&FileDelete{})
	Register(&FileLink{})
	Register(&FilePermissions{})
	Register(&FileOwnership{})
	Register(&FileMkdir{})

	// System configuration
	Register(&SystemHostname{})
	Register(&SystemLocale{})
	Register(&SystemTimezone{})
	Register(&SystemKeymap{})
	Register(&SystemUser{})
	Register(&SystemGroup{})

	// Systemd units
	Register(&SystemdService{})
	Register(&SystemdMount{})
	Register(&SystemdTimer{})
	Register(&SystemdSocket{})
	Register(&SystemdSlice{})
	Register(&SystemdTarget{})
	Register(&SystemdBootInstall{})

	// Partitions
	Register(&PartitionAdd{})
	Register(&PartitionRemove{})
	Register(&PartitionChange{})

	// Scripts
	Register(&Run{})

	// Installer
	Register(&InstallServer{})
	Register(&InstallClient{})
	Register(&InstallPayload{})
}
