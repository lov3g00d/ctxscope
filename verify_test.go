package ctxscope

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestVerifyPassesWhenGoroutineStops(t *testing.T) {
	reporter := &recordingReporter{}
	ready := make(chan struct{})
	done := make(chan struct{})

	verify(
		reporter,
		t.Context(),
		func(ctx context.Context) {
			go func() {
				defer close(done)

				close(ready)
				<-ctx.Done()
			}()

			<-ready
		},
		WithName("cancellable worker"),
		WithGrace(time.Second),
		WithPollInterval(time.Millisecond),
	)

	if len(reporter.fatals) != 0 {
		t.Fatalf("unexpected fatal reports: %v", reporter.fatals)
	}

	if len(reporter.errors) != 0 {
		t.Fatalf("unexpected error reports: %v", reporter.errors)
	}

	select {
	case <-done:
	default:
		t.Error("worker has not stopped")
	}
}

func TestVerifyScopedPassesWhenTaskStops(t *testing.T) {
	done := make(chan struct{})
	ready := make(chan struct{})

	VerifyScoped(
		t,
		func(scope *Scope) {
			scope.Go("cancellable task", func(ctx context.Context) {
				defer close(done)
				close(ready)
				<-ctx.Done()
			})
			<-ready
		},
		WithGrace(time.Second),
		WithPollInterval(time.Millisecond),
	)

	select {
	case <-done:
	default:
		t.Error("scoped task has not stopped")
	}
}

