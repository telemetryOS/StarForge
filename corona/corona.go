package corona

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Version          uint16 = 2
	DefaultChunkSize        = 8 << 20
)

const (
	FSTypeUnknown = fsUnknown
	FSTypeExt     = fsExt
	FSTypeFAT     = fsFAT
)

type createOptions struct {
	SourcePath string
	CoronaPath string
	ChunkSize  int64
	Workers    int
	Progress   func(Progress)
}

type flashOptions struct {
	CoronaPath  string
	TargetPath  string
	Workers     int
	WriteOrder  WriteOrder
	ZeroSkipped bool
	Progress    func(Progress)
}

type flashImageOptions struct {
	ImagePath   string
	TargetPath  string
	ChunkSize   int64
	Workers     int
	WriteOrder  WriteOrder
	ZeroSkipped bool
	Progress    func(Progress)
}

type captureImageOptions struct {
	SourcePath string
	ImagePath  string
	ChunkSize  int64
	Workers    int
	WriteOrder WriteOrder
	Progress   func(Progress)
}

type extractImageOptions struct {
	CoronaPath string
	ImagePath  string
	Workers    int
	WriteOrder WriteOrder
	Progress   func(Progress)
}

type Options struct {
	SourceType  Type
	TargetType  Type
	ChunkSize   int64
	Workers     int
	WriteOrder  WriteOrder
	ZeroSkipped bool
	Progress    func(Progress)
}

type Progress struct {
	ProcessedBytes uint64
	TotalBytes     uint64
	Percent        int
}

type WriteOrder string

const (
	WriteOrderSequential WriteOrder = "sequential"
	WriteOrderStriped    WriteOrder = "striped"
)

type Info struct {
	Version         uint16
	ImageSize       uint64
	ChunkSize       uint64
	UsefulBytes     uint64
	StoredBytes     uint64
	OperationNum    int
	FSType          uint8
	FSVersion       uint16
	FSBlockSize     uint64
	AllocatedSHA256 string
}

type pathKind uint8

const (
	pathImage pathKind = iota + 1
	pathDevice
	pathCorona
)

type Type string

const (
	TypeAuto   Type = ""
	TypeImage  Type = "image"
	TypeDevice Type = "device"
	TypeCorona Type = "corona"
)

func Convert(ctx context.Context, sourcePath, targetPath string, opts Options) error {
	srcKind, err := resolveSourceKind(sourcePath, opts.SourceType)
	if err != nil {
		return err
	}
	dstKind, err := resolveTargetKind(targetPath, opts.TargetType)
	if err != nil {
		return err
	}
	if srcKind == dstKind {
		return fmt.Errorf("corona: cannot convert %s to %s", srcKind, dstKind)
	}
	switch srcKind {
	case pathImage, pathDevice:
		switch dstKind {
		case pathCorona:
			return create(ctx, createOptions{
				SourcePath: sourcePath,
				CoronaPath: targetPath,
				ChunkSize:  opts.ChunkSize,
				Workers:    opts.Workers,
				Progress:   opts.Progress,
			})
		case pathImage:
			return captureImage(ctx, captureImageOptions{
				SourcePath: sourcePath,
				ImagePath:  targetPath,
				ChunkSize:  opts.ChunkSize,
				Workers:    opts.Workers,
				WriteOrder: opts.WriteOrder,
				Progress:   opts.Progress,
			})
		case pathDevice:
			return flashImage(ctx, flashImageOptions{
				ImagePath:   sourcePath,
				TargetPath:  targetPath,
				ChunkSize:   opts.ChunkSize,
				Workers:     opts.Workers,
				WriteOrder:  opts.WriteOrder,
				ZeroSkipped: opts.ZeroSkipped,
				Progress:    opts.Progress,
			})
		}
	case pathCorona:
		switch dstKind {
		case pathImage:
			return extractImage(ctx, extractImageOptions{
				CoronaPath: sourcePath,
				ImagePath:  targetPath,
				Workers:    opts.Workers,
				WriteOrder: opts.WriteOrder,
				Progress:   opts.Progress,
			})
		case pathDevice:
			return flash(ctx, flashOptions{
				CoronaPath:  sourcePath,
				TargetPath:  targetPath,
				Workers:     opts.Workers,
				WriteOrder:  opts.WriteOrder,
				ZeroSkipped: opts.ZeroSkipped,
				Progress:    opts.Progress,
			})
		}
	}
	return fmt.Errorf("corona: unsupported conversion %s to %s", srcKind, dstKind)
}

func resolveSourceKind(path string, typ Type) (pathKind, error) {
	if typ == TypeAuto {
		return detectExistingPathKind(path)
	}
	if path == "" {
		return 0, fmt.Errorf("corona: path is required")
	}
	if _, err := os.Stat(path); err != nil {
		return 0, fmt.Errorf("stat source: %w", err)
	}
	return typeKind(typ)
}

func resolveTargetKind(path string, typ Type) (pathKind, error) {
	if typ == TypeAuto {
		return detectTargetPathKind(path)
	}
	if path == "" {
		return 0, fmt.Errorf("corona: target path is required")
	}
	if typ == TypeDevice {
		info, err := os.Stat(path)
		if err != nil {
			return 0, fmt.Errorf("stat target: %w", err)
		}
		if !isBlockDevice(info.Mode()) {
			return 0, fmt.Errorf("corona: target must be a block device: %s", path)
		}
	}
	return typeKind(typ)
}

func typeKind(typ Type) (pathKind, error) {
	switch typ {
	case TypeImage:
		return pathImage, nil
	case TypeDevice:
		return pathDevice, nil
	case TypeCorona:
		return pathCorona, nil
	default:
		return 0, fmt.Errorf("corona: unknown path type %q", typ)
	}
}

func detectExistingPathKind(path string) (pathKind, error) {
	if path == "" {
		return 0, fmt.Errorf("corona: path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat source: %w", err)
	}
	if isBlockDevice(info.Mode()) {
		return pathDevice, nil
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("corona: unsupported source type %s", info.Mode().Type())
	}
	return regularFileKind(path), nil
}

func detectTargetPathKind(path string) (pathKind, error) {
	if path == "" {
		return 0, fmt.Errorf("corona: target path is required")
	}
	info, err := os.Stat(path)
	if err == nil {
		if isBlockDevice(info.Mode()) {
			return pathDevice, nil
		}
		if !info.Mode().IsRegular() {
			return 0, fmt.Errorf("corona: unsupported target type %s", info.Mode().Type())
		}
		return regularFileKind(path), nil
	}
	if !os.IsNotExist(err) {
		return 0, fmt.Errorf("stat target: %w", err)
	}
	return regularFileKind(path), nil
}

func regularFileKind(path string) pathKind {
	if strings.EqualFold(filepath.Ext(path), ".corona") {
		return pathCorona
	}
	return pathImage
}

func (k pathKind) String() string {
	switch k {
	case pathImage:
		return "image"
	case pathDevice:
		return "device"
	case pathCorona:
		return "corona"
	default:
		return "unknown"
	}
}
