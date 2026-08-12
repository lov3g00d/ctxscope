package ctxscope

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope/internal/profiler"
)

func TestScopedTaskRestoresWorkerLabels(t *testing.T) {
	pool := newSharedTestPool(1)
	defer pool.Close()

	ready := make(chan struct{})
	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			pool.Submit(scope.Task("tracked task", func(ctx context.Context) {
				close(ready)
				<-ctx.Done()
			}))
			<-ready
		},
		WithGrace(time.Second),
		WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("InspectScoped returned an error: %v", err)
	}

	if !report.Passed() {
		t.Fatalf("tracked task did not stop: %+v", report)
	}

	unwrappedReady := make(chan struct{})
	unwrappedRelease := make(chan struct{})
	unwrappedDone := make(chan struct{})
	pool.Submit(func() {
		defer close(unwrappedDone)
		close(unwrappedReady)
		<-unwrappedRelease
	})
	<-unwrappedReady

	survivors, captureErr := profiler.CaptureScope(report.ScopeID)
	close(unwrappedRelease)
	<-unwrappedDone

	if captureErr != nil {
		t.Fatalf("capture completed scope: %v", captureErr)
	}

	if len(survivors) != 0 {
		t.Fatalf(
			"unwrapped follow-up work retained scope %q: %+v",
			report.ScopeID,
			survivors,
		)
	}
}

func TestConcurrentScopedInspectionsSharePoolWithoutCrossTalk(t *testing.T) {
	t.Parallel()
	parent := t.Context()

	pool := newSharedTestPool(2)
	defer pool.Close()

	type result struct {
		name   string
		report Report
		err    error
	}

	var tasksReady sync.WaitGroup
	tasksReady.Add(2)

	results := make(chan result, 2)
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	doneFirst := make(chan struct{})
	doneSecond := make(chan struct{})

	run := func(name string, release <-chan struct{}, done chan<- struct{}) {
		report, err := InspectScoped(
			parent,
			func(scope *Scope) {
				pool.Submit(scope.Task(name, func(context.Context) {
					defer close(done)
					tasksReady.Done()
					tasksReady.Wait()
					<-release
				}))
				tasksReady.Wait()
			},
			WithName(name),
			WithGrace(20*time.Millisecond),
			WithPollInterval(time.Millisecond),
		)

		results <- result{name: name, report: report, err: err}
	}

	go run("first pooled operation", releaseFirst, doneFirst)
	go run("second pooled operation", releaseSecond, doneSecond)

	got := map[string]result{}
	for range 2 {
		inspection := <-results
		got[inspection.name] = inspection
	}

	close(releaseFirst)
	close(releaseSecond)
	<-doneFirst
	<-doneSecond

	first := got["first pooled operation"]
	second := got["second pooled operation"]

	if first.err != nil {
		t.Fatalf("first inspection returned an error: %v", first.err)
	}

	if second.err != nil {
		t.Fatalf("second inspection returned an error: %v", second.err)
	}

	if first.report.ScopeID == second.report.ScopeID {
		t.Fatalf("concurrent inspections share scope ID %q", first.report.ScopeID)
	}

	assertScopedPoolResult(t, first.report, "first pooled operation")
	assertScopedPoolResult(t, second.report, "second pooled operation")
	assertScopeAbsent(t, first.report, second.report.ScopeID)
	assertScopeAbsent(t, second.report, first.report.ScopeID)
}

func TestQueuedTaskStartingBeforeDeadlineIsReportedRunning(t *testing.T) {
	const (
		grace      = 100 * time.Millisecond
		startDelay = 50 * time.Millisecond
	)

	pool := newSharedTestPool(1)
	defer pool.Close()

	blockerReady := make(chan struct{})
	releaseBlocker := make(chan struct{})
	pool.Submit(func() {
		close(blockerReady)
		<-releaseBlocker
	})
	<-blockerReady

	taskStarted := make(chan struct{})
	releaseTask := make(chan struct{})
	taskDone := make(chan struct{})

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			pool.Submit(scope.Task("delayed task", func(context.Context) {
				defer close(taskDone)
				close(taskStarted)
				<-releaseTask
			}))

			time.AfterFunc(startDelay, func() {
				close(releaseBlocker)
			})
		},
		WithGrace(grace),
		WithPollInterval(time.Millisecond),
		WithMaxPollInterval(10*time.Millisecond),
	)

	<-taskStarted
	close(releaseTask)
	<-taskDone

	if err != nil {
		t.Fatalf("InspectScoped returned an error: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected delayed task to exceed the shutdown deadline")
	}

	if len(report.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(report.Tasks))
	}

	task := report.Tasks[0]
	if task.State != TaskRunning {
		t.Fatalf("task state = %q, want %q", task.State, TaskRunning)
	}

	if reportHasViolation(report, ViolationTaskNeverStarted) {
		t.Errorf("running task was classified as never started: %+v", report.Violations)
	}

	if !reportHasViolation(report, ViolationTaskStillRunning) {
		t.Errorf("report lacks running-task violation: %+v", report.Violations)
	}

	startedAfterCancellation := task.StartedAt.Sub(report.CanceledAt)
	if startedAfterCancellation < startDelay/2 {
		t.Errorf(
			"task started only %s after cancellation, expected delayed start",
			startedAfterCancellation,
		)
	}

	if startedAfterCancellation >= grace {
		t.Errorf(
			"task started after the grace deadline: %s >= %s",
			startedAfterCancellation,
			grace,
		)
	}
}

func assertScopedPoolResult(t *testing.T, report Report, taskName string) {
	t.Helper()
	assertSingleScopedSurvivor(t, report)

	if len(report.Tasks) != 1 {
		t.Fatalf("operation %q reported %d tasks, want 1", report.Name, len(report.Tasks))
	}

	task := report.Tasks[0]
	if task.Name != taskName {
		t.Errorf("task name = %q, want %q", task.Name, taskName)
	}

	if task.State != TaskRunning {
		t.Errorf("task state = %q, want %q", task.State, TaskRunning)
	}

	if countGoroutines(task.Survivors) != 1 {
		t.Errorf(
			"task %q has %d attributed survivors, want 1",
			task.Name,
			countGoroutines(task.Survivors),
		)
	}
}

func assertScopeAbsent(t *testing.T, report Report, unwantedScopeID string) {
	t.Helper()

	for _, survivor := range report.Survivors {
		if slices.Contains(
			survivor.Labels[profiler.ScopeLabel],
			unwantedScopeID,
		) {
			t.Errorf(
				"operation %q captured scope %q: %+v",
				report.Name,
				unwantedScopeID,
				survivor.Labels,
			)
		}
	}
}

type sharedTestPool struct {
	jobs    chan func()
	workers sync.WaitGroup
}

func newSharedTestPool(workerCount int) *sharedTestPool {
	pool := &sharedTestPool{
		jobs: make(chan func(), workerCount*2),
	}

	pool.workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer pool.workers.Done()
			for job := range pool.jobs {
				job()
			}
		}()
	}

	return pool
}

func (pool *sharedTestPool) Submit(task func()) {
	pool.jobs <- task
}

func (pool *sharedTestPool) Close() {
	close(pool.jobs)
	pool.workers.Wait()
}
