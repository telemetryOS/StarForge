package actions

import (
	"testing"
)

func TestInsertPattern_Before(t *testing.T) {
	content := "line1\nline2\nline3"
	got := InsertPattern(content, "^line2$", "inserted", "before", 0)
	want := "line1\ninserted\nline2\nline3"
	if got != want {
		t.Errorf("InsertPattern before:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInsertPattern_After(t *testing.T) {
	content := "line1\nline2\nline3"
	got := InsertPattern(content, "^line2$", "inserted", "after", 0)
	want := "line1\nline2\ninserted\nline3"
	if got != want {
		t.Errorf("InsertPattern after:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInsertPattern_MatchLimit(t *testing.T) {
	content := "match\nother\nmatch\nend"

	// match=1 should only insert on first occurrence
	got := InsertPattern(content, "^match$", "INS", "after", 1)
	want := "match\nINS\nother\nmatch\nend"
	if got != want {
		t.Errorf("InsertPattern match=1:\ngot:  %q\nwant: %q", got, want)
	}

	// match=0 means all occurrences
	got = InsertPattern(content, "^match$", "INS", "after", 0)
	want = "match\nINS\nother\nmatch\nINS\nend"
	if got != want {
		t.Errorf("InsertPattern match=0 (all):\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInsertPattern_NoMatch(t *testing.T) {
	content := "line1\nline2\nline3"
	got := InsertPattern(content, "^nope$", "inserted", "before", 0)
	if got != content {
		t.Errorf("InsertPattern no match should return original:\ngot:  %q\nwant: %q", got, content)
	}
}

func TestInsertPattern_InvalidRegex(t *testing.T) {
	content := "line1\nline2"
	got := InsertPattern(content, "[invalid", "inserted", "before", 0)
	if got != content {
		t.Errorf("InsertPattern invalid regex should return original:\ngot:  %q\nwant: %q", got, content)
	}
}

// Test with systemd-style config similar to Edge-OS drop-ins
func TestInsertPattern_SystemdSection(t *testing.T) {
	content := "[Unit]\nDescription=Test\n\n[Service]\nType=simple\nExecStart=/usr/bin/test\n\n[Install]\nWantedBy=multi-user.target"
	got := InsertPattern(content, `^\[Service\]$`, "After=network.target", "after", 1)
	if got != "[Unit]\nDescription=Test\n\n[Service]\nAfter=network.target\nType=simple\nExecStart=/usr/bin/test\n\n[Install]\nWantedBy=multi-user.target" {
		t.Errorf("InsertPattern systemd section:\ngot: %q", got)
	}
}

func TestTruncatePattern_Before(t *testing.T) {
	content := "header\ngarbage\n# START\nkeep1\nkeep2"
	got := TruncatePattern(content, "^# START$", "truncate_before", 0)
	want := "# START\nkeep1\nkeep2"
	if got != want {
		t.Errorf("TruncatePattern before:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestTruncatePattern_After(t *testing.T) {
	content := "keep1\nkeep2\n# END\ngarbage\nmore"
	got := TruncatePattern(content, "^# END$", "truncate_after", 0)
	want := "keep1\nkeep2\n# END"
	if got != want {
		t.Errorf("TruncatePattern after:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestTruncatePattern_NthOccurrence(t *testing.T) {
	content := "MARK\nline1\nMARK\nline2\nMARK\nline3"

	// match=2 targets second occurrence
	got := TruncatePattern(content, "^MARK$", "truncate_before", 2)
	want := "MARK\nline2\nMARK\nline3"
	if got != want {
		t.Errorf("TruncatePattern before match=2:\ngot:  %q\nwant: %q", got, want)
	}

	got = TruncatePattern(content, "^MARK$", "truncate_after", 2)
	want = "MARK\nline1\nMARK"
	if got != want {
		t.Errorf("TruncatePattern after match=2:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestTruncatePattern_DefaultMatch(t *testing.T) {
	// match=0 defaults to 1
	content := "before\nMARK\nafter"
	got := TruncatePattern(content, "^MARK$", "truncate_before", 0)
	want := "MARK\nafter"
	if got != want {
		t.Errorf("TruncatePattern default match:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestTruncatePattern_NoMatch(t *testing.T) {
	content := "line1\nline2\nline3"
	got := TruncatePattern(content, "^NOPE$", "truncate_before", 0)
	if got != content {
		t.Errorf("TruncatePattern no match should return original:\ngot:  %q\nwant: %q", got, content)
	}
}
