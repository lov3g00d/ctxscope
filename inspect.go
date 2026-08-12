package ctxscope

import (
	"context"
	"errors"
	"fmt"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lov3g00d/ctxscope/internal/profiler"
)

// ErrProbeCancellation is the cancellation cause used by Inspect after start
// returns.
var ErrProbeCancellation = errors.New("ctxscope: probe cancellation")

// ErrStartupTimeout is the cancellation cause used when the configured startup
// deadline expires before the start function returns.
var ErrStartupTimeout = errors.New("ctxscope: startup timeout")

var scopeSequence atomic.Uint64

// StartFunc starts the goroutines being inspected.
//
// It must start all operation-scoped goroutines with the supplied context and
// then return. Goroutines created while start runs inherit the scope label that
// Inspect uses to identify them.
type StartFunc func(context.Context)

// Inspect starts an operation, cancels its context, and reports lifecycle
// violations and goroutines that remain alive after the configured grace
// period.
func Inspect(parent context.Context, start StartFunc, options ...Option) (Report, error) {
	if start == nil {
		return Report{}, errors.New("ctxscope: nil start function")
	}

	return inspect(
		parent,
		func(
			_ context.Context,
			labeledContext context.Context,
			_ string,
			_ *taskRegistry,
		) {
			start(labeledContext)
		},
		options...,
	)
}

// InspectScoped starts an operation with a Scope, cancels its context, and
// reports registered tasks and labeled goroutines that violate the lifecycle
// contract.
func InspectScoped(
	parent context.Context,
	start ScopedStartFunc,
	options ...Option,
) (Report, error) {
	if start == nil {
		return Report{}, errors.New("ctxscope: nil scoped start function")
	}

	return inspect(
		parent,
		func(
			operationContext context.Context,
			_ context.Context,
			scopeID string,
			registry *taskRegistry,
		) {
			start(&Scope{
				ctx:      operationContext,
				scopeID:  scopeID,
				registry: registry,
			})
		},
		options...,
	)
}

type inspectionStart func(
	operationContext context.Context,
	labeledContext context.Context,
	scopeID string,
	registry *taskRegistry,
)

func inspect(
	parent context.Context,
	start inspectionStart,
	options ...Option,
) (Report, error) {
	if parent == nil {
		return Report{}, errors.New("ctxscope: nil parent context")
	}

	cfg, err := newConfig(options...)
	if err != nil {
		return Report{}, err
	}

	if err := parent.Err(); err != nil {
		return Report{}, fmt.Errorf("ctxscope: parent context already done: %w", err)
	}

	scopeID := nextScopeID()
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(ErrProbeCancellation)

	registry := &taskRegistry{}
	startedAt := time.Now()
	startupTimedOut := runStart(
		ctx,
		scopeID,
		registry,
		start,
		cfg.startupTimeout,
	)

	cancelledAt := time.Now()
	cause := ErrProbeCancellation
	if startupTimedOut {
		cause = ErrStartupTimeout
	}
	cancel(cause)

	survivors, tasks, elapsed, err := pollScope(
		scopeID,
		cfg,
		cancelledAt,
		registry,
	)
	if err != nil {
		return Report{}, fmt.Errorf(
			"ctxscope: inspect scope %q: %w",
			scopeID,
			err,
		)
	}

	converted := convertGoroutines(survivors)
	attributeSurvivors(tasks, converted)

	return Report{
		SchemaVersion:  ReportSchemaVersion,
		ScopeID:        scopeID,
		Name:           cfg.name,
		Grace:          cfg.grace,
		StartupElapsed: cancelledAt.Sub(startedAt),
		Elapsed:        elapsed,
		CanceledAt:     cancelledAt,
		Survivors:      converted,
		Tasks:          tasks,
		Violations: buildViolations(
			startupTimedOut,
			converted,
			tasks,
		),
	}, nil
}

