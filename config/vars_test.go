package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVarPattern(t *testing.T) {
	pat := VarPattern()

	matches := []struct {
		input string
		name  string
	}{
		{"${{ foo }}", "foo"},
		{"${{foo}}", "foo"},
		{"${{  bar  }}", "bar"},
		{"${{ my_var }}", "my_var"},
		{"${{ _private }}", "_private"},
		{"${{ var123 }}", "var123"},
	}
	for _, m := range matches {
		sub := pat.FindStringSubmatch(m.input)
		if len(sub) < 2 {
			t.Errorf("VarPattern did not match %q", m.input)
			continue
		}
		if sub[1] != m.name {
			t.Errorf("VarPattern(%q) captured %q, want %q", m.input, sub[1], m.name)
		}
	}

	noMatch := []string{
		"${foo}",        // single brace
		"$foo",          // no braces
		"{{ foo }}",     // no dollar sign
		"${{ 123abc }}", // starts with digit
	}
	for _, s := range noMatch {
		if pat.MatchString(s) {
			t.Errorf("VarPattern should not match %q", s)
		}
	}
}

func TestSubstituteVars_SimpleScalar(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "Hello ${{ name }}!"}
	vars := map[string]string{"name": "world"}

	if err := SubstituteVars(node, vars); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if node.Value != "Hello world!" {
		t.Errorf("got %q, want %q", node.Value, "Hello world!")
	}
}

func TestSubstituteVars_MultipleVars(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "${{ host }}:${{ port }}"}
	vars := map[string]string{"host": "localhost", "port": "8080"}

	if err := SubstituteVars(node, vars); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if node.Value != "localhost:8080" {
		t.Errorf("got %q, want %q", node.Value, "localhost:8080")
	}
}

func TestSubstituteVars_UndefinedError(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "${{ missing }}"}
	vars := map[string]string{}

	err := SubstituteVars(node, vars)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if got := err.Error(); got != "undefined variable: missing" {
		t.Errorf("error = %q, want %q", got, "undefined variable: missing")
	}
}

func TestSubstituteVars_NoVarRef(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "no variables here"}
	if err := SubstituteVars(node, nil); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if node.Value != "no variables here" {
		t.Errorf("value changed unexpectedly: %q", node.Value)
	}
}

func TestSubstituteVars_SkipsIncludeTag(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!include", Value: "${{ path }}"}
	if err := SubstituteVars(node, map[string]string{"path": "/resolved"}); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if node.Value != "${{ path }}" {
		t.Errorf("!include node was substituted: got %q", node.Value)
	}
}

func TestSubstituteVars_MappingNode(t *testing.T) {
	// Build: {key: "${{ val }}"}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "key"},
			{Kind: yaml.ScalarNode, Value: "${{ val }}"},
		},
	}
	if err := SubstituteVars(root, map[string]string{"val": "resolved"}); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if root.Content[1].Value != "resolved" {
		t.Errorf("mapping value = %q, want %q", root.Content[1].Value, "resolved")
	}
}

func TestSubstituteVars_SequenceNode(t *testing.T) {
	root := &yaml.Node{
		Kind: yaml.SequenceNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "${{ item1 }}"},
			{Kind: yaml.ScalarNode, Value: "literal"},
			{Kind: yaml.ScalarNode, Value: "${{ item2 }}"},
		},
	}
	vars := map[string]string{"item1": "first", "item2": "third"}
	if err := SubstituteVars(root, vars); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	want := []string{"first", "literal", "third"}
	for i, w := range want {
		if root.Content[i].Value != w {
			t.Errorf("seq[%d] = %q, want %q", i, root.Content[i].Value, w)
		}
	}
}

func TestSubstituteVars_DocumentNode(t *testing.T) {
	doc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "${{ greeting }}"},
		},
	}
	if err := SubstituteVars(doc, map[string]string{"greeting": "hello"}); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if doc.Content[0].Value != "hello" {
		t.Errorf("doc child = %q, want %q", doc.Content[0].Value, "hello")
	}
}

func TestSubstituteVars_RealWorldEdgeOS(t *testing.T) {
	// Edge-OS uses target args like device_hostname passed into layers
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "${{ device_hostname }}"}
	vars := map[string]string{"device_hostname": "edge-device-01"}
	if err := SubstituteVars(node, vars); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if node.Value != "edge-device-01" {
		t.Errorf("got %q, want %q", node.Value, "edge-device-01")
	}
}

func TestDeepCopyNode_Nil(t *testing.T) {
	if DeepCopyNode(nil) != nil {
		t.Error("DeepCopyNode(nil) should return nil")
	}
}

