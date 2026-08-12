package ctxscope

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStressSummarizesSuccessfulRuns(t *testing.T) {
	report, err := Stress(
		t.Context(),
		3,
		func(int) StartFunc {
			ready := make(chan struct{})

			return func(ctx context.Context) {
				go func() {
					close(ready)
					<-ctx.Done()
				}()
				<-ready
			}
		},
		WithGrace(time.Second),
		WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Stress returned an error: %v", err)
	}

	if !report.Passed() {
		t.Fatalf("expected every run to pass: %+v", report)
	}

	if report.Runs != 3 || report.PassedRuns != 3 || report.FailedRuns != 0 {
		t.Errorf("unexpected run counts: %+v", report)
	}

	if len(report.Reports) != 3 {
		t.Errorf("got %d reports, want 3", len(report.Reports))
	}

	if report.Latency.Min > report.Latency.P50 ||
		report.Latency.P50 > report.Latency.P95 ||
		report.Latency.P95 > report.Latency.Max {
		t.Errorf("latency percentiles are not ordered: %+v", report.Latency)
	}
}

func TestStressScopedCountsFailedRuns(t *testing.T) {
	report, err := StressScoped(
		t.Context(),
		2,
		func(iteration int) ScopedStartFunc {
			return func(scope *Scope) {
				scope.Task(
					"pending task",
					func(context.Context) {},
				)
			}
		},
		WithGrace(5*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("StressScoped returned an error: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected stress report to fail")
	}

	if report.PassedRuns != 0 || report.FailedRuns != 2 {
		t.Errorf("unexpected run counts: %+v", report)
	}

	for _, inspection := range report.Reports {
		if !reportHasViolation(inspection, ViolationTaskNeverStarted) {
			t.Errorf("inspection lacks pending-task violation: %+v", inspection)
		}
	}
}

func TestStressRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		runs        int
		factory     StartFactory
		wantMessage string
	}{
		{
			name:        "zero runs",
			factory:     func(int) StartFunc { return func(context.Context) {} },
			wantMessage: "runs must be greater than zero",
		},
		{
			name:        "nil factory",
			runs:        1,
			wantMessage: "nil start factory",
		},
		{
			name: "nil iteration start",
			runs: 1,
			factory: func(int) StartFunc {
				return nil
			},
			wantMessage: "returned nil for iteration 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Stress(t.Context(), test.runs, test.factory)
			if err == nil {
				t.Fatal("expected an error")
			}

			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error %q does not contain %q", err, test.wantMessage)
			}
		})
	}
}

func TestSummarizeLatencies(t *testing.T) {
	stats := summarizeLatencies([]time.Duration{
		100 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		20 * time.Millisecond,
	})

	if stats.Min != 10*time.Millisecond {
		t.Errorf("minimum = %s, want 10ms", stats.Min)
	}

	if stats.P50 != 20*time.Millisecond {
		t.Errorf("p50 = %s, want 20ms", stats.P50)
	}

	if stats.P95 != 100*time.Millisecond {
		t.Errorf("p95 = %s, want 100ms", stats.P95)
	}

	if stats.Max != 100*time.Millisecond {
		t.Errorf("maximum = %s, want 100ms", stats.Max)
	}
}
