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
