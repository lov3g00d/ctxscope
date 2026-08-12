package ctxscope

import (
	"context"
	"fmt"
	"runtime/pprof"
	"sync/atomic"

	"github.com/lov3g00d/ctxscope/internal/profiler"
)

// ScopedStartFunc registers and starts work through a Scope.
type ScopedStartFunc func(*Scope)

// Scope carries an operation context and tracks named tasks that may cross an
// asynchronous boundary such as a worker pool or job queue.
type Scope struct {
	ctx      context.Context
	scopeID  string
	registry *taskRegistry
}

// Context returns the operation context canceled by the inspection.
func (scope *Scope) Context() context.Context {
	return scope.ctx
}

// Task registers a named one-shot task and returns a function suitable for a
// worker pool or job queue. Registration happens before Task returns, so an
// inspection can report work that remains queued and never starts.
//
// The returned function must be called at most once.
func (scope *Scope) Task(
	name string,
	function func(context.Context),
) func() {
	return scope.newTask(name, function, captureRegistrationFrames())
}

// Go registers a named task and starts it in a new goroutine.
func (scope *Scope) Go(name string, function func(context.Context)) {
	task := scope.newTask(name, function, captureRegistrationFrames())
	go task()
}

func (scope *Scope) newTask(
	name string,
	function func(context.Context),
	registrationStack []Frame,
) func() {
	if function == nil {
		panic("ctxscope: nil task function")
	}

	record := scope.registry.register(name, registrationStack)
	var invoked atomic.Bool

	return func() {
		if !invoked.CompareAndSwap(false, true) {
			panic(fmt.Sprintf("ctxscope: task %q invoked more than once", name))
		}

		scope.registry.start(record)
		defer scope.registry.complete(record)

		labels := pprof.Labels(
			profiler.ScopeLabel,
			scope.scopeID,
			profiler.TaskLabel,
			record.id,
		)
		if name != "" {
			labels = pprof.Labels(
				profiler.ScopeLabel,
				scope.scopeID,
				profiler.TaskLabel,
				record.id,
				profiler.TaskNameLabel,
				name,
			)
		}

		pprof.Do(scope.ctx, labels, function)
	}
}
