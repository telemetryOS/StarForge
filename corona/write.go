package corona

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

type chunkJob struct {
	offset uint64
	size   uint64
}

func Write(ctx context.Context, opts WriteOptions) error {
	if opts.ArtifactPath == "" {
		return errors.New("corona: artifact path is required")
	}
	if opts.TargetPath == "" {
		return errors.New("corona: target path is required")
	}
	info, err := Inspect(ctx, opts.ArtifactPath)
	if err != nil {
		return err
	}
	target, err := os.OpenFile(opts.TargetPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	defer target.Close()
	if err := validateTargetCapacity(target, info.ImageSize); err != nil {
		return err
	}
	artifact, err := os.Open(opts.ArtifactPath)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer artifact.Close()
	if _, err := scanArtifact(ctx, artifact, func(header frameHeader, raw []byte) error {
		if header.flags == frameZero {
			return writeZeros(target, header.targetOffset, header.uncompressedSize)
		}
		if header.flags == frameSkip {
			return nil
		}
		if _, err := target.WriteAt(raw, int64(header.targetOffset)); err != nil {
			return fmt.Errorf("write chunk at %d: %w", header.targetOffset, err)
		}
		reportProgress(opts.Progress, header.targetOffset+header.uncompressedSize, info.ImageSize)
		return nil
	}); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync target: %w", err)
	}
	reportProgress(opts.Progress, info.ImageSize, info.ImageSize)
	return nil
}

func WriteImage(ctx context.Context, opts WriteImageOptions) error {
	if opts.ImagePath == "" {
		return errors.New("corona: image path is required")
	}
	if opts.TargetPath == "" {
		return errors.New("corona: target path is required")
	}
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
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
	target, err := os.OpenFile(opts.TargetPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	defer target.Close()
	imageSize := uint64(info.Size())
	if err := validateTargetCapacity(target, imageSize); err != nil {
		return err
	}
	if err := writeImageChunks(ctx, src, target, imageSize, uint64(chunkSize), opts.Workers, opts.WriteOrder, opts.Progress); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync target: %w", err)
	}
	reportProgress(opts.Progress, imageSize, imageSize)
	return nil
}

func writeImageChunks(ctx context.Context, src, target *os.File, imageSize, chunkSize uint64, workers int, order WriteOrder, progress func(Progress)) error {
	workerCount := normalizeWriteWorkers(workers, order)
	orderedJobs := scheduleChunkJobs(chunkJobs(imageSize, chunkSize), order, workerCount)
	jobs := make(chan chunkJob, workerCount)
	var processed atomic.Uint64
	var firstErr errorSlot
	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go writeImageWorker(ctx, src, target, jobs, &processed, imageSize, progress, &firstErr, &wg)
	}
	for _, job := range orderedJobs {
		if firstErr.get() != nil {
			break
		}
		jobs <- job
	}
	close(jobs)
	wg.Wait()
	return firstErr.get()
}

func writeImageWorker(ctx context.Context, src, target *os.File, jobs <-chan chunkJob, processed *atomic.Uint64, imageSize uint64, progress func(Progress), firstErr *errorSlot, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		if err := ctx.Err(); err != nil {
			firstErr.store(err)
			continue
		}
		data := make([]byte, job.size)
		if _, err := src.ReadAt(data, int64(job.offset)); err != nil && !errors.Is(err, io.EOF) {
			firstErr.store(fmt.Errorf("read image at %d: %w", job.offset, err))
			continue
		}
		if allZero(data) {
			if err := writeZeros(target, job.offset, job.size); err != nil {
				firstErr.store(err)
				continue
			}
		} else if _, err := target.WriteAt(data, int64(job.offset)); err != nil {
			firstErr.store(fmt.Errorf("write image chunk at %d: %w", job.offset, err))
			continue
		}
		done := processed.Add(job.size)
		reportProgress(progress, done, imageSize)
	}
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
