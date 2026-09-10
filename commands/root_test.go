package commands

import (
	"testing"

	"github.com/telemetryos/starforge/executor"
)

func TestDefaultExecutorIsVM(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("executor")
	if flag == nil {
		t.Fatal("executor flag is not registered")
	}
	if flag.DefValue != string(executor.ModeVM) {
		t.Fatalf("executor default = %q, want %q", flag.DefValue, executor.ModeVM)
	}
}
