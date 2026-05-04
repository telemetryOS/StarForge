package corona

import (
	"bytes"
	"crypto/sha256"
	"hash"
	"hash/crc32"
	"runtime"
	"sync"
)

var (
	crc32cTable = crc32.MakeTable(crc32.Castagnoli)
	zeroBlock   = make([]byte, 1024*1024)
)

type allocatedHasher struct {
	hash.Hash
}

func newAllocatedHasher() allocatedHasher {
	return allocatedHasher{Hash: sha256.New()}
}

func (h allocatedHasher) WriteZeros(size uint64) error {
	for size > 0 {
		n := uint64(len(zeroBlock))
		if size < n {
			n = size
		}
		if _, err := h.Write(zeroBlock[:n]); err != nil {
			return err
		}
		size -= n
	}
	return nil
}

func (h allocatedHasher) Sum() []byte {
	return h.Hash.Sum(nil)
}

func allZero(buf []byte) bool {
	for len(buf) > 0 {
		n := len(zeroBlock)
		if len(buf) < n {
			n = len(buf)
		}
		if !bytes.Equal(buf[:n], zeroBlock[:n]) {
			return false
		}
		buf = buf[n:]
	}
	return true
}

func normalizeWorkers(workers int) int {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers < 1 {
		return 1
	}
	if workers > 32 {
		return 32
	}
	return workers
}

func normalizeWriteWorkers(workers int, order WriteOrder) int {
	if order == WriteOrderSequential {
		return 1
	}
	return normalizeWorkers(workers)
}

func reportProgress(fn func(Progress), processed, total uint64) {
	if fn == nil {
		return
	}
	pct := 0
	if total > 0 {
		pct = int((processed * 100) / total)
		if pct > 100 {
			pct = 100
		}
	}
	fn(Progress{ProcessedBytes: processed, TotalBytes: total, Percent: pct})
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

type errorSlot struct {
	mu  sync.Mutex
	err error
}

func (s *errorSlot) get() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *errorSlot) store(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}
