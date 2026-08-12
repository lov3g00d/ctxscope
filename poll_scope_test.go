package ctxscope

import (
	"errors"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope/internal/profiler"
)

func TestPollScopeReturnsCaptureError(t *testing.T) {
	cancelledAt := time.Unix(1_700_000_000, 0)
	wantErr := errors.New("capture failed")

	survivors, tasks, elapsed, err := pollScopeWith(
		"error-scope",
		config{
			grace:           10 * time.Millisecond,
			pollInterval:    time.Millisecond,
			maxPollInterval: time.Millisecond,
		},
		cancelledAt,
		&taskRegistry{},
		func(string) ([]profiler.Goroutine, error) { return nil, wantErr },
		func() time.Time { return cancelledAt },
		func(time.Duration) {},
	)

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if survivors != nil || tasks != nil {
		t.Fatalf("unexpected partial result: survivors=%v tasks=%v", survivors, tasks)
	}
	if elapsed != 0 {
		t.Errorf("elapsed = %s, want 0", elapsed)
	}
}

func TestPollScopeDiscardsObservationThatCrossesDeadline(t *testing.T) {
	cancelledAt := time.Unix(1_700_000_000, 0)
	current := cancelledAt
	captures := 0

	survivors, tasks, elapsed, err := pollScopeWith(
		"deadline-scope",
		config{
			grace:           10 * time.Millisecond,
			pollInterval:    time.Millisecond,
			maxPollInterval: time.Millisecond,
		},
		cancelledAt,
		&taskRegistry{},
		func(string) ([]profiler.Goroutine, error) {
			captures++
			if captures == 1 {
				current = current.Add(12 * time.Millisecond)
				return []profiler.Goroutine{{Count: 1}}, nil
			}

			current = current.Add(time.Millisecond)
			return nil, nil
		},
		func() time.Time { return current },
		func(duration time.Duration) { current = current.Add(duration) },
	)
	if err != nil {
		t.Fatalf("poll scope: %v", err)
	}

	if captures != 2 {
		t.Fatalf("profile captures = %d, want 2", captures)
	}
	if len(survivors) != 0 {
		t.Fatalf("stale pre-deadline survivors were returned: %+v", survivors)
	}
	if len(tasks) != 0 {
		t.Fatalf("unexpected task reports: %+v", tasks)
	}
	if elapsed != 13*time.Millisecond {
		t.Errorf("elapsed = %s, want 13ms", elapsed)
	}
}

func TestPollScopeRequiresPostDeadlineObservationAfterCrossing(t *testing.T) {
	cancelledAt := time.Unix(1_700_000_000, 0)
	current := cancelledAt
	captures := 0

	survivors, _, _, err := pollScopeWith(
		"deadline-scope",
		config{
			grace:           10 * time.Millisecond,
			pollInterval:    time.Millisecond,
			maxPollInterval: time.Millisecond,
		},
		cancelledAt,
		&taskRegistry{},
		func(string) ([]profiler.Goroutine, error) {
			captures++
			current = current.Add(11 * time.Millisecond)
			return nil, nil
		},
		func() time.Time { return current },
		func(duration time.Duration) { current = current.Add(duration) },
	)
	if err != nil {
		t.Fatalf("poll scope: %v", err)
	}

	if captures != 2 {
		t.Fatalf("profile captures = %d, want 2", captures)
	}
	if len(survivors) != 0 {
		t.Fatalf("unexpected survivors: %+v", survivors)
	}
}

func TestPollScopeKeepsTaskStateFromFinalObservationStart(t *testing.T) {
	cancelledAt := time.Unix(1_700_000_000, 0)
	current := cancelledAt.Add(10 * time.Millisecond)
	registry := &taskRegistry{}
	record := registry.register("finishing task", nil)
	registry.start(record)

	survivors, tasks, _, err := pollScopeWith(
		"task-scope",
		config{
			grace:           10 * time.Millisecond,
			pollInterval:    time.Millisecond,
			maxPollInterval: time.Millisecond,
		},
		cancelledAt,
		registry,
		func(string) ([]profiler.Goroutine, error) {
			registry.complete(record)
			current = current.Add(time.Millisecond)
			return []profiler.Goroutine{
				{
					Count: 1,
					Labels: map[string][]string{
						profiler.TaskLabel: {record.id},
					},
				},
			}, nil
		},
		func() time.Time { return current },
		func(duration time.Duration) { current = current.Add(duration) },
	)
	if err != nil {
		t.Fatalf("poll scope: %v", err)
	}

	converted := convertGoroutines(survivors)
	attributeSurvivors(tasks, converted)
	violations := buildViolations(false, converted, tasks)

	if len(tasks) != 1 {
		t.Fatalf("task reports = %d, want 1", len(tasks))
	}
	if tasks[0].State != TaskRunning {
		t.Fatalf("task state = %q, want %q", tasks[0].State, TaskRunning)
	}
	if !violationsContain(violations, ViolationTaskStillRunning) {
		t.Errorf("violations lack %q: %+v", ViolationTaskStillRunning, violations)
	}
	if violationsContain(violations, ViolationTaskDescendantSurvived) {
		t.Errorf("task was misclassified as leaving a descendant: %+v", violations)
	}
}

func TestPollScopeReportsTrueCompletedTaskDescendant(t *testing.T) {
	cancelledAt := time.Unix(1_700_000_000, 0)
	current := cancelledAt.Add(10 * time.Millisecond)
	registry := &taskRegistry{}
	record := registry.register("parent task", nil)
	registry.start(record)
	registry.complete(record)

	survivors, tasks, _, err := pollScopeWith(
		"descendant-scope",
		config{
			grace:           10 * time.Millisecond,
			pollInterval:    time.Millisecond,
			maxPollInterval: time.Millisecond,
		},
		cancelledAt,
		registry,
		func(string) ([]profiler.Goroutine, error) {
			current = current.Add(time.Millisecond)
			return []profiler.Goroutine{
				{
					Count: 1,
					Labels: map[string][]string{
						profiler.TaskLabel: {record.id},
					},
				},
			}, nil
		},
		func() time.Time { return current },
		func(duration time.Duration) { current = current.Add(duration) },
	)
	if err != nil {
		t.Fatalf("poll scope: %v", err)
	}

	converted := convertGoroutines(survivors)
	attributeSurvivors(tasks, converted)
	violations := buildViolations(false, converted, tasks)

	if len(tasks) != 1 || tasks[0].State != TaskCompleted {
		t.Fatalf("unexpected task reports: %+v", tasks)
	}
	if !violationsContain(violations, ViolationTaskDescendantSurvived) {
		t.Errorf(
			"violations lack %q: %+v",
			ViolationTaskDescendantSurvived,
			violations,
		)
	}
}

func violationsContain(violations []Violation, kind ViolationKind) bool {
	for _, violation := range violations {
		if violation.Kind == kind {
			return true
		}
	}

	return false
}
