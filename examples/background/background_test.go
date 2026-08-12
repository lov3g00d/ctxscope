package background_test

import (
	"context"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func TestNamedBackgroundTasksStopWhenCanceled(t *testing.T) {
	ready := make(chan string, 3)

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			for _, name := range []string{"cache refresh", "metrics flush", "lease renewal"} {
				scope.Go(name, func(ctx context.Context) {
					ready <- name
					<-ctx.Done()
				})
			}

			for range 3 {
				<-ready
			}
		},
		ctxscope.WithName("background services"),
		ctxscope.WithGrace(time.Second),
	)
	if err != nil {
		t.Fatalf("inspect background tasks: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("expected every background task to stop: %+v", report)
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

func TestDetachedChildIsAttributedToItsParentTask(t *testing.T) {
	childReady := make(chan struct{})
	childDone := make(chan struct{})
	parentDone := make(chan struct{})
	releaseChild := make(chan struct{})

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			scope.Go("configuration watcher", func(context.Context) {
				defer close(parentDone)
				go func() {
					defer close(childDone)
					close(childReady)
					<-releaseChild
				}()
			})

			<-childReady
			<-parentDone
		},
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)

	close(releaseChild)
	<-childDone

	if err != nil {
		t.Fatalf("inspect background task: %v", err)
	}
	if !hasViolation(report, ctxscope.ViolationTaskDescendantSurvived) {
		t.Fatalf("missing descendant violation: %+v", report.Violations)
	}
	if len(report.Tasks) != 1 {
		t.Fatalf("reported %d tasks, want 1", len(report.Tasks))
	}

	task := report.Tasks[0]
	if task.Name != "configuration watcher" {
		t.Fatalf("task name = %q, want %q", task.Name, "configuration watcher")
	}
	if task.State != ctxscope.TaskCompleted {
		t.Fatalf("task state = %q, want %q", task.State, ctxscope.TaskCompleted)
	}
	if len(task.Survivors) == 0 {
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
