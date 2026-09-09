// Package artifact manages immutable full-disk build artifacts and receipts.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/telemetryos/starforge/platform"
)

const receiptVersion = 1

// State identifies the ownership state of an artifact.
type State string

const (
	// StateCreating means the build executor owns and may modify the disk.
	StateCreating State = "creating"
	// StateSealed means the disk is immutable and available to consumers.
	StateSealed State = "sealed"
)

// Receipt records the identity and integrity of a full-disk build artifact.
type Receipt struct {
	Version    int           `json:"version"`
	Target     string        `json:"target"`
	Platform   platform.Spec `json:"platform"`
	State      State         `json:"state"`
	Disk       string        `json:"disk"`
	SourceHash string        `json:"sourceHash"`
	SHA256     string        `json:"sha256,omitempty"`
	Size       int64         `json:"size,omitempty"`
	CreatedAt  time.Time     `json:"createdAt"`
	SealedAt   *time.Time    `json:"sealedAt,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// Store manages a target platform's artifact directory.
type Store struct {
	root     string
	target   string
	platform platform.Spec
}

// NewStore creates an artifact store scoped to one target platform.
func NewStore(buildDir, target string, targetPlatform platform.Spec) *Store {
	resolved := targetPlatform.Resolve()
	root := filepath.Join(
		buildDir,
		"artifacts",
		string(resolved.Architecture),
		string(resolved.Machine),
		string(resolved.Distribution),
	)
	return &Store{root: root, target: target, platform: resolved}
}

// Root returns the store's platform-scoped artifact directory.
func (s *Store) Root() string {
	return s.root
}

// DiskPath returns the path of the sealed raw disk image.
func (s *Store) DiskPath() string {
	return filepath.Join(s.root, "disk.raw")
}

// CreatingDiskPath returns the private path owned by an active build.
func (s *Store) CreatingDiskPath() string {
	return filepath.Join(s.root, "disk.raw.creating")
}

// Begin records a creating receipt and returns the private disk path.
func (s *Store) Begin(sourceHash string) (*Receipt, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("creating artifact directory: %w", err)
	}
	if err := os.Remove(s.CreatingDiskPath()); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing stale creating disk: %w", err)
	}
	receipt := &Receipt{
		Version:    receiptVersion,
		Target:     s.target,
		Platform:   s.platform,
		State:      StateCreating,
		Disk:       filepath.Base(s.CreatingDiskPath()),
		SourceHash: sourceHash,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.save(receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

// Abort leaves the receipt in creating state with diagnostic failure text.
func (s *Store) Abort(receipt *Receipt, buildErr error) error {
	receipt.Error = buildErr.Error()
	return s.save(receipt)
}

// Seal atomically publishes a completed disk and its integrity metadata.
func (s *Store) Seal(receipt *Receipt) (*Receipt, error) {
	if receipt.State != StateCreating {
		return nil, fmt.Errorf("cannot seal artifact in state %q", receipt.State)
	}
	checksum, size, err := hashFile(s.CreatingDiskPath())
	if err != nil {
		return nil, fmt.Errorf("hashing artifact: %w", err)
	}
	if err := os.Rename(s.CreatingDiskPath(), s.DiskPath()); err != nil {
		return nil, fmt.Errorf("publishing artifact: %w", err)
	}
	now := time.Now().UTC()
	receipt.State = StateSealed
	receipt.Disk = filepath.Base(s.DiskPath())
	receipt.SHA256 = checksum
	receipt.Size = size
	receipt.SealedAt = &now
	receipt.Error = ""
	if err := s.save(receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

// Load returns the current receipt after validating its identity and disk.
func (s *Store) Load() (*Receipt, error) {
	data, err := os.ReadFile(s.receiptPath())
	if err != nil {
		return nil, fmt.Errorf("reading artifact receipt: %w", err)
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, fmt.Errorf("parsing artifact receipt: %w", err)
	}
	if receipt.Version != receiptVersion {
		return nil, fmt.Errorf("unsupported artifact receipt version %d", receipt.Version)
	}
	if receipt.Target != s.target || receipt.Platform.Resolve() != s.platform {
		return nil, fmt.Errorf("artifact receipt identity does not match target %q platform %s", s.target, s.platform.Identity())
	}
	if receipt.State == StateSealed {
		if _, err := os.Stat(s.DiskPath()); err != nil {
			return nil, fmt.Errorf("sealed artifact disk is unavailable: %w", err)
		}
	}
	return &receipt, nil
}

// Verify recalculates the sealed disk checksum and size.
func (s *Store) Verify(receipt *Receipt) error {
	if receipt.State != StateSealed {
		return fmt.Errorf("artifact is not sealed")
	}
	checksum, size, err := hashFile(s.DiskPath())
	if err != nil {
		return err
	}
	if size != receipt.Size {
		return fmt.Errorf("artifact size %d does not match receipt %d", size, receipt.Size)
	}
	if checksum != receipt.SHA256 {
		return fmt.Errorf("artifact checksum %s does not match receipt %s", checksum, receipt.SHA256)
	}
	return nil
}

func (s *Store) receiptPath() string {
	return filepath.Join(s.root, "artifact.json")
}

func (s *Store) save(receipt *Receipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling artifact receipt: %w", err)
	}
	tmp := s.receiptPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing artifact receipt: %w", err)
	}
	if err := os.Rename(tmp, s.receiptPath()); err != nil {
		return fmt.Errorf("publishing artifact receipt: %w", err)
	}
	return nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
