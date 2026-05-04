package corona

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type packJob struct {
	offset uint64
	size   uint64
	ops    []plannedOp
	result chan<- frameResult
}

type frameResult struct {
	frames []packedFrame
	err    error
}

type packedFrame struct {
	header  frameHeader
	payload []byte
}

type plannedOp struct {
	offset uint64
	size   uint64
	skip   bool
	data   []byte
}

type writerResult struct {
	frameCount  uint64
	usefulBytes uint64
	storedBytes uint64
	pending     *frameHeader
	err         error
}

func create(ctx context.Context, opts createOptions) error {
	sourcePath := opts.SourcePath
	if sourcePath == "" {
		return errors.New("corona: source path is required")
	}
	if opts.CoronaPath == "" {
		return errors.New("corona: corona path is required")
	}
	if err := rejectSamePath(sourcePath, opts.CoronaPath, "create corona"); err != nil {
		return err
	}
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize < 4096 {
		return fmt.Errorf("corona: chunk size %d is too small", chunkSize)
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()
	imageSize, err := sourceCapacity(src)
	if err != nil {
		return err
	}
	alloc := detectAllocationChecker(src, imageSize)
	if err := packCorona(ctx, src, opts.CoronaPath, imageSize, uint64(chunkSize), normalizeWorkers(opts.Workers), opts.Progress, alloc); err != nil {
		return err
	}
	reportProgress(opts.Progress, imageSize, imageSize)
	return nil
}

func packCorona(ctx context.Context, src *os.File, coronaPath string, imageSize, chunkSize uint64, workers int, progress func(Progress), alloc allocationChecker) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dst, err := os.Create(coronaPath)
	if err != nil {
		return fmt.Errorf("create corona: %w", err)
	}
	defer func() {
		if err := dst.Close(); retErr == nil && err != nil {
			retErr = fmt.Errorf("close corona: %w", err)
		}
	}()
	header := fileHeader{imageSize: imageSize, chunkSize: chunkSize}
	if alloc != nil {
		header = alloc.header()
		header.imageSize = imageSize
		header.chunkSize = chunkSize
	}
	if err := writeFileHeader(dst, header); err != nil {
		return err
	}

	jobs := make(chan packJob, workers)
	frameSlots := make(chan chan frameResult, workers)
	writerDone := make(chan writerResult, 1)
	go writeFrameSlots(ctx, dst, frameSlots, writerDone, progress, imageSize, cancel)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go packWorker(ctx, jobs, &wg)
	}

	h := sha256.New()
	var readErr error
readLoop:
	for _, chunk := range chunkJobs(imageSize, chunkSize) {
		if err := ctx.Err(); err != nil {
			readErr = err
			break
		}
		ops, err := plannedOpsForChunk(alloc, chunk.offset, chunk.size)
		if err != nil {
			readErr = err
			cancel()
			break
		}
		for i := range ops {
			if ops[i].skip {
				continue
			}
			data := make([]byte, ops[i].size)
			if _, err := src.ReadAt(data, int64(ops[i].offset)); err != nil && !errors.Is(err, io.EOF) {
				readErr = fmt.Errorf("read image at %d: %w", ops[i].offset, err)
				cancel()
				break
			}
			if _, err := h.Write(data); err != nil {
				readErr = err
				cancel()
				break
			}
			ops[i].data = data
		}
		if readErr != nil {
			break
		}
		result := make(chan frameResult, 1)
		select {
		case frameSlots <- result:
		case <-ctx.Done():
			readErr = ctx.Err()
			break readLoop
		}
		select {
		case jobs <- packJob{offset: chunk.offset, size: chunk.size, ops: ops, result: result}:
		case <-ctx.Done():
			readErr = ctx.Err()
			result <- frameResult{err: readErr}
			close(result)
			break readLoop
		}
	}
	close(jobs)
	wg.Wait()
	close(frameSlots)
	writer := <-writerDone
	if writer.err != nil {
		return writer.err
	}
	if readErr != nil {
		return readErr
	}
	if err := writeFileTrailer(dst, fileTrailer{
		frameCount:      writer.frameCount,
		usefulBytes:     writer.usefulBytes,
		storedBytes:     writer.storedBytes,
		allocatedSHA256: h.Sum(nil),
	}); err != nil {
		return err
	}
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("sync corona: %w", err)
	}
	return nil
}

