package ctxscope

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInspectRejectsInvalidParentContext(t *testing.T) {
	start := func(context.Context) {}

	t.Run("nil", func(t *testing.T) {
		//lint:ignore SA1012 nil is the invalid input under test
		_, err := Inspect(nil, start)
		if err == nil || !strings.Contains(err.Error(), "nil parent context") {
			t.Fatalf("error = %v, want nil parent context error", err)
		}
	})

	t.Run("already canceled", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Inspect(parent, start)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}

func TestInspectPassesWhenGoroutineStops(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan struct{})

	report, err := Inspect(
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

	if err != nil {
		t.Fatalf("Inspect returned an error: %v", err)
	}

	if !report.Passed() {
		t.Fatalf(
			"expected report to pass, got %d survivors",
			len(report.Survivors),
		)
	}

	if report.Name != "cancellable worker" {
		t.Errorf(
			"got report name %q, want %q",
			report.Name,
			"cancellable worker",
		)
	}

	if report.ScopeID == "" {
		t.Error("expected a non-empty scope ID")
	}

	select {
	case <-done:
	default:
		t.Error("worker has not stopped")
	}
}

func TestInspectReportsSurvivingGoroutine(t *testing.T) {
	const grace = 20 * time.Millisecond

	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	report, err := Inspect(
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
		WithGrace(grace),
		WithPollInterval(time.Millisecond),
	)

	// Release the intentionally blocked worker before making assertions.
	close(release)
	<-done

	if err != nil {
		t.Fatalf("Inspect returned an error: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected report to fail")
	}

	if len(report.Survivors) == 0 {
		t.Fatal("expected at least one surviving goroutine")
	}

	if report.Name != "blocked worker" {
		t.Errorf(
			"got report name %q, want %q",
			report.Name,
			"blocked worker",
		)
	}

	if report.Elapsed < grace {
		t.Errorf(
			"inspection elapsed %s, want at least %s",
			report.Elapsed,
			grace,
		)
	}

	if !framesContain(
		report.Survivors[0].Frames,
		"TestInspectReportsSurvivingGoroutine",
	) {
		t.Errorf(
			"survivor frames do not contain test function: %+v",
			report.Survivors[0].Frames,
		)
	}
}

func TestInspectReportsSurvivingNestedDescendant(t *testing.T) {
	ready := make(chan struct{})
	parentDone := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	report, err := Inspect(
		t.Context(),
		func(context.Context) {
			go func() {
				defer close(parentDone)

				go func() {
					defer close(done)
					close(ready)
					<-release
				}()
			}()

			<-ready
			<-parentDone
		},
		WithName("nested worker"),
		WithGrace(20*time.Millisecond),
		WithPollInterval(time.Millisecond),
	)

	close(release)
	<-done

	if err != nil {
		t.Fatalf("Inspect returned an error: %v", err)
	}

	if report.Passed() {
		t.Fatal("expected the nested descendant to survive")
	}

	if countGoroutines(report.Survivors) != 1 {
		t.Fatalf(
			"got %d surviving goroutines, want 1",
			countGoroutines(report.Survivors),
		)
	}
}

func TestInspectIsolatesConcurrentScopes(t *testing.T) {
	t.Parallel()
	parent := t.Context()

	type result struct {
		name   string
		report Report
		err    error
	}

	var workersReady sync.WaitGroup
	workersReady.Add(2)

	results := make(chan result, 2)
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	doneFirst := make(chan struct{})
	doneSecond := make(chan struct{})

	run := func(name string, release <-chan struct{}, done chan<- struct{}) {
		report, err := Inspect(
			parent,
			func(context.Context) {
				go func() {
					defer close(done)
					workersReady.Done()
					<-release
				}()

				workersReady.Wait()
			},
			WithName(name),
			WithGrace(20*time.Millisecond),
			WithPollInterval(time.Millisecond),
		)

		results <- result{name: name, report: report, err: err}
	}

	go run("first operation", releaseFirst, doneFirst)
	go run("second operation", releaseSecond, doneSecond)

	got := map[string]result{}
	for range 2 {
		inspection := <-results
		got[inspection.name] = inspection
	}

	close(releaseFirst)
	close(releaseSecond)
	<-doneFirst
	<-doneSecond

	first := got["first operation"]
	second := got["second operation"]

	if first.err != nil {
		t.Fatalf("first inspection returned an error: %v", first.err)
	}

	if second.err != nil {
		t.Fatalf("second inspection returned an error: %v", second.err)
	}

	if first.report.ScopeID == second.report.ScopeID {
		t.Fatalf("concurrent inspections share scope ID %q", first.report.ScopeID)
	}

	assertSingleScopedSurvivor(t, first.report)
	assertSingleScopedSurvivor(t, second.report)
}

func assertSingleScopedSurvivor(t *testing.T, report Report) {
	t.Helper()

	if report.Passed() {
		t.Fatalf("operation %q unexpectedly passed", report.Name)
	}

	if countGoroutines(report.Survivors) != 1 {
		t.Fatalf(
			"operation %q reported %d goroutines, want 1",
			report.Name,
			countGoroutines(report.Survivors),
		)
	}

	for _, survivor := range report.Survivors {
		if !slices.Contains(survivor.Labels["ctxscope.scope"], report.ScopeID) {
			t.Errorf(
				"operation %q survivor labels %v do not contain scope ID %q",
				report.Name,
				survivor.Labels,
				report.ScopeID,
			)
		}
	}
}

func framesContain(frames []Frame, fragment string) bool {
	for _, frame := range frames {
		if strings.Contains(frame.Function, fragment) {
			return true
		}
	}

	return false
}

func TestNextPollInterval(t *testing.T) {
	tests := []struct {
		name    string
		current time.Duration
		maximum time.Duration
		want    time.Duration
	}{
		{
			name:    "doubles",
			current: 5 * time.Millisecond,
			maximum: 40 * time.Millisecond,
			want:    10 * time.Millisecond,
		},
		{
			name:    "caps without overshooting",
			current: 30 * time.Millisecond,
			maximum: 40 * time.Millisecond,
			want:    40 * time.Millisecond,
		},
		{
			name:    "remains capped",
			current: 40 * time.Millisecond,
			maximum: 40 * time.Millisecond,
			want:    40 * time.Millisecond,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := nextPollInterval(test.current, test.maximum)
			if got != test.want {
				t.Errorf("next interval = %s, want %s", got, test.want)
			}
		})
	}
}
