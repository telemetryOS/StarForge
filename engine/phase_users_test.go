package engine

import (
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

func TestPhaseUsers_RejectsNewlineInPassword(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{
		Users: []actions.UserDef{
			{Name: "alice", Password: "pass\nword"},
		},
	}
	b := &Builder{project: nil}
	err := b.phaseUsers(ctx, dir)
	if err == nil {
		t.Fatal("expected error for password containing newline")
	}
	if !strings.Contains(err.Error(), "newline") {
		t.Errorf("error should mention newline: %v", err)
	}
}

func TestPhaseUsers_RejectsCarriageReturnInPassword(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{
		Users: []actions.UserDef{
			{Name: "bob", Password: "pass\rword"},
		},
	}
	b := &Builder{project: nil}
	err := b.phaseUsers(ctx, dir)
	if err == nil {
		t.Fatal("expected error for password containing carriage return")
	}
	if !strings.Contains(err.Error(), "newline") {
		t.Errorf("error should mention newline: %v", err)
	}
}

func TestPhaseUsers_EmptyContext_NoOp(t *testing.T) {
	dir := t.TempDir()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()

	ctx := &actions.BuildContext{}
	b := &Builder{project: nil}
	// Empty context: no groups or users to create, should succeed immediately
	// (ChrootRun is never called, so no arch-chroot dependency)
	if err := b.phaseUsers(ctx, dir); err != nil {
		t.Fatalf("phaseUsers on empty context error: %v", err)
	}
}
