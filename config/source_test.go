package config

import (
	"testing"
)

func TestIsGitSource(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/user/repo.git", true},
		{"https://github.com/user/repo.git#main", true},
		{"https://github.com/user/repo.git#v1.0.0", true},
		{"git@github.com:user/repo.git", true},
		{"git@github.com:user/repo.git#develop", true},
		{"https://github.com/user/repo", false},
		{"layers/base", false},
		{"", false},
		{"https://example.com/archive.tar.gz", false},
		{"file.git.bak", false}, // .git is a suffix of the base, not extension
	}
	for _, tt := range tests {
		got := IsGitSource(tt.input)
		if got != tt.want {
			t.Errorf("IsGitSource(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseGitSource(t *testing.T) {
	tests := []struct {
		input    string
		wantRepo string
		wantRef  string
	}{
		{"https://github.com/user/repo.git", "https://github.com/user/repo.git", ""},
		{"https://github.com/user/repo.git#main", "https://github.com/user/repo.git", "main"},
		{"https://github.com/user/repo.git#v1.0.0", "https://github.com/user/repo.git", "v1.0.0"},
		{"git@github.com:user/repo.git#develop", "git@github.com:user/repo.git", "develop"},
	}
	for _, tt := range tests {
		repo, ref := ParseGitSource(tt.input)
		if repo != tt.wantRepo {
			t.Errorf("ParseGitSource(%q) repo = %q, want %q", tt.input, repo, tt.wantRepo)
		}
		if ref != tt.wantRef {
			t.Errorf("ParseGitSource(%q) ref = %q, want %q", tt.input, ref, tt.wantRef)
		}
	}
}

func TestIsArchiveSource(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com/layer.tar.gz", true},
		{"https://example.com/layer.tgz", true},
		{"https://example.com/layer.tar.bz2", true},
		{"https://example.com/layer.tar.xz", true},
		{"https://example.com/layer.zip", true},
		// With query string and fragment
		{"https://example.com/layer.tar.gz?token=abc", true},
		{"https://example.com/layer.zip#subdir", true},
		// Non-archives
		{"https://example.com/layer.yaml", false},
		{"https://github.com/user/repo.git", false},
		{"layers/base", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsArchiveSource(tt.input)
		if got != tt.want {
			t.Errorf("IsArchiveSource(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com/layer", true},
		{"http://example.com/layer", true},
		{"https://github.com/user/repo.git", true},
		{"layers/base", false},
		{"/absolute/path", false},
		{"", false},
		{"ftp://example.com", false},
	}
	for _, tt := range tests {
		got := IsURL(tt.input)
		if got != tt.want {
			t.Errorf("IsURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCollectLayerPaths(t *testing.T) {
	layer := &Layer{
		Steps: []Step{
			{
				Action:     "file-create",
				FileCreate: &FileCreateStep{LayerPath: "files/config.toml"},
			},
			{
				Action: "run",
				Run:    &RunStep{ScriptPath: "scripts/setup.sh"},
			},
			{
				Action:         "systemd-service",
				SystemdService: &SystemdServiceStep{LayerPath: "units/myapp.service"},
			},
			{
				Action:   "pacman-add",
				PacmanAdd: &PacmanAddStep{Packages: []string{"base"}},
			},
			{
				Action:       "systemd-timer",
				SystemdTimer: &SystemdTimerStep{LayerPath: "units/backup.timer"},
			},
		},
	}

	paths := CollectLayerPaths(layer)
	want := []string{"files/config.toml", "scripts/setup.sh", "units/myapp.service", "units/backup.timer"}

	if len(paths) != len(want) {
		t.Fatalf("CollectLayerPaths() returned %d paths, want %d: %v", len(paths), len(want), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestCollectLayerPaths_Empty(t *testing.T) {
	layer := &Layer{
		Steps: []Step{
			{Action: "pacman-add", PacmanAdd: &PacmanAddStep{Packages: []string{"vim"}}},
			{Action: "system-hostname", SystemHostname: &SystemHostnameStep{Hostname: "test"}},
		},
	}
	paths := CollectLayerPaths(layer)
	if len(paths) != 0 {
		t.Errorf("expected no paths, got %v", paths)
	}
}
