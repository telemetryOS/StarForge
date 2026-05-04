package corona

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	frameZero uint8 = 1
	frameZstd uint8 = 2
	frameSkip uint8 = 3

	fsUnknown uint8 = 0
	fsExt     uint8 = 1
	fsFAT     uint8 = 2

	fileHeaderLen  = 8 + 2 + 8 + 8 + 1 + 2 + 8
	frameHeaderLen = 1 + 8 + 8 + 8 + 4
	fileTrailerLen = 8 + 8 + 8 + 8 + sha256.Size
)

var (
	magic           = [8]byte{'C', 'O', 'R', 'O', 'N', 'A', 0, 2}
	trailerMagic    = [8]byte{'C', 'F', 'S', 'H', 'A', '2', '5', '6'}
	errShortHeader  = errors.New("corona: short header")
	errInvalidMagic = errors.New("corona: invalid magic")
)

type fileHeader struct {
	imageSize   uint64
	chunkSize   uint64
	fsType      uint8
	fsVersion   uint16
	fsBlockSize uint64
}

type frameHeader struct {
	flags            uint8
	targetOffset     uint64
	uncompressedSize uint64
	compressedSize   uint64
	crc32c           uint32
}

type fileTrailer struct {
	frameCount      uint64
	usefulBytes     uint64
	storedBytes     uint64
	allocatedSHA256 []byte
}

func writeFileHeader(w io.Writer, header fileHeader) error {
	if _, err := w.Write(magic[:]); err != nil {
		return fmt.Errorf("write magic: %w", err)
	}
	var buf [29]byte
	binary.BigEndian.PutUint16(buf[:2], Version)
	binary.BigEndian.PutUint64(buf[2:10], header.imageSize)
	binary.BigEndian.PutUint64(buf[10:18], header.chunkSize)
	buf[18] = header.fsType
	binary.BigEndian.PutUint16(buf[19:21], header.fsVersion)
	binary.BigEndian.PutUint64(buf[21:29], header.fsBlockSize)
	if _, err := w.Write(buf[:]); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	return nil
}

func readFileHeader(r io.Reader) (fileHeader, error) {
	var prefix [fileHeaderLen]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return fileHeader{}, fmt.Errorf("read header: %w", err)
	}
	if !bytes.Equal(prefix[:len(magic)], magic[:]) {
		return fileHeader{}, errInvalidMagic
	}
	version := binary.BigEndian.Uint16(prefix[len(magic):10])
	if version != Version {
		return fileHeader{}, fmt.Errorf("corona: unsupported version %d", version)
	}
	header := fileHeader{
		imageSize:   binary.BigEndian.Uint64(prefix[10:18]),
		chunkSize:   binary.BigEndian.Uint64(prefix[18:26]),
		fsType:      prefix[26],
		fsVersion:   binary.BigEndian.Uint16(prefix[27:29]),
		fsBlockSize: binary.BigEndian.Uint64(prefix[29:37]),
	}
	if header.imageSize == 0 {
		return fileHeader{}, errors.New("corona: image size must be set")
	}
	if header.chunkSize < 4096 {
		return fileHeader{}, fmt.Errorf("corona: invalid chunk size %d", header.chunkSize)
	}
	switch header.fsType {
	case fsUnknown, fsExt, fsFAT:
	default:
		return fileHeader{}, fmt.Errorf("corona: invalid filesystem type %d", header.fsType)
	}
	return header, nil
}

func writeFrame(w io.Writer, header frameHeader, payload []byte) error {
	var buf [frameHeaderLen]byte
	buf[0] = header.flags
	binary.BigEndian.PutUint64(buf[1:9], header.targetOffset)
	binary.BigEndian.PutUint64(buf[9:17], header.uncompressedSize)
	binary.BigEndian.PutUint64(buf[17:25], header.compressedSize)
	binary.BigEndian.PutUint32(buf[25:29], header.crc32c)
	if _, err := w.Write(buf[:]); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("write frame payload: %w", err)
		}
	}
	return nil
}

func readFrameHeader(r io.Reader) (frameHeader, error) {
	var buf [frameHeaderLen]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return frameHeader{}, fmt.Errorf("read frame header: %w", err)
	}
	return frameHeader{
		flags:            buf[0],
		targetOffset:     binary.BigEndian.Uint64(buf[1:9]),
		uncompressedSize: binary.BigEndian.Uint64(buf[9:17]),
		compressedSize:   binary.BigEndian.Uint64(buf[17:25]),
		crc32c:           binary.BigEndian.Uint32(buf[25:29]),
	}, nil
}

func writeFileTrailer(w io.Writer, trailer fileTrailer) error {
	if len(trailer.allocatedSHA256) != sha256.Size {
		return fmt.Errorf("corona: allocated sha256 length %d, want %d", len(trailer.allocatedSHA256), sha256.Size)
	}
	var buf [fileTrailerLen]byte
	copy(buf[:len(trailerMagic)], trailerMagic[:])
	binary.BigEndian.PutUint64(buf[8:16], trailer.frameCount)
	binary.BigEndian.PutUint64(buf[16:24], trailer.usefulBytes)
	binary.BigEndian.PutUint64(buf[24:32], trailer.storedBytes)
	copy(buf[32:], trailer.allocatedSHA256)
	if _, err := w.Write(buf[:]); err != nil {
		return fmt.Errorf("write trailer: %w", err)
	}
	return nil
}

func readFileTrailer(f *os.File, size int64) (fileTrailer, error) {
	if size < fileHeaderLen+fileTrailerLen {
		return fileTrailer{}, errShortHeader
	}
	var buf [fileTrailerLen]byte
	if _, err := f.ReadAt(buf[:], size-fileTrailerLen); err != nil {
		return fileTrailer{}, fmt.Errorf("read trailer: %w", err)
	}
	return parseFileTrailer(buf[:])
}

func readFileTrailerFromReader(r io.Reader) (fileTrailer, error) {
	var buf [fileTrailerLen]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return fileTrailer{}, fmt.Errorf("read trailer: %w", err)
	}
	return parseFileTrailer(buf[:])
}

func parseFileTrailer(buf []byte) (fileTrailer, error) {
	if !bytes.Equal(buf[:len(trailerMagic)], trailerMagic[:]) {
		return fileTrailer{}, errors.New("corona: invalid trailer")
	}
	allocatedSHA := append([]byte(nil), buf[32:]...)
	return fileTrailer{
		frameCount:      binary.BigEndian.Uint64(buf[8:16]),
		usefulBytes:     binary.BigEndian.Uint64(buf[16:24]),
		storedBytes:     binary.BigEndian.Uint64(buf[24:32]),
		allocatedSHA256: allocatedSHA,
	}, nil
}
