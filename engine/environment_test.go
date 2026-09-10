package engine

import (
	"strings"
	"testing"
)

func TestTargetEnvUsesTargetLocalTemporaryDirectory(t *testing.T) {
	t.Setenv("TMPDIR", "/work/tmp")
	t.Setenv("TMP", "/host/tmp")
	t.Setenv("TEMP", "/host/temp")
	t.Setenv("STARFORGE_TMPDIR", "/work/tmp")

	values := map[string]string{}
	for _, entry := range targetEnv() {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		if values[name] != "/tmp" {
			t.Errorf("%s = %q, want /tmp", name, values[name])
		}
	}
	if _, ok := values["STARFORGE_TMPDIR"]; ok {
		t.Fatal("host-only STARFORGE_TMPDIR leaked into target environment")
	}
}
