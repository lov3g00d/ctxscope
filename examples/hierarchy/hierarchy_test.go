package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func TestTaskHierarchyStopsCleanly(t *testing.T) {
	jobs := make(chan func())
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		(<-jobs)()
	}()

	ready := make(chan struct{}, 3)
	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			scope.Go("HTTP request", func(ctx context.Context) {
				ready <- struct{}{}
				scope.GoChild(ctx, "refresh account", func(ctx context.Context) {
					ready <- struct{}{}
					jobs <- scope.TaskChild(ctx, "write cache", func(ctx context.Context) {
						ready <- struct{}{}
						<-ctx.Done()
					})
					<-ctx.Done()
				})
				<-ctx.Done()
			})

			for range 3 {
				<-ready
			}
		},
		ctxscope.WithName("account refresh"),
		ctxscope.WithGrace(time.Second),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	<-workerDone

	if err != nil {
		t.Fatalf("inspect account refresh: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("expected every task to stop: %+v", report)
	}

	tasks := taskIndex(report.Tasks)
	if tasks["HTTP request"].ParentID != "" {
		t.Errorf("root task unexpectedly has a parent: %+v", tasks["HTTP request"])
	}
	if tasks["refresh account"].ParentID != tasks["HTTP request"].ID {
		t.Errorf("refresh task has the wrong parent: %+v", tasks["refresh account"])
	}
	if tasks["write cache"].ParentID != tasks["refresh account"].ID {
		t.Errorf("cache task has the wrong parent: %+v", tasks["write cache"])
	}
}

func TestTaskHierarchyAttributesDetachedDescendant(t *testing.T) {
	childReady := make(chan struct{})
	childTaskDone := make(chan struct{})
	detachedDone := make(chan struct{})
	releaseDetached := make(chan struct{})

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			scope.Go("HTTP request", func(ctx context.Context) {
				scope.GoChild(ctx, "audit event", func(context.Context) {
					defer close(childTaskDone)
					go func() {
						defer close(detachedDone)
						close(childReady)
						<-releaseDetached
					}()
				})
				<-childTaskDone
			})
			<-childReady
		},
		ctxscope.WithName("request shutdown"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	close(releaseDetached)
	<-detachedDone

	if err != nil {
		t.Fatalf("inspect request shutdown: %v", err)
	}
	if report.Passed() {
		t.Fatal("expected the detached descendant to be reported")
	}

	tasks := taskIndex(report.Tasks)
	request := tasks["HTTP request"]
	audit := tasks["audit event"]
	if audit.ParentID != request.ID {
		t.Errorf("audit task has the wrong parent: %+v", audit)
	}
	if survivorCount(request.Survivors) != 0 {
		t.Errorf("request task received its child's survivor: %+v", request.Survivors)
	}
	if survivorCount(audit.Survivors) != 1 {
		t.Errorf("audit task has %d survivors, want 1", survivorCount(audit.Survivors))
	}
	if !hasTaskViolation(
		report,
		ctxscope.ViolationTaskDescendantSurvived,
		audit.ID,
	) {
		t.Errorf("report lacks the audit descendant violation: %+v", report.Violations)
	}
}

func taskIndex(tasks []ctxscope.TaskReport) map[string]ctxscope.TaskReport {
	indexed := make(map[string]ctxscope.TaskReport, len(tasks))
	for _, task := range tasks {
		indexed[task.Name] = task
	}
	return indexed
}

func survivorCount(survivors []ctxscope.Goroutine) int64 {
	var count int64
	for _, survivor := range survivors {
		count += survivor.Count
	}
	return count
}

func hasTaskViolation(
	report ctxscope.Report,
	kind ctxscope.ViolationKind,
	taskID string,
) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind && violation.TaskID == taskID {
			return true
		}
	}
	return false
}
