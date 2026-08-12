package ctxscope

import (
	"context"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope/internal/profiler"
)

func TestInspectScopedTracksTaskHierarchy(t *testing.T) {
	ready := make(chan struct{}, 3)

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			scope.Go("request", func(ctx context.Context) {
				ready <- struct{}{}

				scope.GoChild(ctx, "refresh account", func(ctx context.Context) {
					ready <- struct{}{}
					derived := context.WithValue(ctx, hierarchyTestContextKey{}, "value")
					persist := scope.TaskChild(
						derived,
						"persist cache",
						func(ctx context.Context) {
							ready <- struct{}{}
							<-ctx.Done()
						},
					)
					go persist()
					<-ctx.Done()
				})

				<-ctx.Done()
			})

			for range 3 {
				<-ready
			}
		},
		WithGrace(time.Second),
		WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("inspect hierarchy: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("expected hierarchy to stop cleanly: %+v", report)
	}

	tasks := tasksByName(report.Tasks)
	request := tasks["request"]
	refresh := tasks["refresh account"]
	persist := tasks["persist cache"]
	if len(tasks) != 3 {
		t.Fatalf("reported %d tasks, want 3: %+v", len(tasks), report.Tasks)
	}
	if request.ParentID != "" {
		t.Errorf("root parent ID = %q, want empty", request.ParentID)
	}
	if refresh.ParentID != request.ID {
		t.Errorf("refresh parent ID = %q, want %q", refresh.ParentID, request.ID)
	}
	if persist.ParentID != refresh.ID {
		t.Errorf("persist parent ID = %q, want %q", persist.ParentID, refresh.ID)
	}
	for _, task := range report.Tasks {
		if task.State != TaskCompleted {
			t.Errorf("task %q state = %q, want %q", task.Name, task.State, TaskCompleted)
		}
	}
}

func TestInspectScopedReportsPendingChildTask(t *testing.T) {
	registered := make(chan struct{})

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			scope.Go("request", func(ctx context.Context) {
				scope.TaskChild(ctx, "queued write", func(context.Context) {})
				close(registered)
			})
			<-registered
		},
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("inspect pending child: %v", err)
	}

	tasks := tasksByName(report.Tasks)
	request := tasks["request"]
	child := tasks["queued write"]
	if child.ParentID != request.ID {
		t.Fatalf("child parent ID = %q, want %q", child.ParentID, request.ID)
	}
	if child.State != TaskPending {
		t.Errorf("child state = %q, want %q", child.State, TaskPending)
	}
	if !reportHasViolation(report, ViolationTaskNeverStarted) {
		t.Errorf("report lacks pending-task violation: %+v", report.Violations)
	}

	failure := formatFailure(report)
	for _, fragment := range []string{
		"task hierarchy:",
		`"request" (id: `,
		`"queued write" (id: `,
		`parent: "request"`,
	} {
		if !strings.Contains(failure, fragment) {
			t.Errorf("failure does not contain %q:\n%s", fragment, failure)
		}
	}
}

func TestInspectScopedAttributesSurvivorToChildTask(t *testing.T) {
	childReady := make(chan struct{})
	childTaskDone := make(chan struct{})
	releaseChild := make(chan struct{})
	childDone := make(chan struct{})

	report, err := InspectScoped(
		t.Context(),
		func(scope *Scope) {
			scope.Go("request", func(ctx context.Context) {
				scope.GoChild(ctx, "audit write", func(context.Context) {
					defer close(childTaskDone)
					go func() {
						defer close(childDone)
						close(childReady)
						<-releaseChild
					}()
				})
				<-childTaskDone
			})
			<-childReady
		},
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)
	close(releaseChild)
	<-childDone

	if err != nil {
		t.Fatalf("inspect child survivor: %v", err)
	}

	tasks := tasksByName(report.Tasks)
	request := tasks["request"]
	audit := tasks["audit write"]
	if audit.ParentID != request.ID {
		t.Errorf("audit parent ID = %q, want %q", audit.ParentID, request.ID)
	}
	if countGoroutines(request.Survivors) != 0 {
		t.Errorf("root task received child survivors: %+v", request.Survivors)
	}
	if countGoroutines(audit.Survivors) != 1 {
		t.Errorf("child task has %d survivors, want 1", countGoroutines(audit.Survivors))
	}
	if !reportHasTaskViolation(report, ViolationTaskDescendantSurvived, audit.ID) {
		t.Errorf("report lacks child descendant violation: %+v", report.Violations)
	}
}

func TestScopeChildRejectsInvalidParentContext(t *testing.T) {
	scope := &Scope{
		ctx:      context.Background(),
		scopeID:  "scope-a",
		registry: &taskRegistry{},
	}

	tests := []struct {
		name        string
		parent      context.Context
		wantMessage string
	}{
		{
			name:        "nil",
			wantMessage: "nil parent task context",
		},
		{
			name:        "operation context",
			parent:      scope.Context(),
			wantMessage: "does not belong to a task",
		},
		{
			name: "different scope",
			parent: context.WithValue(
				context.Background(),
				taskContextKey{},
				taskContextValue{scopeID: "scope-b", taskID: "1"},
			),
			wantMessage: "different scope",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				value := recover()
				if value == nil {
					t.Fatal("expected a panic")
				}
				if !strings.Contains(value.(string), test.wantMessage) {
					t.Fatalf("panic = %q, want it to contain %q", value, test.wantMessage)
				}
			}()

			scope.GoChild(test.parent, "invalid child", func(context.Context) {})
		})
	}
}

func TestUnnamedChildClearsParentTaskNameLabel(t *testing.T) {
	scope := &Scope{
		ctx:      context.Background(),
		scopeID:  "label-scope",
		registry: &taskRegistry{},
	}

	var childLabel string
	parent := scope.Task("named parent", func(ctx context.Context) {
		child := scope.TaskChild(ctx, "", func(ctx context.Context) {
			childLabel, _ = pprof.Label(ctx, profiler.TaskNameLabel)
		})
		child()
	})
	parent()

	if childLabel != "" {
		t.Errorf("unnamed child task label = %q, want empty", childLabel)
	}
	tasks := scope.registry.snapshot()
	if len(tasks) != 2 || tasks[1].ParentID != tasks[0].ID {
		t.Fatalf("unexpected task hierarchy: %+v", tasks)
	}
}

type hierarchyTestContextKey struct{}

func tasksByName(tasks []TaskReport) map[string]TaskReport {
	byName := make(map[string]TaskReport, len(tasks))
	for _, task := range tasks {
		byName[task.Name] = task
	}
	return byName
}

func reportHasTaskViolation(report Report, kind ViolationKind, taskID string) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind && violation.TaskID == taskID {
			return true
		}
	}
	return false
}