func TestDeepCopyNode_Scalar(t *testing.T) {
	orig := &yaml.Node{Kind: yaml.ScalarNode, Value: "hello", Tag: "!!str"}
	cp := DeepCopyNode(orig)

	if cp == orig {
		t.Error("copy should be a different pointer")
	}
	if cp.Value != orig.Value {
		t.Errorf("copy value = %q, want %q", cp.Value, orig.Value)
	}
	if cp.Tag != orig.Tag {
		t.Errorf("copy tag = %q, want %q", cp.Tag, orig.Tag)
	}

	// Mutation independence
	cp.Value = "modified"
	if orig.Value != "hello" {
		t.Error("modifying copy changed original")
	}
}

func TestDeepCopyNode_Mapping(t *testing.T) {
	orig := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "key"},
			{Kind: yaml.ScalarNode, Value: "value"},
		},
	}
	cp := DeepCopyNode(orig)

	if len(cp.Content) != 2 {
		t.Fatalf("copy content length = %d, want 2", len(cp.Content))
	}
	if cp.Content[0] == orig.Content[0] {
		t.Error("copy content children should be different pointers")
	}

	// Mutation independence
	cp.Content[1].Value = "changed"
	if orig.Content[1].Value != "value" {
		t.Error("modifying copy changed original child")
	}
}

func TestDeepCopyNode_Nested(t *testing.T) {
	// mapping { seq: [a, b] }
	orig := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "seq"},
			{
				Kind: yaml.SequenceNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "a"},
					{Kind: yaml.ScalarNode, Value: "b"},
				},
			},
		},
	}
	cp := DeepCopyNode(orig)

	// Modify deeply nested
	cp.Content[1].Content[0].Value = "X"
	if orig.Content[1].Content[0].Value != "a" {
		t.Error("deep modification changed original")
	}
}

func TestSubstituteVars_AliasNode(t *testing.T) {
	// A simple alias pointing to a scalar should have its value substituted.
	target := &yaml.Node{Kind: yaml.ScalarNode, Value: "${{ name }}"}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: target}

	if err := SubstituteVars(alias, map[string]string{"name": "world"}); err != nil {
		t.Fatalf("SubstituteVars on alias error: %v", err)
	}
	if target.Value != "world" {
		t.Errorf("alias target value = %q, want %q", target.Value, "world")
	}
}

func TestSubstituteVars_CircularAlias_NoInfiniteLoop(t *testing.T) {
	// A circular alias must not cause infinite recursion.
	node := &yaml.Node{Kind: yaml.AliasNode}
	node.Alias = node // self-referential

	// Must return without hanging or panicking.
	if err := SubstituteVars(node, map[string]string{}); err != nil {
		t.Fatalf("SubstituteVars on circular alias error: %v", err)
	}
}

func TestSubstituteVars_SharedAliasTarget_OnlySubstitutedOnce(t *testing.T) {
	// Two alias nodes pointing to the same target should not double-substitute.
	target := &yaml.Node{Kind: yaml.ScalarNode, Value: "${{ v }}"}
	root := &yaml.Node{
		Kind: yaml.SequenceNode,
		Content: []*yaml.Node{
			{Kind: yaml.AliasNode, Alias: target},
			{Kind: yaml.AliasNode, Alias: target},
		},
	}

	if err := SubstituteVars(root, map[string]string{"v": "ok"}); err != nil {
		t.Fatalf("SubstituteVars error: %v", err)
	}
	if target.Value != "ok" {
		t.Errorf("target.Value = %q, want %q", target.Value, "ok")
	}
}

func TestDeepCopyNode_PreservesMetadata(t *testing.T) {
	orig := &yaml.Node{
		Kind:        yaml.ScalarNode,
		Value:       "test",
		Anchor:      "myAnchor",
		HeadComment: "# head",
		LineComment: "# line",
		FootComment: "# foot",
		Line:        42,
		Column:      5,
	}
	cp := DeepCopyNode(orig)

	if cp.Anchor != "myAnchor" {
		t.Errorf("Anchor = %q, want %q", cp.Anchor, "myAnchor")
	}
	if cp.HeadComment != "# head" {
		t.Errorf("HeadComment = %q, want %q", cp.HeadComment, "# head")
	}
	if cp.Line != 42 {
		t.Errorf("Line = %d, want 42", cp.Line)
	}
	if cp.Column != 5 {
		t.Errorf("Column = %d, want 5", cp.Column)
	}
}
