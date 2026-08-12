package ctxscope

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInspectScopedTracksTaskAcrossWorkerPool(t *testing.T) {
	jobs := make(chan func())
	poolDone := make(chan struct{})
	ready := make(chan struct{})

	go func() {
		defer close(poolDone)
		(<-jobs)()
	}()

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			jobs <- scope.Task(
				"pooled worker",
				func(ctx context.Context) {
					close(ready)
					<-ctx.Done()
				},
			)
			<-ready
		},
		WithGrace(time.Second),
		WithPollInterval(time.Millisecond),
	)
	<-poolDone

	if err != nil {
		t.Fatalf("InspectScoped returned an error: %v", err)
	}

	if !report.Passed() {
		t.Fatalf("expected report to pass: %+v", report)
	}

	if len(report.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(report.Tasks))
	}

	task := report.Tasks[0]
	if task.Name != "pooled worker" {
		t.Errorf("got task name %q, want %q", task.Name, "pooled worker")
	}

	if task.State != TaskCompleted {
		t.Errorf("got task state %q, want %q", task.State, TaskCompleted)
	}

	if task.RegisteredAt.IsZero() || task.StartedAt.IsZero() || task.CompletedAt.IsZero() {
		t.Errorf("task lifecycle timestamps are incomplete: %+v", task)
	}

	if task.StartedAt.Before(task.RegisteredAt) {
		t.Errorf("task started before registration: %+v", task)
	}

	if task.CompletedAt.Before(task.StartedAt) {
		t.Errorf("task completed before it started: %+v", task)
	}

	if !framesContain(task.RegistrationStack, "TestInspectScopedTracksTaskAcrossWorkerPool") {
		t.Errorf("registration stack does not contain test function: %+v", task.RegistrationStack)
	}
}

func TestInspectScopedReportsRunningPooledTask(t *testing.T) {
	jobs := make(chan func())
	poolDone := make(chan struct{})
	ready := make(chan struct{})
	release := make(chan struct{})

	go func() {
		defer close(poolDone)
		(<-jobs)()
	}()

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			jobs <- scope.Task(
				"stuck pooled worker",
				func(context.Context) {
					close(ready)
					<-release
				},
			)
			<-ready
		},
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	close(release)
	<-poolDone

	if err != nil {
		t.Fatalf("InspectScoped returned an error: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected the running task to violate shutdown")
	}

	if !reportHasViolation(report, ViolationShutdownTimeout) {
		t.Errorf("report does not contain shutdown violation: %+v", report.Violations)
	}

	if !reportHasViolation(report, ViolationTaskStillRunning) {
		t.Errorf("report does not contain running-task violation: %+v", report.Violations)
	}

	if len(report.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(report.Tasks))
	}

	task := report.Tasks[0]
	if task.State != TaskRunning {
		t.Errorf("got task state %q, want %q", task.State, TaskRunning)
	}

	if countGoroutines(task.Survivors) != 1 {
		t.Fatalf(
			"task has %d attributed survivors, want 1",
			countGoroutines(task.Survivors),
		)
	}

	if !framesContain(task.RegistrationStack, "TestInspectScopedReportsRunningPooledTask") {
		t.Errorf("registration stack does not contain test function: %+v", task.RegistrationStack)
	}

	if !framesContain(task.Survivors[0].Frames, "TestInspectScopedReportsRunningPooledTask") {
		t.Errorf("survivor stack does not contain task function: %+v", task.Survivors)
	}
}

