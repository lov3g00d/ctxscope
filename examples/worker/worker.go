// Package worker provides a small service used by the ctxscope example.
package worker

import "context"

// Worker consumes jobs until its context is canceled or the jobs channel is
// closed.
type Worker struct {
	jobs  <-chan string
	ready chan struct{}
}

// New creates a Worker that consumes jobs.
func New(jobs <-chan string) *Worker {
	return &Worker{
		jobs:  jobs,
		ready: make(chan struct{}),
	}
}

// Run blocks until the worker stops.
func (worker *Worker) Run(ctx context.Context) {
	close(worker.ready)

	for {
		select {
		case <-ctx.Done():
			return
		case _, open := <-worker.jobs:
			if !open {
				return
			}
		}
	}
}

// Ready is closed when Run starts receiving work.
func (worker *Worker) Ready() <-chan struct{} {
	return worker.ready
}
