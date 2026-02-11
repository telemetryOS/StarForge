package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- navigateYAMLPath tests ---

func parseYAML(t *testing.T, input string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &doc
}

func TestNavigateYAMLPath_MappingKey(t *testing.T) {
	node := parseYAML(t, "top:\n  nested: value")
	result, err := navigateYAMLPath(node, "top.nested")
	if err != nil {
		t.Fatalf("navigateYAMLPath error: %v", err)
	}
	if result.Value != "value" {
		t.Errorf("got %q, want %q", result.Value, "value")
	}
}

func TestNavigateYAMLPath_SequenceIndex(t *testing.T) {
	node := parseYAML(t, "items:\n  - alpha\n  - beta\n  - gamma")
	result, err := navigateYAMLPath(node, "items.1")
	if err != nil {
		t.Fatalf("navigateYAMLPath error: %v", err)
	}
	if result.Value != "beta" {
		t.Errorf("got %q, want %q", result.Value, "beta")
	}
}

func TestNavigateYAMLPath_EmptyParts(t *testing.T) {
	node := parseYAML(t, "key: value")
	result, err := navigateYAMLPath(node, "key")
	if err != nil {
		t.Fatalf("navigateYAMLPath error: %v", err)
	}
	if result.Value != "value" {
		t.Errorf("got %q, want %q", result.Value, "value")
	}
}

