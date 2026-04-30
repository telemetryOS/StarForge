package installations

import "testing"

func TestSanitizeLogLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "hello world"},
		{"keeps tab", "a\tb", "a\tb"},
		{"keeps newline mid-string", "a\nb", "a\nb"},
		{"strips bell", "a\x07b", "ab"},
		{"strips raw esc alone", "x\x1by", "xy"},
		{"strips csi color", "\x1b[31mred\x1b[0m", "red"},
		{"strips csi cursor", "abc\x1b[2Jxyz", "abcxyz"},
		{"strips osc title", "\x1b]0;hostile title\x07normal", "normal"},
		{"strips osc title (ST)", "\x1b]0;hostile\x1b\\normal", "normal"},
		{"strips DEL", "a\x7fb", "ab"},
		{"keeps high-ASCII", "ünïcödé", "ünïcödé"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeLogLine(c.in)
			if got != c.want {
				t.Errorf("sanitizeLogLine(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