func packWorker(ctx context.Context, jobs <-chan packJob, wg *sync.WaitGroup) {
	defer wg.Done()
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		for job := range jobs {
			job.result <- frameResult{err: fmt.Errorf("create zstd writer: %w", err)}
			close(job.result)
		}
		return
	}
	defer enc.Close()
	for job := range jobs {
		if err := ctx.Err(); err != nil {
			job.result <- frameResult{err: err}
			close(job.result)
			continue
		}
		res := frameResult{frames: make([]packedFrame, 0, len(job.ops))}
		for _, op := range job.ops {
			header := frameHeader{targetOffset: op.offset, uncompressedSize: op.size}
			if op.skip {
				header.flags = frameSkip
				res.frames = append(res.frames, packedFrame{header: header})
				continue
			}
			if allZero(op.data) {
				header.flags = frameZero
				res.frames = append(res.frames, packedFrame{header: header})
				continue
			}
			payload := enc.EncodeAll(op.data, nil)
			header.flags = frameZstd
			header.compressedSize = uint64(len(payload))
			header.crc32c = crc32.Checksum(op.data, crc32cTable)
			res.frames = append(res.frames, packedFrame{header: header, payload: payload})
		}
		job.result <- res
		close(job.result)
	}
}

func writeFrameSlots(ctx context.Context, dst io.Writer, frameSlots <-chan chan frameResult, done chan<- writerResult, progress func(Progress), imageSize uint64, cancel context.CancelFunc) {
	var out writerResult
	for resultCh := range frameSlots {
		if err := ctx.Err(); err != nil {
			out.err = err
			done <- out
			return
		}
		res := <-resultCh
		if res.err != nil {
			out.err = res.err
			cancel()
			done <- out
			return
		}
		for _, frame := range res.frames {
			if err := writePackedFrameCoalesced(dst, frame, &out); err != nil {
				out.err = err
				cancel()
				done <- out
				return
			}
			reportProgress(progress, out.usefulBytes, imageSize)
		}
	}
	if err := flushPendingFrame(dst, &out); err != nil {
		out.err = err
	}
	done <- out
}

func plannedOpsForChunk(alloc allocationChecker, offset, size uint64) ([]plannedOp, error) {
	ops := make([]plannedOp, 0, 1)
	for cursor := offset; cursor < offset+size; {
		remaining := offset + size - cursor
		plan := framePlan{offset: cursor, size: remaining}
		if alloc != nil {
			var err error
			plan, err = alloc.nextFramePlan(cursor, remaining)
			if err != nil {
				return nil, err
			}
		}
		ops = append(ops, plannedOp{offset: plan.offset, size: plan.size, skip: plan.skip})
		cursor += plan.size
	}
	return ops, nil
}

func writePackedFrameCoalesced(w io.Writer, frame packedFrame, out *writerResult) error {
	if frame.header.flags == frameSkip || frame.header.flags == frameZero {
		if out.pending != nil && out.pending.flags == frame.header.flags && out.pending.targetOffset+out.pending.uncompressedSize == frame.header.targetOffset {
			out.pending.uncompressedSize += frame.header.uncompressedSize
			out.usefulBytes += frame.header.uncompressedSize
			return nil
		}
		if err := flushPendingFrame(w, out); err != nil {
			return err
		}
		pending := frame.header
		out.pending = &pending
		out.usefulBytes += frame.header.uncompressedSize
		return nil
	}
	if err := flushPendingFrame(w, out); err != nil {
		return err
	}
	if err := writeFrame(w, frame.header, frame.payload); err != nil {
		return err
	}
	out.frameCount++
	out.usefulBytes += frame.header.uncompressedSize
	out.storedBytes += frame.header.compressedSize
	return nil
}

func flushPendingFrame(w io.Writer, out *writerResult) error {
	if out.pending == nil {
		return nil
	}
	if err := writeFrame(w, *out.pending, nil); err != nil {
		return err
	}
	out.frameCount++
	out.pending = nil
	return nil
}