func TestVerifyReportsSurvivingGoroutine(t *testing.T) {
	reporter := &recordingReporter{}
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	verify(
		reporter,
		t.Context(),
		func(context.Context) {
			go func() {
				defer close(done)

				close(ready)
				<-release
			}()

			<-ready
		},
		WithName("blocked worker"),
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	close(release)
	<-done

	if len(reporter.fatals) != 0 {
		t.Fatalf("unexpected fatal reports: %v", reporter.fatals)
	}

	if len(reporter.errors) != 1 {
		t.Fatalf(
			"got %d error reports, want 1: %v",
			len(reporter.errors),
			reporter.errors,
		)
	}

	for _, fragment := range []string{
		`operation "blocked worker"`,
		"left 1 goroutine(s)",
		"survivor sample 1",
		"TestVerifyReportsSurvivingGoroutine",
	} {
		if !strings.Contains(reporter.errors[0], fragment) {
			t.Errorf(
				"failure report does not contain %q:\n%s",
				fragment,
				reporter.errors[0],
			)
		}
	}
}

func TestVerifyReportsInspectionError(t *testing.T) {
	reporter := &recordingReporter{}

	verify(reporter, t.Context(), nil)

	if len(reporter.fatals) != 1 {
		t.Fatalf(
			"got %d fatal reports, want 1: %v",
			len(reporter.fatals),
			reporter.fatals,
		)
	}

	if !strings.Contains(reporter.fatals[0], "nil start function") {
		t.Errorf(
			"fatal report does not describe the error: %q",
			reporter.fatals[0],
		)
	}
}

func TestVerifyScopedReportsPendingTask(t *testing.T) {
	reporter := &recordingReporter{}

	verifyScoped(
		reporter,
		t.Context(),
		func(scope *Scope) {
			scope.Task("queued forever", func(context.Context) {})
		},
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	if len(reporter.fatals) != 0 {
		t.Fatalf("unexpected fatal reports: %v", reporter.fatals)
	}

	if len(reporter.errors) != 1 {
		t.Fatalf(
			"got %d error reports, want 1: %v",
			len(reporter.errors),
			reporter.errors,
		)
	}

	for _, fragment := range []string{
		`task "queued forever" never started`,
		"registered at:",
		"TestVerifyScopedReportsPendingTask",
	} {
		if !strings.Contains(reporter.errors[0], fragment) {
			t.Errorf(
				"failure report does not contain %q:\n%s",
				fragment,
				reporter.errors[0],
			)
		}
	}
}

func TestFormatViolation(t *testing.T) {
	tests := []struct {
		name      string
		violation Violation
		want      string
	}{
		{
			name:      "startup timeout",
			violation: Violation{Kind: ViolationStartupTimeout},
			want:      "start function exceeded the startup timeout",
		},
		{
			name:      "shutdown timeout",
			violation: Violation{Kind: ViolationShutdownTimeout},
			want:      "operation work exceeded the shutdown grace period",
		},
		{
			name: "unnamed pending task uses ID",
			violation: Violation{
				Kind:   ViolationTaskNeverStarted,
				TaskID: "7",
			},
			want: `task "7" never started`,
		},
		{
			name: "running task",
			violation: Violation{
				Kind:     ViolationTaskStillRunning,
				TaskName: "flush",
			},
			want: `task "flush" is still running`,
		},
		{
			name: "surviving descendant",
			violation: Violation{
				Kind:     ViolationTaskDescendantSurvived,
				TaskName: "audit",
			},
			want: `task "audit" completed but left a descendant goroutine`,
		},
		{
			name:      "future violation kind",
			violation: Violation{Kind: ViolationKind("future_kind")},
			want:      "future_kind",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatViolation(test.violation); got != test.want {
				t.Errorf("formatViolation() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFormatFailureHandlesTaskParentCycle(t *testing.T) {
	report := Report{
		ScopeID: "cycle-scope",
		Grace:   time.Second,
		Tasks: []TaskReport{
			{ID: "1", ParentID: "2", Name: "first", State: TaskPending},
			{ID: "2", ParentID: "1", Name: "second", State: TaskPending},
		},
		Violations: []Violation{{Kind: ViolationShutdownTimeout}},
	}

	failure := formatFailure(report)
	for _, name := range []string{"first", "second"} {
		if count := strings.Count(failure, "- \""+name+"\" (id:"); count != 1 {
			t.Errorf("hierarchy contains %q %d times, want once:\n%s", name, count, failure)
		}
	}
}

func TestFormatFailureShowsOnlyFailedTaskBranches(t *testing.T) {
	report := Report{
		ScopeID: "filtered-scope",
		Grace:   time.Second,
		Tasks: []TaskReport{
			{ID: "1", Name: "request", State: TaskCompleted},
			{ID: "2", ParentID: "1", Name: "healthy sibling", State: TaskCompleted},
			{ID: "3", ParentID: "1", Name: "queued write", State: TaskPending},
			{ID: "4", Name: "unrelated cleanup", State: TaskCompleted},
		},
		Violations: []Violation{
			{Kind: ViolationTaskNeverStarted, TaskID: "3", TaskName: "queued write"},
		},
	}

	failure := formatFailure(report)
	for _, fragment := range []string{
		`- "request" (id: 1, state: completed)`,
		`- "queued write" (id: 3, state: pending)`,
		"2 completed tasks omitted",
	} {
		if !strings.Contains(failure, fragment) {
			t.Errorf("failure does not contain %q:\n%s", fragment, failure)
		}
	}
	for _, omitted := range []string{"healthy sibling", "unrelated cleanup"} {
		if strings.Contains(failure, omitted) {
			t.Errorf("failure unexpectedly contains completed task %q:\n%s", omitted, failure)
		}
	}
}

func TestFormatFailureQuotesTaskHierarchyNames(t *testing.T) {
	name := "worker\n\x1b[31mred"
	report := Report{
		ScopeID: "quoted-scope",
		Grace:   time.Second,
		Tasks: []TaskReport{
			{ID: "1", Name: name, State: TaskPending},
		},
		Violations: []Violation{
			{Kind: ViolationTaskNeverStarted, TaskID: "1", TaskName: name},
		},
	}

	failure := formatFailure(report)
	if !strings.Contains(failure, `- "worker\n\x1b[31mred" (id: 1, state: pending)`) {
		t.Errorf("hierarchy does not contain the quoted task name:\n%s", failure)
	}
	if strings.Contains(failure, "- worker\n") || strings.Contains(failure, "\x1b") {
		t.Errorf("hierarchy contains an unescaped task name:\n%s", failure)
	}
}

type recordingReporter struct {
	helperCalls int
	errors      []string
	fatals      []string
}

func (reporter *recordingReporter) Helper() {
	reporter.helperCalls++
}

func (reporter *recordingReporter) Errorf(
	format string,
	arguments ...any,
) {
	reporter.errors = append(
		reporter.errors,
		fmt.Sprintf(format, arguments...),
	)
}

func (reporter *recordingReporter) Fatalf(
	format string,
	arguments ...any,
) {
	reporter.fatals = append(
		reporter.fatals,
		fmt.Sprintf(format, arguments...),
	)
}
