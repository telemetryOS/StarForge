package corona

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectRejectsBadAllocatedSHA(t *testing.T) {
	artifact := createFixtureArtifact(t)
	f, err := os.OpenFile(artifact, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{0x99}, -1); err == nil {
		t.Fatal("expected negative write offset to fail")
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{0x99}, info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Inspect(context.Background(), artifact)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("Inspect err = %v, want sha mismatch", err)
	}
}

func TestWriteRejectsCorruptChunkCRC(t *testing.T) {
	artifact := createFixtureArtifact(t)
	corruptFirstPayloadByte(t, artifact)

	target := filepath.Join(t.TempDir(), "target.blk")
	if err := os.WriteFile(target, bytes.Repeat([]byte{0xff}, 8192+4096), 0644); err != nil {
		t.Fatal(err)
	}
	err := Write(context.Background(), WriteOptions{ArtifactPath: artifact, TargetPath: target})
	if err == nil || (!strings.Contains(err.Error(), "crc32c mismatch") && !strings.Contains(err.Error(), "decompress chunk")) {
		t.Fatalf("Flash err = %v, want corrupt chunk failure", err)
	}
}

func TestValidationRejectsOutOfOrderFrame(t *testing.T) {
	err := validateFrame(frameHeader{
		flags:            frameZero,
		targetOffset:     2048,
		uncompressedSize: 4096,
	}, 8192, 4096, 0)
	if err == nil || !strings.Contains(err.Error(), "expected 4096") {
		t.Fatalf("validateFrame err = %v, want non-contiguous frame error", err)
	}
}

func corruptFirstPayloadByte(t *testing.T, artifact string) {
	t.Helper()
	f, err := os.OpenFile(artifact, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := readFileHeader(f); err != nil {
		t.Fatal(err)
	}
	for {
		frameStart, err := f.Seek(0, os.SEEK_CUR)
		if err != nil {
			t.Fatal(err)
		}
		header, err := readFrameHeader(f)
		if err != nil {
			t.Fatal(err)
		}
		payloadStart := frameStart + frameHeaderLen
		if header.flags == frameZstd {
			if _, err := f.WriteAt([]byte{0xff}, payloadStart); err != nil {
				t.Fatal(err)
			}
			return
		}
		if _, err := f.Seek(int64(header.compressedSize), os.SEEK_CUR); err != nil {
			t.Fatal(err)
		}
	}
}