func runStart(
	ctx context.Context,
	scopeID string,
	registry *taskRegistry,
	start inspectionStart,
	timeout time.Duration,
) bool {
	run := func() {
		pprof.Do(
			ctx,
			pprof.Labels(profiler.ScopeLabel, scopeID),
			func(labeledContext context.Context) {
				start(ctx, labeledContext, scopeID, registry)
			},
		)
	}

	if timeout == 0 {
		run()
		return false
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		run()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}

func nextScopeID() string {
	sequence := scopeSequence.Add(1)
	return strconv.FormatUint(sequence, 10)
}

func pollScope(
	scopeID string,
	cfg config,
	cancelledAt time.Time,
	registry *taskRegistry,
) ([]profiler.Goroutine, []TaskReport, time.Duration, error) {
	return pollScopeWith(
		scopeID,
		cfg,
		cancelledAt,
		registry,
		profiler.CaptureScope,
		time.Now,
		time.Sleep,
	)
}

type scopeCapture func(string) ([]profiler.Goroutine, error)

func pollScopeWith(
	scopeID string,
	cfg config,
	cancelledAt time.Time,
	registry *taskRegistry,
	capture scopeCapture,
	now func() time.Time,
	sleep func(time.Duration),
) ([]profiler.Goroutine, []TaskReport, time.Duration, error) {
	deadline := cancelledAt.Add(cfg.grace)
	pollInterval := cfg.pollInterval

	for {
		observationStartedAt := now()
		finalObservation := !observationStartedAt.Before(deadline)
		var tasks []TaskReport
		if finalObservation {
			// Keep final task state and the profile anchored to the same
			// post-deadline observation.
			tasks = registry.snapshot()
		}

		survivors, err := capture(scopeID)
		if err != nil {
			return nil, nil, now().Sub(cancelledAt), err
		}

		observationCompletedAt := now()
		elapsed := observationCompletedAt.Sub(cancelledAt)

		if finalObservation {
			if len(survivors) == 0 && !hasActiveTasks(tasks) {
				return nil, tasks, elapsed, nil
			}

			return survivors, tasks, elapsed, nil
		}
		// A capture that crosses the deadline may contain pre-deadline stacks.
		// Discard it and make the next observation entirely post-deadline.
		if !observationCompletedAt.Before(deadline) {
			continue
		}
		if len(survivors) == 0 && registry.active() == 0 {
			tasks = registry.snapshot()
			if !hasActiveTasks(tasks) {
				return nil, tasks, elapsed, nil
			}
		}

		sleepFor := min(pollInterval, deadline.Sub(observationCompletedAt))
		if sleepFor > 0 {
			sleep(sleepFor)
		}

		pollInterval = nextPollInterval(
			pollInterval,
			cfg.maxPollInterval,
		)
	}
}

func nextPollInterval(current, maximum time.Duration) time.Duration {
	if current >= maximum {
		return maximum
	}

	if current > maximum/2 {
		return maximum
	}

	return current * 2
}

func convertGoroutines(source []profiler.Goroutine) []Goroutine {
	if len(source) == 0 {
		return nil
	}

	converted := make([]Goroutine, len(source))

	for index, goroutine := range source {
		converted[index] = Goroutine{
			Count:  goroutine.Count,
			Labels: goroutine.Labels,
			Frames: convertFrames(goroutine.Frames),
		}
	}

	return converted
}

func convertFrames(source []profiler.Frame) []Frame {
	converted := make([]Frame, len(source))

	for index, frame := range source {
		converted[index] = Frame{
			Function: frame.Function,
			File:     frame.File,
			Line:     frame.Line,
		}
	}

	return converted
}

func attributeSurvivors(tasks []TaskReport, survivors []Goroutine) {
	byID := make(map[string]int, len(tasks))
	for index, task := range tasks {
		byID[task.ID] = index
	}

	for _, survivor := range survivors {
		for _, taskID := range survivor.Labels[profiler.TaskLabel] {
			index, exists := byID[taskID]
			if !exists {
				continue
			}

			tasks[index].Survivors = append(
				tasks[index].Survivors,
				survivor,
			)
		}
	}
}

func buildViolations(
	startupTimedOut bool,
	survivors []Goroutine,
	tasks []TaskReport,
) []Violation {
	var violations []Violation

	if startupTimedOut {
		violations = append(violations, Violation{
			Kind: ViolationStartupTimeout,
		})
	}
	if len(survivors) > 0 || hasActiveTasks(tasks) {
		violations = append(violations, Violation{
			Kind: ViolationShutdownTimeout,
		})
	}

	for _, task := range tasks {
		kind := ViolationKind("")
		switch {
		case task.State == TaskPending:
			kind = ViolationTaskNeverStarted
		case task.State == TaskRunning:
			kind = ViolationTaskStillRunning
		case task.State == TaskCompleted && len(task.Survivors) > 0:
			kind = ViolationTaskDescendantSurvived
		}

		if kind != "" {
			violations = append(violations, Violation{
				Kind:     kind,
				TaskID:   task.ID,
				TaskName: task.Name,
			})
		}
	}

	return violations
}

func hasActiveTasks(tasks []TaskReport) bool {
	for _, task := range tasks {
		if task.State != TaskCompleted {
			return true
		}
	}

	return false
}
