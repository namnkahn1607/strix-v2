package writeback

import (
	"context"
	"strix/internal/gateway/pb"
	"sync"
	"time"
)

type Job struct {
	SlotId  uint32
	Payload []byte
}

type WorkerPool struct {
	queue  chan Job
	wg     sync.WaitGroup
	mu     sync.RWMutex
	closed bool
}

func NewWorkerPool(stub pb.CacheServiceClient, numWorkers int) *WorkerPool {
	const queueCap = 1000
	wp := &WorkerPool{queue: make(chan Job, queueCap)}

	wp.wg.Add(numWorkers)
	for range numWorkers {
		go wp.startWorker(stub)
	}

	return wp
}

func (wp *WorkerPool) TrySchedule(job Job) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if wp.closed {
		return // Pool stopped, drop
	}

	select {
	case wp.queue <- job:
	default: // Saturated queue, drop
	}
}

func (wp *WorkerPool) Stop(ctx context.Context) error {
	wp.mu.Lock()
	wp.closed = true
	close(wp.queue)
	wp.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		// Deadline exceeded, force dropping remaining jobs.
		return ctx.Err()
	}
}

func (wp *WorkerPool) startWorker(stub pb.CacheServiceClient) {
	defer wp.wg.Done()

	const engineTimeout = 100 * time.Millisecond
	for job := range wp.queue {
		ctx, cancel := context.WithTimeout(context.Background(), engineTimeout)
		_, _ = stub.SetCache(ctx, &pb.SetCacheRequest{ // Should handle error
			SlotId:          job.SlotId,
			UncachedPayload: job.Payload,
		})
		cancel()
	}
}
