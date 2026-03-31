package engine

import (
	"strings"
	"testing"
)

// --- injectPrelude tests ---

func TestInjectPrelude_WithShebang(t *testing.T) {
	script := "#!/bin/bash\necho hello\n"
	prelude := "# prelude\n"
	got := injectPrelude(script, prelude)
	want := "#!/bin/bash\n# prelude\necho hello\n"
	if got != want {
		t.Errorf("injectPrelude with shebang:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInjectPrelude_WithoutShebang(t *testing.T) {
	script := "echo hello\n"
	prelude := "# prelude\n"
	got := injectPrelude(script, prelude)
	want := "# prelude\necho hello\n"
	if got != want {
		t.Errorf("injectPrelude without shebang:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInjectPrelude_ShebangOnly(t *testing.T) {
	// Shebang with no newline — can't split, so prepend
	script := "#!/bin/bash"
	prelude := "# prelude\n"
	got := injectPrelude(script, prelude)
	want := "# prelude\n#!/bin/bash"
	if got != want {
		t.Errorf("injectPrelude shebang-only:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInjectPrelude_EnvShebang(t *testing.T) {
	script := "#!/usr/bin/env bash\nset -e\necho test\n"
	prelude := "export FOO=bar\n"
	got := injectPrelude(script, prelude)
	want := "#!/usr/bin/env bash\nexport FOO=bar\nset -e\necho test\n"
	if got != want {
		t.Errorf("injectPrelude env shebang:\ngot:  %q\nwant: %q", got, want)
	}
}

// --- mergeScriptEnv tests ---

func TestMergeScriptEnv_BothNil(t *testing.T) {
	got := mergeScriptEnv(nil, nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestMergeScriptEnv_TargetOnly(t *testing.T) {
	target := map[string]string{"FOO": "bar"}
	got := mergeScriptEnv(target, nil)
	if got["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "bar")
	}
}

func TestMergeScriptEnv_StepOverridesTarget(t *testing.T) {
	target := map[string]string{"FOO": "bar", "BAZ": "qux"}
	step := map[string]string{"FOO": "overridden", "NEW": "value"}
	got := mergeScriptEnv(target, step)
	if got["FOO"] != "overridden" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "overridden")
	}
	if got["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", got["BAZ"], "qux")
	}
	if got["NEW"] != "value" {
		t.Errorf("NEW = %q, want %q", got["NEW"], "value")
	}
}

// --- buildScriptPrelude tests ---

func TestBuildScriptPrelude_Empty(t *testing.T) {
	got := buildScriptPrelude(nil)
	if !strings.Contains(got, "sf_set()") {
		t.Error("prelude should contain sf_set")
	}
	if !strings.Contains(got, "sf_get()") {
		t.Error("prelude should contain sf_get")
	}
	if !strings.Contains(got, "declare -A __sf_vars=()") {
		t.Error("prelude should contain empty __sf_vars")
	}
}

func TestBuildScriptPrelude_WithVars(t *testing.T) {
	vars := map[string]string{"hostname": "edge-01", "mode": "production"}
	got := buildScriptPrelude(vars)
	if !strings.Contains(got, "[hostname]='edge-01'") {
		t.Errorf("prelude should contain hostname var, got:\n%s", got)
	}
	if !strings.Contains(got, "[mode]='production'") {
		t.Errorf("prelude should contain mode var, got:\n%s", got)
	}
}

func TestBuildScriptPrelude_SingleQuoteEscaping(t *testing.T) {
	vars := map[string]string{"msg": "it's a test"}
	got := buildScriptPrelude(vars)
	// Single quotes are escaped: ' → '\''
	if !strings.Contains(got, `it'\''s a test`) {
		t.Errorf("single quote not escaped in:\n%s", got)
	}
}

func TestBuildScriptPrelude_SortedKeys(t *testing.T) {
	vars := map[string]string{"z_var": "last", "a_var": "first", "m_var": "middle"}
	got := buildScriptPrelude(vars)
	aIdx := strings.Index(got, "[a_var]")
	mIdx := strings.Index(got, "[m_var]")
	zIdx := strings.Index(got, "[z_var]")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("vars not sorted in prelude:\n%s", got)
	}
}


// --- varNameRe validation (sf_set output key validation) ---

func TestVarNameRe_AcceptsValid(t *testing.T) {
	valid := []string{
		"foo", "Foo", "FOO", "_foo", "foo_bar", "foo123", "_", "a1_B2",
	}
	for _, name := range valid {
		if !varNameRe.MatchString(name) {
			t.Errorf("varNameRe should accept %q", name)
		}
	}
}

func TestVarNameRe_RejectsInvalid(t *testing.T) {
	invalid := []string{
		"", "1foo", "foo-bar", "foo.bar", "foo bar", "foo=bar",
		"my-key", "key with spaces", "123",
	}
	for _, name := range invalid {
		if varNameRe.MatchString(name) {
			t.Errorf("varNameRe should reject %q", name)
		}
	}
}
