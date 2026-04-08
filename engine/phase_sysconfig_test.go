package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telemetryos/starforge/actions"
)

// phaseSysconfigFiles runs phaseSysconfig and returns the file contents written
// before locale-gen (or similar chroot commands) is attempted. The test
// accepts any error from ChrootRun since arch-chroot is unavailable in tests.
func runPhaseSysconfig(t *testing.T, ctx *actions.BuildContext, dir string) error {
	t.Helper()
	o, _ := InitOutput(dir, "test", "target")
	defer o.Close()
	b := &Builder{project: nil}
	return b.phaseSysconfig(ctx, dir)
}

func TestPhaseSysconfig_WritesHostname(t *testing.T) {
	dir := t.TempDir()
	ctx := &actions.BuildContext{Hostname: "edge-device"}
	runPhaseSysconfig(t, ctx, dir) // may fail at locale-gen, but hostname is written first

	data, err := os.ReadFile(filepath.Join(dir, "etc/hostname"))
	if err != nil {
		t.Fatalf("hostname not written: %v", err)
	}
	if string(data) != "edge-device\n" {
		t.Errorf("hostname = %q, want %q", string(data), "edge-device\n")
	}
}

func TestPhaseSysconfig_WritesLocaleConf(t *testing.T) {
	dir := t.TempDir()
	ctx := &actions.BuildContext{Locale: "en_US.UTF-8"}
	runPhaseSysconfig(t, ctx, dir)

	data, err := os.ReadFile(filepath.Join(dir, "etc/locale.conf"))
	if err != nil {
		t.Fatalf("locale.conf not written: %v", err)
	}
	if string(data) != "LANG=en_US.UTF-8\n" {
		t.Errorf("locale.conf = %q, want %q", string(data), "LANG=en_US.UTF-8\n")
	}
}

func TestPhaseSysconfig_WritesLocaleGen_NotDuplicated(t *testing.T) {
	dir := t.TempDir()
	ctx := &actions.BuildContext{
		Locale:  "en_US.UTF-8",
		Locales: []string{"de_DE.UTF-8"},
	}

	// Run twice — locale.gen must not accumulate duplicates (write not append)
	runPhaseSysconfig(t, ctx, dir)
	runPhaseSysconfig(t, ctx, dir)

	data, err := os.ReadFile(filepath.Join(dir, "etc/locale.gen"))
	if err != nil {
		t.Fatalf("locale.gen not written: %v", err)
	}
	content := string(data)

	// Each locale must appear exactly once
	for _, want := range []string{"en_US.UTF-8 UTF-8\n", "de_DE.UTF-8 UTF-8\n"} {
		idx := strings.Index(content, want)
		if idx < 0 {
			t.Errorf("locale.gen missing %q:\n%s", want, content)
			continue
		}
		// Must not appear a second time
		if strings.Index(content[idx+1:], want) >= 0 {
			t.Errorf("locale.gen has duplicate %q:\n%s", want, content)
		}
	}
}

func TestPhaseSysconfig_LocaleGen_DeduplicatesPrimary(t *testing.T) {
	dir := t.TempDir()
	// Primary locale also listed in Locales — must appear once in locale.gen
	ctx := &actions.BuildContext{
		Locale:  "en_US.UTF-8",
		Locales: []string{"en_US.UTF-8"},
	}
	runPhaseSysconfig(t, ctx, dir)

	data, _ := os.ReadFile(filepath.Join(dir, "etc/locale.gen"))
	content := string(data)
	count := strings.Count(content, "en_US.UTF-8 UTF-8\n")
	if count != 1 {
		t.Errorf("expected en_US.UTF-8 exactly once in locale.gen, got %d:\n%s", count, content)
	}
}

func TestPhaseSysconfig_CreatesTimezoneSymlink(t *testing.T) {
	dir := t.TempDir()
	ctx := &actions.BuildContext{Timezone: "America/New_York"}
	runPhaseSysconfig(t, ctx, dir)

	link := filepath.Join(dir, "etc/localtime")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	want := "/usr/share/zoneinfo/America/New_York"
	if target != want {
		t.Errorf("symlink target = %q, want %q", target, want)
	}
}

func TestPhaseSysconfig_TimezoneSymlink_Idempotent(t *testing.T) {
	dir := t.TempDir()
	ctx := &actions.BuildContext{Timezone: "UTC"}
	runPhaseSysconfig(t, ctx, dir)
	if err := runPhaseSysconfig(t, ctx, dir); err != nil {
		// Only fail if the error is about the symlink, not locale-gen
		if strings.Contains(err.Error(), "localtime") {
			t.Errorf("second run failed on timezone symlink: %v", err)
		}
	}
}

func TestPhaseSysconfig_EmptyContext_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	ctx := &actions.BuildContext{}
	runPhaseSysconfig(t, ctx, dir)

	for _, f := range []string{"etc/hostname", "etc/locale.conf", "etc/locale.gen", "etc/localtime"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("unexpected file %s for empty context", f)
		}
	}
}
