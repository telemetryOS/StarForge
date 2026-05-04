package corona

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type chunkJob struct {
	offset uint64
	size   uint64
	ops    []plannedOp
	result chan<- imageChunkResult
}

type imageChunkResult struct {
	ops []plannedOp
	err error
}

type imageWriterResult struct {
	processed uint64
	err       error
}

type coronaFrameJob struct {
	header  frameHeader
	payload []byte
	result  chan<- coronaFrameResult
}

type coronaFrameResult struct {
	header frameHeader
	raw    []byte
	err    error
}

type coronaWriterResult struct {
	summary writeSummary
	err     error
}

func flash(ctx context.Context, opts flashOptions) error {
	if opts.CoronaPath == "" {
		return errors.New("corona: corona path is required")
	}
	if opts.TargetPath == "" {
		return errors.New("corona: target path is required")
	}
	if err := rejectSamePath(opts.CoronaPath, opts.TargetPath, "flash corona"); err != nil {
		return err
	}
	corona, err := os.Open(opts.CoronaPath)
	if err != nil {
		return fmt.Errorf("open corona: %w", err)
	}
	defer corona.Close()
	coronaInfo, err := corona.Stat()
	if err != nil {
		return fmt.Errorf("stat corona: %w", err)
	}
	header, err := readFileHeader(corona)
	if err != nil {
		return err
	}
	target, err := os.OpenFile(opts.TargetPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	defer target.Close()
	if err := requireBlockDevice(opts.TargetPath, target, "target"); err != nil {
		return err
	}
	if err := validateTargetCapacity(target, header.imageSize); err != nil {
		return err
	}
	if _, err := writeCoronaFrames(ctx, corona, target, header, uint64(coronaInfo.Size()), opts.Workers, opts.WriteOrder, opts.Progress, false, opts.ZeroSkipped); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync target: %w", err)
	}
	reportProgress(opts.Progress, header.imageSize, header.imageSize)
	return nil
}

func writeCoronaFrames(ctx context.Context, corona, target *os.File, header fileHeader, coronaSize uint64, workers int, order WriteOrder, progress func(Progress), sparseZero, zeroSkipped bool) (Info, error) {
	if coronaSize < fileHeaderLen+fileTrailerLen {
		return Info{}, errShortHeader
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerCount := normalizeWriteWorkers(workers, order)
	jobs := make(chan coronaFrameJob, workerCount)
	frameSlots := make(chan chan coronaFrameResult, workerCount)
	writerDone := make(chan coronaWriterResult, 1)
	go writeCoronaFrameSlots(ctx, target, header, frameSlots, writerDone, progress, cancel, sparseZero, zeroSkipped)

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go coronaFrameWorker(ctx, jobs, &wg)
	}

	var expectedOffset uint64
	trailerStart := coronaSize - fileTrailerLen
	var readErr error
readLoop:
	for expectedOffset < header.imageSize {
		if err := ctx.Err(); err != nil {
			readErr = err
			break
		}
		pos, err := corona.Seek(0, io.SeekCurrent)
		if err != nil {
			readErr = fmt.Errorf("seek corona: %w", err)
			cancel()
			break
		}
		if uint64(pos)+frameHeaderLen > trailerStart {
			readErr = errors.New("corona: frame data ended before image was complete")
			cancel()
			break
		}
		frame, err := readFrameHeader(corona)
		if err != nil {
			readErr = err
			cancel()
			break
		}
		if err := validateFrame(frame, header.imageSize, expectedOffset, trailerStart-uint64(pos)-frameHeaderLen); err != nil {
			readErr = err
			cancel()
			break
		}
		var payload []byte
		if frame.flags == frameZstd {
			payload = make([]byte, frame.compressedSize)
			if _, err := io.ReadFull(corona, payload); err != nil {
				readErr = fmt.Errorf("read frame payload: %w", err)
				cancel()
				break
			}
		}
		result := make(chan coronaFrameResult, 1)
		select {
		case frameSlots <- result:
		case <-ctx.Done():
			readErr = ctx.Err()
			break readLoop
		}
		select {
		case jobs <- coronaFrameJob{header: frame, payload: payload, result: result}:
		case <-ctx.Done():
			readErr = ctx.Err()
			result <- coronaFrameResult{err: readErr}
			close(result)
			break readLoop
		}
		expectedOffset += frame.uncompressedSize
	}
	var trailer fileTrailer
	if readErr == nil {
		var err error
		trailer, err = readFileTrailerFromReader(corona)
		if err != nil {
			readErr = err
			cancel()
		}
	}
	pos, err := corona.Seek(0, io.SeekCurrent)
	if readErr == nil {
		if err != nil {
			readErr = fmt.Errorf("seek corona: %w", err)
			cancel()
		} else if uint64(pos) != coronaSize {
			readErr = fmt.Errorf("corona: trailing bytes after trailer: at %d, want %d", pos, coronaSize)
			cancel()
		}
	}
	close(jobs)
	wg.Wait()
	close(frameSlots)
	writer := <-writerDone
	if writer.err != nil {
		return Info{}, writer.err
	}
	if readErr != nil {
		return Info{}, readErr
	}
	writer.summary.trailer = trailer
	return writer.summary.info()
}