func TestNavigateYAMLPath_KeyNotFound(t *testing.T) {
	node := parseYAML(t, "foo: bar")
	_, err := navigateYAMLPath(node, "missing")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestNavigateYAMLPath_IndexOutOfRange(t *testing.T) {
	node := parseYAML(t, "items:\n  - one")
	_, err := navigateYAMLPath(node, "items.5")
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestNavigateYAMLPath_NonIntegerIndex(t *testing.T) {
	node := parseYAML(t, "items:\n  - one")
	_, err := navigateYAMLPath(node, "items.abc")
	if err == nil {
		t.Fatal("expected error for non-integer index on sequence")
	}
}

func TestNavigateYAMLPath_NavigateIntoScalar(t *testing.T) {
	node := parseYAML(t, "key: value")
	_, err := navigateYAMLPath(node, "key.deeper")
	if err == nil {
		t.Fatal("expected error navigating into scalar")
	}
}

func TestNavigateYAMLPath_DeepNesting(t *testing.T) {
	node := parseYAML(t, "a:\n  b:\n    c: deep")
	result, err := navigateYAMLPath(node, "a.b.c")
	if err != nil {
		t.Fatalf("navigateYAMLPath error: %v", err)
	}
	if result.Value != "deep" {
		t.Errorf("got %q, want %q", result.Value, "deep")
	}
}

// --- ResolveIncludes tests ---

func TestResolveIncludes_ScalarInclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fragment.yaml"), []byte("included_key: included_value"), 0o644)

	input := "data: !include ./fragment.yaml"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if err := ResolveIncludes(&doc, dir, ""); err != nil {
		t.Fatalf("ResolveIncludes error: %v", err)
	}

	// The value of "data" should now be the mapping from fragment.yaml
	root := doc.Content[0] // unwrap document
	if root.Kind != yaml.MappingNode {
		t.Fatalf("root kind = %d, want MappingNode", root.Kind)
	}
	val := root.Content[1]
	if val.Kind != yaml.MappingNode {
		t.Fatalf("included value kind = %d, want MappingNode", val.Kind)
	}
	if val.Content[0].Value != "included_key" {
		t.Errorf("got key %q, want %q", val.Content[0].Value, "included_key")
	}
	if val.Content[1].Value != "included_value" {
		t.Errorf("got value %q, want %q", val.Content[1].Value, "included_value")
	}
}

func TestResolveIncludes_SequenceSplice(t *testing.T) {
	dir := t.TempDir()
	// Fragment is a sequence
	os.WriteFile(filepath.Join(dir, "items.yaml"), []byte("- c\n- d"), 0o644)

	input := "items:\n  - a\n  - b\n  - !include ./items.yaml\n  - e"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if err := ResolveIncludes(&doc, dir, ""); err != nil {
		t.Fatalf("ResolveIncludes error: %v", err)
	}

	// items should now be [a, b, c, d, e]
	root := doc.Content[0]
	seq := root.Content[1]
	if seq.Kind != yaml.SequenceNode {
		t.Fatalf("expected sequence, got kind %d", seq.Kind)
	}
	got := make([]string, len(seq.Content))
	for i, n := range seq.Content {
		got[i] = n.Value
	}
	want := []string{"a", "b", "c", "d", "e"}
	if len(got) != len(want) {
		t.Fatalf("seq length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("seq[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveIncludes_MappingIncludeWithYAMLPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "data.yaml"), []byte("outer:\n  inner: found"), 0o644)

	// !include with yaml_path to navigate into the included file
	input := "result: !include\n  layer_path: ./data.yaml\n  yaml_path: outer.inner"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if err := ResolveIncludes(&doc, dir, ""); err != nil {
		t.Fatalf("ResolveIncludes error: %v", err)
	}

	root := doc.Content[0]
	val := root.Content[1]
	if val.Value != "found" {
		t.Errorf("got %q, want %q", val.Value, "found")
	}
}

func TestResolveIncludes_MissingFile(t *testing.T) {
	dir := t.TempDir()
	input := "data: !include ./nonexistent.yaml"
	var doc yaml.Node
	yaml.Unmarshal([]byte(input), &doc)

	err := ResolveIncludes(&doc, dir, "")
	if err == nil {
		t.Fatal("expected error for missing include file")
	}
}

func TestResolveIncludes_DepthLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a self-referencing include
	os.WriteFile(filepath.Join(dir, "loop.yaml"), []byte("data: !include ./loop.yaml"), 0o644)

	input := "root: !include ./loop.yaml"
	var doc yaml.Node
	yaml.Unmarshal([]byte(input), &doc)

	err := ResolveIncludes(&doc, dir, "")
	if err == nil {
		t.Fatal("expected error for include depth limit")
	}
}

func TestResolveIncludes_NestedIncludes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "inner.yaml"), []byte("deep: value"), 0o644)
	os.WriteFile(filepath.Join(dir, "outer.yaml"), []byte("nested: !include ./inner.yaml"), 0o644)

	input := "top: !include ./outer.yaml"
	var doc yaml.Node
	yaml.Unmarshal([]byte(input), &doc)

	if err := ResolveIncludes(&doc, dir, ""); err != nil {
		t.Fatalf("ResolveIncludes error: %v", err)
	}

	// top -> mapping { nested -> mapping { deep -> value } }
	root := doc.Content[0]
	topVal := root.Content[1] // outer.yaml content
	if topVal.Kind != yaml.MappingNode {
		t.Fatalf("expected mapping, got kind %d", topVal.Kind)
	}
	nestedVal := topVal.Content[1] // inner.yaml content
	if nestedVal.Kind != yaml.MappingNode {
		t.Fatalf("expected mapping for nested, got kind %d", nestedVal.Kind)
	}
	if nestedVal.Content[1].Value != "value" {
		t.Errorf("nested include value = %q, want %q", nestedVal.Content[1].Value, "value")
	}
}

func TestResolveIncludes_RelativeToIncludedFile(t *testing.T) {
	// Create subdir structure:
	// dir/main.yaml includes subdir/a.yaml which includes b.yaml (relative to subdir)
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0o755)

	os.WriteFile(filepath.Join(subdir, "b.yaml"), []byte("result: found"), 0o644)
	os.WriteFile(filepath.Join(subdir, "a.yaml"), []byte("data: !include ./b.yaml"), 0o644)

	input := "top: !include ./subdir/a.yaml"
	var doc yaml.Node
	yaml.Unmarshal([]byte(input), &doc)

	if err := ResolveIncludes(&doc, dir, ""); err != nil {
		t.Fatalf("ResolveIncludes error: %v", err)
	}

	root := doc.Content[0]
	topVal := root.Content[1]
	if topVal.Kind != yaml.MappingNode {
		t.Fatalf("expected mapping, got kind %d", topVal.Kind)
	}
	innerVal := topVal.Content[1]
	if innerVal.Kind != yaml.MappingNode {
		t.Fatalf("expected mapping for inner, got kind %d", innerVal.Kind)
	}
	if innerVal.Content[1].Value != "found" {
		t.Errorf("nested relative include value = %q, want %q", innerVal.Content[1].Value, "found")
	}
}
