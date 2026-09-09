package engine

import (
	"os"
	"path/filepath"
	"slices"
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

func TestUseraddArgs_PinnedIdentity(t *testing.T) {
	got := useraddArgs(actions.UserDef{
		Name:         "data",
		PrimaryGroup: "data",
		Groups:       []string{"storage"},
		Shell:        "/usr/bin/nologin",
		System:       true,
		UID:          962,
	})
	want := []string{
		"useradd", "-r", "-M", "-s", "/usr/bin/nologin",
		"-u", "962", "-g", "data", "-G", "storage", "data",
	}
	if !slices.Equal(got, want) {
		t.Errorf("useradd args = %v, want %v", got, want)
	}
}

func TestVerifyGroupGID(t *testing.T) {
	rootfs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootfs, "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "etc", "group"), []byte("data:x:962:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := verifyGroupGID(rootfs, "data", 962); err != nil {
		t.Fatalf("verifyGroupGID returned error: %v", err)
	}
	if err := verifyGroupGID(rootfs, "data", 963); err == nil || !strings.Contains(err.Error(), "want pinned gid 963") {
		t.Fatalf("verifyGroupGID mismatch error = %v", err)
	}
	if err := verifyGroupGID(rootfs, "recovery", 961); err == nil || !strings.Contains(err.Error(), "was not created") {
		t.Fatalf("verifyGroupGID missing error = %v", err)
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
