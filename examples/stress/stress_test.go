package stress_test

import (
	"context"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func TestStressSummarizesHealthyShutdowns(t *testing.T) {
	report, err := ctxscope.Stress(
		t.Context(),
		5,
		func(int) ctxscope.StartFunc {
			ready := make(chan struct{})

			return func(ctx context.Context) {
				go func() {
					close(ready)
					<-ctx.Done()
				}()
				<-ready
			}
		},
		ctxscope.WithName("healthy consumer"),
		ctxscope.WithGrace(time.Second),
	)
	if err != nil {
		t.Fatalf("stress healthy consumer: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("expected every run to pass: %+v", report)
	}
	if report.PassedRuns != 5 || report.FailedRuns != 0 {
		t.Fatalf("unexpected run counts: %+v", report)
	}
	if report.Latency.Min > report.Latency.P50 ||
		report.Latency.P50 > report.Latency.P95 ||
		report.Latency.P95 > report.Latency.Max {
		t.Fatalf("latency percentiles are not ordered: %+v", report.Latency)
	}
}

func TestStressCountsIntermittentCancellationFailures(t *testing.T) {
	var cleanup []func()
	defer func() {
		for _, stop := range cleanup {
			stop()
		}
	}()

	report, err := ctxscope.Stress(
		t.Context(),
		6,
		func(iteration int) ctxscope.StartFunc {
			ready := make(chan struct{})
			release := make(chan struct{})
			done := make(chan struct{})
			cleanup = append(cleanup, func() {
				close(release)
				<-done
			})

			return func(ctx context.Context) {
				go func() {
					defer close(done)
					close(ready)
					if iteration%3 == 2 {
						<-release
						return
					}
					<-ctx.Done()
				}()
				<-ready
			}
		},
		ctxscope.WithName("occasionally stuck consumer"),
		ctxscope.WithGrace(100*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("stress intermittent consumer: %v", err)
	}
	if report.Passed() {
		t.Fatal("expected intermittent failures")
	}
	if report.PassedRuns != 4 || report.FailedRuns != 2 {
		t.Fatalf("unexpected run counts: %+v", report)
	}
	if report.Reports[2].Passed() || report.Reports[5].Passed() {
		t.Fatal("expected iterations 2 and 5 to fail")
	}
}

func TestStressScopedCountsDroppedJobs(t *testing.T) {
	report, err := ctxscope.StressScoped(
		t.Context(),
		4,
		func(iteration int) ctxscope.ScopedStartFunc {
			return func(scope *ctxscope.Scope) {
				if iteration%2 == 1 {
					scope.Task("dropped job", func(context.Context) {})
					return
				}

				ready := make(chan struct{})
				scope.Go("delivered job", func(ctx context.Context) {
					close(ready)
					<-ctx.Done()
				})
				<-ready
			}
		},
		ctxscope.WithGrace(100*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("stress scoped jobs: %v", err)
	}
	if report.PassedRuns != 2 || report.FailedRuns != 2 {
		t.Fatalf("unexpected run counts: %+v", report)
	}

	for _, iteration := range []int{1, 3} {
		if !hasViolation(report.Reports[iteration], ctxscope.ViolationTaskNeverStarted) {
			t.Errorf("iteration %d lacks never-started violation", iteration)
		}
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
