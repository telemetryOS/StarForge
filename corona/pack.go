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
	data   []byte
	result chan<- frameResult
}

type frameResult struct {
	header  frameHeader
	payload []byte
	err     error
}

type writerResult struct {
	frameCount  uint64
	usefulBytes uint64
	storedBytes uint64
	err         error
}

func Pack(ctx context.Context, opts PackOptions) error {
	if opts.ImagePath == "" {
		return errors.New("corona: image path is required")
	}
	if opts.ArtifactPath == "" {
		return errors.New("corona: artifact path is required")
	}
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize < 4096 {
		return fmt.Errorf("corona: chunk size %d is too small", chunkSize)
	}
	src, err := os.Open(opts.ImagePath)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat image: %w", err)
	}
	if info.Size() <= 0 {
		return fmt.Errorf("corona: invalid image size %d", info.Size())
	}
	alloc := detectAllocationChecker(src, uint64(info.Size()))
	if err := packArtifact(ctx, src, opts.ArtifactPath, uint64(info.Size()), uint64(chunkSize), normalizeWorkers(opts.Workers), opts.Progress, alloc); err != nil {
		return err
	}
	reportProgress(opts.Progress, uint64(info.Size()), uint64(info.Size()))
	return nil
}

func packArtifact(ctx context.Context, src *os.File, artifactPath string, imageSize, chunkSize uint64, workers int, progress func(Progress), alloc allocationChecker) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dst, err := os.Create(artifactPath)
	if err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}
	defer func() {
		if err := dst.Close(); retErr == nil && err != nil {
			retErr = fmt.Errorf("close artifact: %w", err)
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
	for off := uint64(0); off < imageSize; {
		if err := ctx.Err(); err != nil {
			readErr = err
			break
		}
		maxSize := chunkSize
		if remain := imageSize - off; remain < maxSize {
			maxSize = remain
		}
		plan := framePlan{offset: off, size: maxSize}
		if alloc != nil {
			var err error
			plan, err = alloc.nextFramePlan(off, maxSize)
			if err != nil {
				readErr = err
				cancel()
				break
			}
		}
		result := make(chan frameResult, 1)
		select {
		case frameSlots <- result:
		case <-ctx.Done():
			readErr = ctx.Err()
			break readLoop
		}
		if plan.skip {
			result <- frameResult{header: frameHeader{
				flags:            frameSkip,
				targetOffset:     plan.offset,
				uncompressedSize: plan.size,
			}}
			close(result)
			off += plan.size
			continue
		}
		data := make([]byte, plan.size)
		if _, err := src.ReadAt(data, int64(plan.offset)); err != nil && !errors.Is(err, io.EOF) {
			readErr = fmt.Errorf("read image at %d: %w", plan.offset, err)
			cancel()
			break
		}
		if _, err := h.Write(data); err != nil {
			readErr = err
			cancel()
			break
		}
		select {
		case jobs <- packJob{offset: plan.offset, size: plan.size, data: data, result: result}:
		case <-ctx.Done():
			readErr = ctx.Err()
			break readLoop
		}
		off += plan.size
	}
	close(jobs)
	wg.Wait()
	close(frameSlots)
	writer := <-writerDone
	if readErr != nil {
		return readErr
	}
	if writer.err != nil {
		return writer.err
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
		return fmt.Errorf("sync artifact: %w", err)
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
		header := frameHeader{targetOffset: job.offset, uncompressedSize: job.size}
		if allZero(job.data) {
			header.flags = frameZero
			job.result <- frameResult{header: header}
			close(job.result)
			continue
		}
		payload := enc.EncodeAll(job.data, nil)
		header.flags = frameZstd
		header.compressedSize = uint64(len(payload))
		header.crc32c = crc32.Checksum(job.data, crc32cTable)
		job.result <- frameResult{header: header, payload: payload}
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
		if err := writeFrame(dst, res.header, res.payload); err != nil {
			out.err = err
			cancel()
			done <- out
			return
		}
		out.frameCount++
		out.usefulBytes += res.header.uncompressedSize
		out.storedBytes += res.header.compressedSize
		reportProgress(progress, out.usefulBytes, imageSize)
	}
	done <- out
}
