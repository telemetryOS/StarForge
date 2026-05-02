package corona

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

func Inspect(ctx context.Context, artifactPath string) (Info, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return Info{}, fmt.Errorf("open artifact: %w", err)
	}
	defer f.Close()
	return scanArtifact(ctx, f, nil)
}

func scanArtifact(ctx context.Context, f *os.File, apply func(frameHeader, []byte) error) (Info, error) {
	info, err := f.Stat()
	if err != nil {
		return Info{}, fmt.Errorf("stat artifact: %w", err)
	}
	trailer, err := readFileTrailer(f, info.Size())
	if err != nil {
		return Info{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Info{}, fmt.Errorf("seek artifact: %w", err)
	}
	header, err := readFileHeader(f)
	if err != nil {
		return Info{}, err
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return Info{}, fmt.Errorf("create zstd reader: %w", err)
	}
	defer dec.Close()

	h := newAllocatedHasher()
	var frameCount uint64
	var usefulBytes uint64
	var storedBytes uint64
	var expectedOffset uint64
	trailerStart := uint64(info.Size() - fileTrailerLen)
	for expectedOffset < header.imageSize {
		if err := ctx.Err(); err != nil {
			return Info{}, err
		}
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return Info{}, fmt.Errorf("seek artifact: %w", err)
		}
		if uint64(pos)+frameHeaderLen > trailerStart {
			return Info{}, errors.New("corona: frame data ended before image was complete")
		}
		frame, err := readFrameHeader(f)
		if err != nil {
			return Info{}, err
		}
		if err := validateFrame(frame, header.imageSize, expectedOffset, trailerStart-uint64(pos)-frameHeaderLen); err != nil {
			return Info{}, err
		}
		var raw []byte
		switch frame.flags {
		case frameZero:
			if err := h.WriteZeros(frame.uncompressedSize); err != nil {
				return Info{}, err
			}
		case frameSkip:
		case frameZstd:
			payload := make([]byte, frame.compressedSize)
			if _, err := io.ReadFull(f, payload); err != nil {
				return Info{}, fmt.Errorf("read frame payload: %w", err)
			}
			raw, err = dec.DecodeAll(payload, nil)
			if err != nil {
				return Info{}, fmt.Errorf("decompress chunk at %d: %w", frame.targetOffset, err)
			}
			if uint64(len(raw)) != frame.uncompressedSize {
				return Info{}, fmt.Errorf("corona: chunk at %d size mismatch", frame.targetOffset)
			}
			if got := crc32.Checksum(raw, crc32cTable); got != frame.crc32c {
				return Info{}, fmt.Errorf("corona: chunk at %d crc32c mismatch", frame.targetOffset)
			}
			if _, err := h.Write(raw); err != nil {
				return Info{}, err
			}
		default:
			return Info{}, fmt.Errorf("corona: invalid frame flags %d", frame.flags)
		}
		if apply != nil {
			if err := apply(frame, raw); err != nil {
				return Info{}, err
			}
		}
		frameCount++
		usefulBytes += frame.uncompressedSize
		storedBytes += frame.compressedSize
		expectedOffset += frame.uncompressedSize
	}
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return Info{}, fmt.Errorf("seek artifact: %w", err)
	}
	if uint64(pos) != trailerStart {
		return Info{}, fmt.Errorf("corona: trailing frame bytes before trailer: at %d, want %d", pos, trailerStart)
	}
	if frameCount != trailer.frameCount {
		return Info{}, fmt.Errorf("corona: frame count mismatch: trailer=%d frames=%d", trailer.frameCount, frameCount)
	}
	if usefulBytes != trailer.usefulBytes {
		return Info{}, fmt.Errorf("corona: useful bytes mismatch: trailer=%d frames=%d", trailer.usefulBytes, usefulBytes)
	}
	if storedBytes != trailer.storedBytes {
		return Info{}, fmt.Errorf("corona: stored bytes mismatch: trailer=%d frames=%d", trailer.storedBytes, storedBytes)
	}
	allocatedSHA := h.Sum()
	if !bytes.Equal(allocatedSHA, trailer.allocatedSHA256) {
		return Info{}, errors.New("corona: allocated sha256 mismatch")
	}
	return Info{
		Version:         Version,
		ImageSize:       header.imageSize,
		ChunkSize:       header.chunkSize,
		UsefulBytes:     usefulBytes,
		StoredBytes:     storedBytes,
		OperationNum:    int(frameCount),
		FSType:          header.fsType,
		FSVersion:       header.fsVersion,
		FSBlockSize:     header.fsBlockSize,
		AllocatedSHA256: hex.EncodeToString(allocatedSHA),
	}, nil
}

func validateFrame(frame frameHeader, imageSize, expectedOffset, remainingPayloadBytes uint64) error {
	if frame.targetOffset != expectedOffset {
		return fmt.Errorf("corona: frame starts at %d, expected %d", frame.targetOffset, expectedOffset)
	}
	if frame.uncompressedSize == 0 {
		return errors.New("corona: frame has zero size")
	}
	end := frame.targetOffset + frame.uncompressedSize
	if end < frame.targetOffset || end > imageSize {
		return fmt.Errorf("corona: frame at %d exceeds image size", frame.targetOffset)
	}
	switch frame.flags {
	case frameZero, frameSkip:
		if frame.compressedSize != 0 {
			return fmt.Errorf("corona: frame at %d has unexpected compressed data", frame.targetOffset)
		}
	case frameZstd:
		if frame.compressedSize == 0 {
			return fmt.Errorf("corona: zstd frame at %d has no compressed data", frame.targetOffset)
		}
		if frame.compressedSize > remainingPayloadBytes {
			return fmt.Errorf("corona: zstd frame at %d exceeds artifact size", frame.targetOffset)
		}
	default:
		return fmt.Errorf("corona: invalid frame flags %d", frame.flags)
	}
	return nil
}
