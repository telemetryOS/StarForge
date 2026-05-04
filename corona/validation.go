package corona

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
)

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
		if frame.crc32c != 0 {
			return fmt.Errorf("corona: frame at %d has unexpected crc32c", frame.targetOffset)
		}
	case frameZstd:
		if frame.compressedSize == 0 {
			return fmt.Errorf("corona: zstd frame at %d has no compressed data", frame.targetOffset)
		}
		if frame.compressedSize > remainingPayloadBytes {
			return fmt.Errorf("corona: zstd frame at %d exceeds corona size", frame.targetOffset)
		}
	default:
		return fmt.Errorf("corona: invalid frame flags %d", frame.flags)
	}
	return nil
}

type writeSummary struct {
	header      fileHeader
	trailer     fileTrailer
	frameCount  uint64
	usefulBytes uint64
	storedBytes uint64
	hash        allocatedHasher
}

func (s *writeSummary) addFrame(frame frameHeader) {
	s.frameCount++
	s.usefulBytes += frame.uncompressedSize
	s.storedBytes += frame.compressedSize
}

func (s *writeSummary) info() (Info, error) {
	if s.frameCount != s.trailer.frameCount {
		return Info{}, fmt.Errorf("corona: frame count mismatch: trailer=%d frames=%d", s.trailer.frameCount, s.frameCount)
	}
	if s.usefulBytes != s.trailer.usefulBytes {
		return Info{}, fmt.Errorf("corona: useful bytes mismatch: trailer=%d frames=%d", s.trailer.usefulBytes, s.usefulBytes)
	}
	if s.storedBytes != s.trailer.storedBytes {
		return Info{}, fmt.Errorf("corona: stored bytes mismatch: trailer=%d frames=%d", s.trailer.storedBytes, s.storedBytes)
	}
	allocatedSHA := s.hash.Sum()
	if !bytes.Equal(allocatedSHA, s.trailer.allocatedSHA256) {
		return Info{}, errors.New("corona: allocated sha256 mismatch")
	}
	return Info{
		Version:         Version,
		ImageSize:       s.header.imageSize,
		ChunkSize:       s.header.chunkSize,
		UsefulBytes:     s.usefulBytes,
		StoredBytes:     s.storedBytes,
		OperationNum:    int(s.frameCount),
		FSType:          s.header.fsType,
		FSVersion:       s.header.fsVersion,
		FSBlockSize:     s.header.fsBlockSize,
		AllocatedSHA256: hex.EncodeToString(allocatedSHA),
	}, nil
}