func coronaFrameWorker(ctx context.Context, jobs <-chan coronaFrameJob, wg *sync.WaitGroup) {
	defer wg.Done()
	dec, err := zstd.NewReader(nil)
	if err != nil {
		for job := range jobs {
			job.result <- coronaFrameResult{err: fmt.Errorf("create zstd reader: %w", err)}
			close(job.result)
		}
		return
	}
	defer dec.Close()
	for job := range jobs {
		if err := ctx.Err(); err != nil {
			job.result <- coronaFrameResult{err: err}
			close(job.result)
			continue
		}
		res := coronaFrameResult{header: job.header}
		if job.header.flags == frameZstd {
			raw, err := dec.DecodeAll(job.payload, nil)
			if err != nil {
				res.err = fmt.Errorf("decompress chunk at %d: %w", job.header.targetOffset, err)
			} else if uint64(len(raw)) != job.header.uncompressedSize {
				res.err = fmt.Errorf("corona: chunk at %d size mismatch", job.header.targetOffset)
			} else if got := crc32.Checksum(raw, crc32cTable); got != job.header.crc32c {
				res.err = fmt.Errorf("corona: chunk at %d crc32c mismatch", job.header.targetOffset)
			} else {
				res.raw = raw
			}
		}
		job.result <- res
		close(job.result)
	}
}

func writeCoronaFrameSlots(ctx context.Context, target *os.File, header fileHeader, frameSlots <-chan chan coronaFrameResult, done chan<- coronaWriterResult, progress func(Progress), cancel context.CancelFunc, sparseZero, zeroSkipped bool) {
	out := coronaWriterResult{summary: writeSummary{
		header: header,
		hash:   newAllocatedHasher(),
	}}
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
		switch res.header.flags {
		case frameZero:
			if !sparseZero {
				if err := writeZeros(target, res.header.targetOffset, res.header.uncompressedSize); err != nil {
					out.err = err
					cancel()
					done <- out
					return
				}
			}
			if err := out.summary.hash.WriteZeros(res.header.uncompressedSize); err != nil {
				out.err = err
				cancel()
				done <- out
				return
			}
		case frameSkip:
			if zeroSkipped {
				if err := writeZeros(target, res.header.targetOffset, res.header.uncompressedSize); err != nil {
					out.err = err
					cancel()
					done <- out
					return
				}
			}
		case frameZstd:
			if _, err := target.WriteAt(res.raw, int64(res.header.targetOffset)); err != nil {
				out.err = fmt.Errorf("write chunk at %d: %w", res.header.targetOffset, err)
				cancel()
				done <- out
				return
			}
			if _, err := out.summary.hash.Write(res.raw); err != nil {
				out.err = err
				cancel()
				done <- out
				return
			}
		default:
			out.err = fmt.Errorf("corona: invalid frame flags %d", res.header.flags)
			cancel()
			done <- out
			return
		}
		out.summary.addFrame(res.header)
		reportProgress(progress, out.summary.usefulBytes, header.imageSize)
	}
	done <- out
}

