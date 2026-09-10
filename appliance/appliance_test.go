package appliance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/telemetryos/starforge/platform"
	"github.com/telemetryos/starforge/worker"
)

type temporarilyUnavailableWriter struct {
	remainingFailures int
	buffer            bytes.Buffer
}

func (w *temporarilyUnavailableWriter) Write(data []byte) (int, error) {
	if w.remainingFailures > 0 {
		w.remainingFailures--
		return 0, syscall.EAGAIN
	}
	return w.buffer.Write(data)
}

func TestNormalizedArchivePath(t *testing.T) {
	tests := []struct {
		name    string
		source  sourceArchive
		want    string
		include bool
		wantErr bool
	}{
		{name: "root.x86_64/etc/pacman.conf", source: sourceArchive{StripPrefix: "root.x86_64/"}, want: "etc/pacman.conf", include: true},
		{name: "other/etc/passwd", source: sourceArchive{StripPrefix: "root.x86_64/"}},
		{name: ".PKGINFO", source: sourceArchive{Metadata: true}},
		{name: "../../etc/shadow", wantErr: true},
	}
	for _, test := range tests {
		got, include, err := normalizedArchivePath(test.name, test.source)
		if (err != nil) != test.wantErr {
			t.Errorf("normalizedArchivePath(%q) error = %v", test.name, err)
		}
		if got != test.want || include != test.include {
			t.Errorf("normalizedArchivePath(%q) = (%q, %t), want (%q, %t)", test.name, got, include, test.want, test.include)
		}
	}
}

func TestCreateInitramfsContainsExactWorkerAndJob(t *testing.T) {
	directory := t.TempDir()
	workerPath := filepath.Join(directory, "worker")
	workerData := []byte("worker-binary")
	if err := os.WriteFile(workerPath, workerData, 0o755); err != nil {
		t.Fatal(err)
	}
	job := worker.Job{
		Protocol: worker.ProtocolVersion,
		Target:   "device-arm64",
		Platform: platform.Spec{
			Architecture: platform.ArchitectureAArch64,
			Machine:      platform.MachineGenericUEFI,
			Distribution: platform.DistributionArchLinuxARM,
		},
		Environment: map[string]string{"EDGE_RELEASE_CHANNEL": "beta"},
	}
	path := filepath.Join(directory, "initramfs.gz")
	if err := createInitramfs(path, workerPath, job); err != nil {
		t.Fatal(err)
	}
	entries := readNewc(t, path)
	if string(entries["init"]) != string(workerData) {
		t.Fatalf("init = %q", entries["init"])
	}
	var actual worker.Job
	if err := json.Unmarshal(entries["starforge-job.json"], &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Target != job.Target || actual.Platform.Resolve() != job.Platform.Resolve() {
		t.Fatalf("job = %#v, want %#v", actual, job)
	}
	if actual.Environment["EDGE_RELEASE_CHANNEL"] != "beta" {
		t.Fatalf("environment = %#v", actual.Environment)
	}
}

func TestInstalledWorkerCandidates(t *testing.T) {
	t.Setenv("STARFORGE_WORKER_ARM64", "")
	executable := filepath.Join(string(filepath.Separator), "opt", "starforge", "bin", "starforge")
	candidates := installedWorkerCandidates(executable, platform.Spec{Architecture: platform.ArchitectureAArch64})
	want := []string{
		filepath.Join(string(filepath.Separator), "opt", "starforge", "bin", "starforge-worker-linux-arm64"),
		filepath.Join(string(filepath.Separator), "opt", "starforge", "bin", "starforge-workers", "starforge-worker-linux-arm64"),
		filepath.Join(string(filepath.Separator), "opt", "starforge", "lib", "starforge", "starforge-worker-linux-arm64"),
	}
	if fmt.Sprint(candidates) != fmt.Sprint(want) {
		t.Fatalf("candidates = %v, want %v", candidates, want)
	}
}

func TestInstalledWorkerOverride(t *testing.T) {
	t.Setenv("STARFORGE_WORKER_AMD64", "/workers/starforge-amd64")
	candidates := installedWorkerCandidates("/usr/bin/starforge", platform.Spec{Architecture: platform.ArchitectureX86_64})
	if len(candidates) != 1 || candidates[0] != "/workers/starforge-amd64" {
		t.Fatalf("candidates = %v", candidates)
	}
}

func TestRetryingWriterHandlesTemporarilyUnavailableOutput(t *testing.T) {
	destination := &temporarilyUnavailableWriter{remainingFailures: 2}
	writer := retryingWriter{ctx: context.Background(), writer: destination}

	if _, err := writer.Write([]byte("worker output")); err != nil {
		t.Fatal(err)
	}
	if destination.buffer.String() != "worker output" {
		t.Fatalf("output = %q", destination.buffer.String())
	}
}

func TestBuildLaunchCommandUsesDeterministicVirtioOrder(t *testing.T) {
	directory := t.TempDir()
	paths := []string{"root", "kernel", "initramfs", "project", "work", "output"}
	for _, name := range paths {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, args, err := buildLaunchCommand(launchRequest{
		Platform:    platform.Spec{Architecture: platform.ArchitectureX86_64},
		RootDisk:    filepath.Join(directory, "root"),
		Kernel:      filepath.Join(directory, "kernel"),
		Initramfs:   filepath.Join(directory, "initramfs"),
		ProjectDisk: filepath.Join(directory, "project"),
		WorkDisk:    filepath.Join(directory, "work"),
		OutputDisk:  filepath.Join(directory, "output"),
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	positions := []int{
		strings.Index(joined, "id=root"),
		strings.Index(joined, "id=project"),
		strings.Index(joined, "id=work"),
		strings.Index(joined, "id=output"),
	}
	for index, position := range positions {
		if position < 0 || index > 0 && position <= positions[index-1] {
			t.Fatalf("disk order is not root, project, work, output: %s", joined)
		}
	}
	if !strings.Contains(joined, "id=root") || !strings.Contains(joined, "format=qcow2") || !strings.Contains(joined, "readonly=on") {
		t.Fatalf("missing ephemeral root overlay or read-only project: %s", joined)
	}
}

func readNewc(t *testing.T, path string) map[string][]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := map[string][]byte{}
	for {
		header := make([]byte, 110)
		if _, err := io.ReadFull(reader, header); err != nil {
			t.Fatal(err)
		}
		if string(header[:6]) != "070701" {
			t.Fatalf("bad cpio magic %q", header[:6])
		}
		nameSize := parseHex(t, header[94:102])
		fileSize := parseHex(t, header[54:62])
		nameData := make([]byte, nameSize)
		if _, err := io.ReadFull(reader, nameData); err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(string(nameData), "\x00")
		discardPadding(t, reader, 110+nameSize)
		data := make([]byte, fileSize)
		if _, err := io.ReadFull(reader, data); err != nil {
			t.Fatal(err)
		}
		discardPadding(t, reader, fileSize)
		if name == "TRAILER!!!" {
			return entries
		}
		entries[name] = data
	}
}

func parseHex(t *testing.T, data []byte) int {
	t.Helper()
	var value int
	if _, err := fmt.Sscanf(string(data), "%x", &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func discardPadding(t *testing.T, reader io.Reader, length int) {
	t.Helper()
	padding := (4 - length%4) % 4
	if _, err := io.CopyN(io.Discard, reader, int64(padding)); err != nil {
		t.Fatal(err)
	}
}
