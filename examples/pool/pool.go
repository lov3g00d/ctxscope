// Package pool provides a small fixed-size worker pool for the ctxscope
// examples.
package pool

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed is returned when work is submitted after the pool is closed.
var ErrClosed = errors.New("worker pool is closed")

// Pool executes submitted functions on a fixed number of reusable goroutines.
type Pool struct {
	jobs chan func()

	mu      sync.RWMutex
	closed  bool
	workers sync.WaitGroup
}

// New starts workerCount workers and creates a queue with queueCapacity slots.
func New(workerCount, queueCapacity int) *Pool {
	if workerCount <= 0 {
		panic("worker pool requires at least one worker")
	}
	if queueCapacity < 0 {
		panic("worker pool queue capacity cannot be negative")
	}

	pool := &Pool{
		jobs: make(chan func(), queueCapacity),
	}

	pool.workers.Add(workerCount)
	for range workerCount {
		go pool.runWorker()
	}

	return pool
}

// Submit waits until task enters the queue, the context is canceled, or the
// pool is closed.
func (pool *Pool) Submit(ctx context.Context, task func()) error {
	if ctx == nil {
		return errors.New("worker pool requires a non-nil context")
	}
	if task == nil {
		return errors.New("worker pool requires a non-nil task")
	}

	pool.mu.RLock()
	defer pool.mu.RUnlock()

	if pool.closed {
		return ErrClosed
	}

	select {
	case pool.jobs <- task:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// Close drains queued work, stops the workers, and waits for them to return.
// Callers must arrange for running tasks to finish before calling Close.
func (pool *Pool) Close() {
	pool.mu.Lock()
	if !pool.closed {
		pool.closed = true
		close(pool.jobs)
	}
	pool.mu.Unlock()

	pool.workers.Wait()
}

func (pool *Pool) runWorker() {
	defer pool.workers.Done()
	for task := range pool.jobs {
		task()
	}
}