func flashImage(ctx context.Context, opts flashImageOptions) error {
	if opts.ImagePath == "" {
		return errors.New("corona: image path is required")
	}
	if opts.TargetPath == "" {
		return errors.New("corona: target path is required")
	}
	if err := rejectSamePath(opts.ImagePath, opts.TargetPath, "write image"); err != nil {
		return err
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
	if !info.Mode().IsRegular() {
		return fmt.Errorf("corona: image source must be a regular file")
	}
	target, err := os.OpenFile(opts.TargetPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	defer target.Close()
	if err := requireBlockDevice(opts.TargetPath, target, "target"); err != nil {
		return err
	}
	imageSize := uint64(info.Size())
	if err := validateTargetCapacity(target, imageSize); err != nil {
		return err
	}
	alloc := detectAllocationChecker(src, imageSize)
	if err := writeImageChunks(ctx, src, target, imageSize, uint64(chunkSize), opts.Workers, opts.WriteOrder, opts.Progress, alloc, false, opts.ZeroSkipped); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync target: %w", err)
	}
	reportProgress(opts.Progress, imageSize, imageSize)
	return nil
}

func captureImage(ctx context.Context, opts captureImageOptions) error {
	if opts.SourcePath == "" {
		return errors.New("corona: source path is required")
	}
	if opts.ImagePath == "" {
		return errors.New("corona: image path is required")
	}
	if err := rejectSamePath(opts.SourcePath, opts.ImagePath, "read image"); err != nil {
		return err
	}
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize < 4096 {
		return fmt.Errorf("corona: chunk size %d is too small", chunkSize)
	}
	src, err := os.Open(opts.SourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()
	imageSize, err := sourceCapacity(src)
	if err != nil {
		return err
	}
	target, err := os.OpenFile(opts.ImagePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create image: %w", err)
	}
	defer target.Close()
	if err := target.Truncate(int64(imageSize)); err != nil {
		return fmt.Errorf("size image: %w", err)
	}
	alloc := detectAllocationChecker(src, imageSize)
	if err := writeImageChunks(ctx, src, target, imageSize, uint64(chunkSize), opts.Workers, opts.WriteOrder, opts.Progress, alloc, true, false); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync image: %w", err)
	}
	reportProgress(opts.Progress, imageSize, imageSize)
	return nil
}

func extractImage(ctx context.Context, opts extractImageOptions) error {
	if opts.CoronaPath == "" {
		return errors.New("corona: corona path is required")
	}
	if opts.ImagePath == "" {
		return errors.New("corona: image path is required")
	}
	if err := rejectSamePath(opts.CoronaPath, opts.ImagePath, "extract corona"); err != nil {
		return err
	}
	corona, err := os.Open(opts.CoronaPath)
	if err != nil {
		return fmt.Errorf("open corona: %w", err)
	}
	defer corona.Close()
	coronaInfo, err := corona.Stat()
	if err != nil {
		return fmt.Errorf("stat corona: %w", err)
	}
	header, err := readFileHeader(corona)
	if err != nil {
		return err
	}
	target, err := os.OpenFile(opts.ImagePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create image: %w", err)
	}
	defer target.Close()
	if err := target.Truncate(int64(header.imageSize)); err != nil {
		return fmt.Errorf("size image: %w", err)
	}
	if _, err := writeCoronaFrames(ctx, corona, target, header, uint64(coronaInfo.Size()), opts.Workers, opts.WriteOrder, opts.Progress, true, false); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync image: %w", err)
	}
	reportProgress(opts.Progress, header.imageSize, header.imageSize)
	return nil
}

func writeImageChunks(ctx context.Context, src, target *os.File, imageSize, chunkSize uint64, workers int, order WriteOrder, progress func(Progress), alloc allocationChecker, sparseZero, zeroSkipped bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	workerCount := normalizeWriteWorkers(workers, order)
	jobs := make(chan chunkJob, workerCount)
	resultSlots := make(chan chan imageChunkResult, workerCount)
	writerDone := make(chan imageWriterResult, 1)
	go writeImageResultSlots(ctx, target, imageSize, sparseZero, zeroSkipped, resultSlots, writerDone, progress, cancel)

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go writeImageWorker(ctx, src, jobs, &wg)
	}

	var readErr error
	emitErr := produceScheduledChunks(imageSize, chunkSize, order, workerCount, func(chunk chunkJob) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		ops, err := plannedOpsForChunk(alloc, chunk.offset, chunk.size)
		if err != nil {
			return err
		}
		result := make(chan imageChunkResult, 1)
		select {
		case resultSlots <- result:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case jobs <- chunkJob{offset: chunk.offset, size: chunk.size, ops: ops, result: result}:
		case <-ctx.Done():
			err := ctx.Err()
			result <- imageChunkResult{err: err}
			close(result)
			return err
		}
		return nil
	})
	if emitErr != nil {
		readErr = emitErr
		cancel()
	}
	close(jobs)
	wg.Wait()
	close(resultSlots)
	writer := <-writerDone
	if writer.err != nil {
		return writer.err
	}
	if readErr != nil {
		return readErr
	}
	return nil
}

