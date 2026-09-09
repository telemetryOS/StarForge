package engine

import "testing"

type recordingHostTools struct {
	groups []string
}

func (r *recordingHostTools) Ensure(groups ...string) error {
	r.groups = append(r.groups, groups...)
	return nil
}

func TestBuilderUsesConfiguredHostTools(t *testing.T) {
	tools := &recordingHostTools{}
	builder := NewBuilderWithHostTools(nil, tools)
	if builder.hostTools != tools {
		t.Fatal("builder did not retain configured host tool provider")
	}
}

func TestSystemHostToolsRejectsRunGroup(t *testing.T) {
	if err := (SystemHostTools{}).Ensure("run"); err == nil {
		t.Fatal("build appliance unexpectedly accepted target run tools")
	}
}
