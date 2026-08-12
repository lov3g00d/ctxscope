package pool_test

import (
	"context"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
	"github.com/lov3g00d/ctxscope/examples/pool"
)

func TestPoolTasksStopWhenCanceled(t *testing.T) {
	workers := pool.New(3, 3)
	defer workers.Close()

	ready := make(chan struct{}, 3)
	var submitErr error

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			for _, name := range []string{"refresh cache", "flush metrics", "renew lease"} {
				submitErr = workers.Submit(
					scope.Context(),
					scope.Task(name, func(ctx context.Context) {
						ready <- struct{}{}
						<-ctx.Done()
					}),
				)
				if submitErr != nil {
					return
				}
			}

			for range 3 {
				<-ready
			}
		},
		ctxscope.WithName("service shutdown"),
		ctxscope.WithGrace(time.Second),
	)
	if err != nil {
		t.Fatalf("inspect pool: %v", err)
	}
	if submitErr != nil {
		t.Fatalf("submit task: %v", submitErr)
	}
	if !report.Passed() {
		t.Fatalf("expected all pool tasks to stop: %+v", report)
	}
	if len(report.Tasks) != 3 {
		t.Fatalf("reported %d tasks, want 3", len(report.Tasks))
	}

	for _, task := range report.Tasks {
		if task.State != ctxscope.TaskCompleted {
			t.Errorf("task %q state = %q, want %q", task.Name, task.State, ctxscope.TaskCompleted)
		}
	}
}

func TestPoolReportsTaskThatNeverStarts(t *testing.T) {
	workers := pool.New(1, 1)
	blockerReady := make(chan struct{})
	releaseBlocker := make(chan struct{})

	if err := workers.Submit(t.Context(), func() {
		close(blockerReady)
		<-releaseBlocker
	}); err != nil {
		t.Fatalf("submit blocker: %v", err)
	}
	<-blockerReady

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			if submitErr := workers.Submit(
				scope.Context(),
				scope.Task("queued email", func(context.Context) {}),
			); submitErr != nil {
				panic(submitErr)
			}
		},
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)

	close(releaseBlocker)
	workers.Close()

	if err != nil {
		t.Fatalf("inspect pool: %v", err)
	}
	if report.Passed() {
		t.Fatal("expected the queued task to be reported")
	}
	if !hasViolation(report, ctxscope.ViolationTaskNeverStarted) {
		t.Fatalf("missing never-started violation: %+v", report.Violations)
	}
	if len(report.Tasks) != 1 || report.Tasks[0].State != ctxscope.TaskPending {
		t.Fatalf("unexpected task lifecycle: %+v", report.Tasks)
	}
}

func TestPoolReportsTaskStillRunning(t *testing.T) {
	workers := pool.New(1, 1)
	ready := make(chan struct{})
	release := make(chan struct{})

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			if submitErr := workers.Submit(
				scope.Context(),
				scope.Task("stuck export", func(context.Context) {
					close(ready)
					<-release
				}),
			); submitErr != nil {
				panic(submitErr)
			}
			<-ready
		},
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)

	close(release)
	workers.Close()

	if err != nil {
		t.Fatalf("inspect pool: %v", err)
	}
	if !hasViolation(report, ctxscope.ViolationTaskStillRunning) {
		t.Fatalf("missing still-running violation: %+v", report.Violations)
	}
	if !hasViolation(report, ctxscope.ViolationShutdownTimeout) {
		t.Fatalf("missing shutdown violation: %+v", report.Violations)
	}
	if len(report.Tasks) != 1 || report.Tasks[0].State != ctxscope.TaskRunning {
		t.Fatalf("unexpected task lifecycle: %+v", report.Tasks)
	}
	if len(report.Tasks[0].Survivors) == 0 {
		t.Fatal("running task has no attributed survivor stack")
	}
}

func TestPoolReportsDescendantOfCompletedTask(t *testing.T) {
	workers := pool.New(1, 1)
	childReady := make(chan struct{})
	childDone := make(chan struct{})
	taskDone := make(chan struct{})
	releaseChild := make(chan struct{})

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			if submitErr := workers.Submit(
				scope.Context(),
				scope.Task("index documents", func(context.Context) {
					defer close(taskDone)
					go func() {
						defer close(childDone)
						close(childReady)
						<-releaseChild
					}()
				}),
			); submitErr != nil {
				panic(submitErr)
			}

			<-childReady
			<-taskDone
		},
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)

	close(releaseChild)
	<-childDone
	workers.Close()

	if err != nil {
		t.Fatalf("inspect pool: %v", err)
	}
	if !hasViolation(report, ctxscope.ViolationTaskDescendantSurvived) {
		t.Fatalf("missing descendant violation: %+v", report.Violations)
	}
	if len(report.Tasks) != 1 || report.Tasks[0].State != ctxscope.TaskCompleted {
		t.Fatalf("unexpected task lifecycle: %+v", report.Tasks)
	}
	if len(report.Tasks[0].Survivors) == 0 {
		t.Fatal("completed task has no attributed descendant stack")
	}
}

func hasViolation(report ctxscope.Report, kind ctxscope.ViolationKind) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind {
			return true
		}
	}

	return false
}
