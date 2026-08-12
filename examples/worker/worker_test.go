package worker_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
	"github.com/lov3g00d/ctxscope/examples/worker"
)

func TestHappyPathWorkerStopsWhenCanceled(t *testing.T) {
	jobs := make(chan string)
	service := worker.New(jobs)

	ctxscope.Verify(
		t,
		func(ctx context.Context) {
			go service.Run(ctx)
			<-service.Ready()
		},
		ctxscope.WithName("job worker"),
		ctxscope.WithGrace(time.Second),
	)
}

func TestNonHappyPathWorkerThatIgnoresCancellationIsReported(t *testing.T) {
	service := newNonCancellableWorker()
	defer service.Stop()

	report, err := ctxscope.Inspect(
		t.Context(),
		func(ctx context.Context) {
			go service.Run(ctx)
			<-service.Ready()
		},
		ctxscope.WithName("non-cancellable worker"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("inspect worker: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected the cancellation contract to fail")
	}

	if len(report.Survivors) == 0 {
		t.Fatal("expected a surviving goroutine")
	}

	if !reportContainsFunction(report, "nonCancellableWorker") {
		t.Errorf("survivor report does not contain worker Run frame: %+v", report)
	}
}

type nonCancellableWorker struct {
	ready   chan struct{}
	release chan struct{}
	done    chan struct{}
}

func newNonCancellableWorker() *nonCancellableWorker {
	return &nonCancellableWorker{
		ready:   make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (worker *nonCancellableWorker) Run(context.Context) {
	defer close(worker.done)
	close(worker.ready)
	<-worker.release
}

func (worker *nonCancellableWorker) Ready() <-chan struct{} {
	return worker.ready
}

func (worker *nonCancellableWorker) Stop() {
	close(worker.release)
	<-worker.done
}

func reportContainsFunction(report ctxscope.Report, fragment string) bool {
	for _, survivor := range report.Survivors {
		for _, frame := range survivor.Frames {
			if strings.Contains(frame.Function, fragment) {
				return true
			}
		}
	}

	return false
}