func TestInspectScopedReportsTaskThatNeverStarts(t *testing.T) {
	queue := make(chan func(), 1)

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			queue <- scope.Task(
				"queued forever",
				func(context.Context) {},
			)
		},
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	if err != nil {
		t.Fatalf("InspectScoped returned an error: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected the pending task to violate shutdown")
	}

	if len(report.Survivors) != 0 {
		t.Fatalf("got %d goroutine survivors, want 0", len(report.Survivors))
	}

	if !reportHasViolation(report, ViolationTaskNeverStarted) {
		t.Errorf("report does not contain pending-task violation: %+v", report.Violations)
	}

	if len(report.Tasks) != 1 || report.Tasks[0].State != TaskPending {
		t.Fatalf("unexpected task report: %+v", report.Tasks)
	}

	failure := formatFailure(report)
	for _, fragment := range []string{
		`task "queued forever" never started`,
		`task "queued forever"`,
		"registered at:",
	} {
		if !strings.Contains(failure, fragment) {
			t.Errorf("failure does not contain %q:\n%s", fragment, failure)
		}
	}
}

func TestInspectScopedReportsCompletedTaskDescendant(t *testing.T) {
	ready := make(chan struct{})
	taskDone := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			scope.Go("parent task", func(context.Context) {
				defer close(taskDone)

				go func() {
					defer close(done)
					close(ready)
					<-release
				}()
			})

			<-ready
			<-taskDone
		},
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	close(release)
	<-done

	if err != nil {
		t.Fatalf("InspectScoped returned an error: %v", err)
	}

	if !reportHasViolation(report, ViolationTaskDescendantSurvived) {
		t.Errorf("report does not contain descendant violation: %+v", report.Violations)
	}

	if len(report.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(report.Tasks))
	}

	if report.Tasks[0].State != TaskCompleted {
		t.Errorf("got task state %q, want %q", report.Tasks[0].State, TaskCompleted)
	}

	if countGoroutines(report.Tasks[0].Survivors) != 1 {
		t.Errorf("unexpected attributed survivors: %+v", report.Tasks[0].Survivors)
	}
}

func TestInspectStartupTimeout(t *testing.T) {
	const startupTimeout = 10 * time.Millisecond

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	cause := make(chan error, 1)

	report, err := Inspect(
		t.Context(),
		func(ctx context.Context) {
			defer close(done)
			close(started)
			<-release
			cause <- context.Cause(ctx)
		},
		WithStartupTimeout(startupTimeout),
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	close(release)
	<-done

	if err != nil {
		t.Fatalf("Inspect returned an error: %v", err)
	}

	select {
	case <-started:
	default:
		t.Fatal("start function never ran")
	}

	if report.Passed() {
		t.Fatal("expected startup to time out")
	}

	if !reportHasViolation(report, ViolationStartupTimeout) {
		t.Errorf("report does not contain startup violation: %+v", report.Violations)
	}

	if report.StartupElapsed < startupTimeout {
		t.Errorf(
			"startup elapsed %s, want at least %s",
			report.StartupElapsed,
			startupTimeout,
		)
	}

	if got := <-cause; !errors.Is(got, ErrStartupTimeout) {
		t.Errorf("cancellation cause = %v, want %v", got, ErrStartupTimeout)
	}
}

func TestInspectScopedRejectsNilStart(t *testing.T) {
	_, err := InspectScoped(t.Context(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "nil scoped start function") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScopeTaskPanicsWhenInvokedTwice(t *testing.T) {
	scope := &Scope{
		ctx:      context.Background(),
		registry: &taskRegistry{},
	}
	task := scope.Task("one shot", func(context.Context) {})
	task()

	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("expected a panic")
		}

		if !strings.Contains(value.(string), "invoked more than once") {
			t.Fatalf("unexpected panic: %v", value)
		}
	}()

	task()
}

func TestScopeTaskPanicsForNilFunction(t *testing.T) {
	scope := &Scope{
		ctx:      context.Background(),
		registry: &taskRegistry{},
	}

	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("expected a panic")
		}

		if !strings.Contains(value.(string), "nil task function") {
			t.Fatalf("unexpected panic: %v", value)
		}
	}()

	scope.Task("invalid", nil)
}

func reportHasViolation(report Report, kind ViolationKind) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind {
			return true
		}
	}

	return false
}