func writeImageWorker(ctx context.Context, src *os.File, jobs <-chan chunkJob, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		if err := ctx.Err(); err != nil {
			job.result <- imageChunkResult{err: err}
			close(job.result)
			continue
		}
		result := imageChunkResult{ops: make([]plannedOp, 0, len(job.ops))}
		for _, op := range job.ops {
			if err := ctx.Err(); err != nil {
				result.err = err
				break
			}
			out := plannedOp{offset: op.offset, size: op.size, skip: op.skip}
			if op.skip {
				result.ops = append(result.ops, out)
				continue
			}
			data := make([]byte, op.size)
			if _, err := src.ReadAt(data, int64(op.offset)); err != nil && !errors.Is(err, io.EOF) {
				result.err = fmt.Errorf("read image at %d: %w", op.offset, err)
				break
			}
			out.data = data
			result.ops = append(result.ops, out)
		}
		job.result <- result
		close(job.result)
	}
}

func writeImageResultSlots(ctx context.Context, target *os.File, imageSize uint64, sparseTarget, zeroSkipped bool, resultSlots <-chan chan imageChunkResult, done chan<- imageWriterResult, progress func(Progress), cancel context.CancelFunc) {
	var out imageWriterResult
	for resultCh := range resultSlots {
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
		for _, op := range res.ops {
			if op.skip {
				if zeroSkipped {
					if err := writeZeros(target, op.offset, op.size); err != nil {
						out.err = err
						cancel()
						done <- out
						return
					}
				}
				out.processed += op.size
				reportProgress(progress, out.processed, imageSize)
				continue
			}
			if allZero(op.data) {
				if !sparseTarget {
					if err := writeZeros(target, op.offset, op.size); err != nil {
						out.err = err
						cancel()
						done <- out
						return
					}
				}
			} else if _, err := target.WriteAt(op.data, int64(op.offset)); err != nil {
				out.err = fmt.Errorf("write image chunk at %d: %w", op.offset, err)
				cancel()
				done <- out
				return
			}
			out.processed += op.size
			reportProgress(progress, out.processed, imageSize)
		}
	}
	done <- out
}

func chunkJobs(imageSize, chunkSize uint64) []chunkJob {
	jobs := make([]chunkJob, 0, (imageSize+chunkSize-1)/chunkSize)
	for off := uint64(0); off < imageSize; off += chunkSize {
		n := chunkSize
		if remain := imageSize - off; remain < n {
			n = remain
		}
		jobs = append(jobs, chunkJob{offset: off, size: n})
	}
	return jobs
}

func produceScheduledChunks(imageSize, chunkSize uint64, order WriteOrder, workers int, emit func(chunkJob) error) error {
	if order != WriteOrderStriped || workers <= 1 {
		for off := uint64(0); off < imageSize; off += chunkSize {
			n := chunkSize
			if remain := imageSize - off; remain < n {
				n = remain
			}
			if err := emit(chunkJob{offset: off, size: n}); err != nil {
				return err
			}
		}
		return nil
	}
	count := (imageSize + chunkSize - 1) / chunkSize
	for start := 0; start < workers; start++ {
		for i := uint64(start); i < count; i += uint64(workers) {
			off := i * chunkSize
			n := chunkSize
			if remain := imageSize - off; remain < n {
				n = remain
			}
			if err := emit(chunkJob{offset: off, size: n}); err != nil {
				return err
			}
		}
	}
	return nil
}

func scheduleChunkJobs(jobs []chunkJob, order WriteOrder, workers int) []chunkJob {
	cp := append([]chunkJob(nil), jobs...)
	if order != WriteOrderStriped || workers <= 1 {
		return cp
	}
	out := make([]chunkJob, 0, len(cp))
	for start := 0; start < workers; start++ {
		for i := start; i < len(cp); i += workers {
			out = append(out, cp[i])
		}
	}
	return out
}
