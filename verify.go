package ctxscope

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Verify inspects the goroutines started by start and fails the current test
// when startup or shutdown violates the configured lifecycle contract.
//
// Verify uses the context associated with t. Use Inspect directly when the
// operation needs a different parent context or when the caller needs access
// to the resulting Report.
func Verify(t testing.TB, start StartFunc, options ...Option) {
	t.Helper()
	verify(t, t.Context(), start, options...)
}

// VerifyScoped inspects named tasks registered through a Scope and fails the
// current test when startup or shutdown violates the configured lifecycle
// contract.
func VerifyScoped(
	t testing.TB,
	start ScopedStartFunc,
	options ...Option,
) {
	t.Helper()
	verifyScoped(t, t.Context(), start, options...)
}

type testReporter interface {
	Helper()
	Errorf(format string, arguments ...any)
	Fatalf(format string, arguments ...any)
}

func verify(
	t testReporter,
	parent context.Context,
	start StartFunc,
	options ...Option,
) {
	t.Helper()

	report, err := Inspect(parent, start, options...)
	reportInspection(t, report, err)
}

func verifyScoped(
	t testReporter,
	parent context.Context,
	start ScopedStartFunc,
	options ...Option,
) {
	t.Helper()

	report, err := InspectScoped(parent, start, options...)
	reportInspection(t, report, err)
}

func reportInspection(t testReporter, report Report, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("ctxscope: verify: %v", err)
		return
	}

	if report.Passed() {
		return
	}

	t.Errorf("%s", formatFailure(report))
}

func formatFailure(report Report) string {
	var output strings.Builder

	name := report.Name
	if name == "" {
		name = "unnamed operation"
	}

	survivorCount := countGoroutines(report.Survivors)
	if survivorCount > 0 {
		fmt.Fprintf(
			&output,
			"ctxscope: operation %q left %d goroutine(s) after a %s grace period",
			name,
			survivorCount,
			report.Grace,
		)
	} else {
		fmt.Fprintf(
			&output,
			"ctxscope: operation %q violated its lifecycle contract after a %s grace period",
			name,
			report.Grace,
		)
	}

	fmt.Fprintf(&output, "\nscope ID: %s", report.ScopeID)

	if len(report.Violations) > 0 {
		output.WriteString("\n\nviolations:")
		for _, violation := range report.Violations {
			fmt.Fprintf(
				&output,
				"\n  - %s",
				formatViolation(violation),
			)
		}
	}

	for _, task := range report.Tasks {
		if task.State == TaskCompleted && len(task.Survivors) == 0 {
			continue
		}

		name := task.Name
		if name == "" {
			name = "unnamed task"
		}

		fmt.Fprintf(
			&output,
			"\n\ntask %q (id: %s, state: %s)",
			name,
			task.ID,
			task.State,
		)

		if len(task.RegistrationStack) > 0 {
			output.WriteString("\n  registered at:")
			frame := task.RegistrationStack[0]
			fmt.Fprintf(
				&output,
				"\n    %s\n      %s:%d",
				frame.Function,
				frame.File,
				frame.Line,
			)
		}
	}

	for index, survivor := range report.Survivors {
		fmt.Fprintf(
			&output,
			"\n\nsurvivor sample %d (goroutines: %d)",
			index+1,
			survivor.Count,
		)

		if len(survivor.Frames) == 0 {
			output.WriteString("\n  <stack unavailable>")
			continue
		}

		for _, frame := range survivor.Frames {
			fmt.Fprintf(
				&output,
				"\n  %s\n    %s:%d",
				frame.Function,
				frame.File,
				frame.Line,
			)
		}
	}

	return output.String()
}

func formatViolation(violation Violation) string {
	switch violation.Kind {
	case ViolationStartupTimeout:
		return "start function exceeded the startup timeout"
	case ViolationShutdownTimeout:
		return "operation work exceeded the shutdown grace period"
	case ViolationTaskNeverStarted:
		return fmt.Sprintf("task %q never started", violationName(violation))
	case ViolationTaskStillRunning:
		return fmt.Sprintf("task %q is still running", violationName(violation))
	case ViolationTaskDescendantSurvived:
		return fmt.Sprintf(
			"task %q completed but left a descendant goroutine",
			violationName(violation),
		)
	default:
		return string(violation.Kind)
	}
}

func violationName(violation Violation) string {
	if violation.TaskName != "" {
		return violation.TaskName
	}

	return violation.TaskID
}

func countGoroutines(survivors []Goroutine) int64 {
	var count int64

	for _, survivor := range survivors {
		count += survivor.Count
	}

	return count
}
