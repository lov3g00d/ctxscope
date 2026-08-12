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

type taskContextKey struct{}

type taskContextValue struct {
	scopeID string
	taskID  string
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
	return scope.newTask(
		scope.ctx,
		"",
		name,
		function,
		captureRegistrationFrames(),
	)
}

// Go registers a named task and starts it in a new goroutine.
func (scope *Scope) Go(name string, function func(context.Context)) {
	task := scope.newTask(
		scope.ctx,
		"",
		name,
		function,
		captureRegistrationFrames(),
	)
	go task()
}

// TaskChild registers a named one-shot child of the task represented by
// parent and returns a function suitable for a worker pool or job queue.
// Parent must be the context passed to a task in this Scope, or a context
// derived from it.
//
// The returned function must be called at most once. TaskChild panics if
// parent is invalid or function is nil.
func (scope *Scope) TaskChild(
	parent context.Context,
	name string,
	function func(context.Context),
) func() {
	parentID := scope.parentTaskID(parent)
	return scope.newTask(
		parent,
		parentID,
		name,
		function,
		captureRegistrationFrames(),
	)
}

// GoChild registers a named child of the task represented by parent and starts
// it in a new goroutine. Parent must be the context passed to a task in this
// Scope, or a context derived from it. GoChild panics if parent is invalid or
// function is nil.
func (scope *Scope) GoChild(
	parent context.Context,
	name string,
	function func(context.Context),
) {
	parentID := scope.parentTaskID(parent)
	task := scope.newTask(
		parent,
		parentID,
		name,
		function,
		captureRegistrationFrames(),
	)
	go task()
}

func (scope *Scope) newTask(
	parent context.Context,
	parentID string,
	name string,
	function func(context.Context),
	registrationStack []Frame,
) func() {
	if function == nil {
		panic("ctxscope: nil task function")
	}

	record := scope.registry.register(parentID, name, registrationStack)
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
			profiler.TaskNameLabel,
			name,
		)

		taskContext := context.WithValue(
			parent,
			taskContextKey{},
			taskContextValue{
				scopeID: scope.scopeID,
				taskID:  record.id,
			},
		)
		pprof.Do(taskContext, labels, function)
	}
}

func (scope *Scope) parentTaskID(parent context.Context) string {
	if parent == nil {
		panic("ctxscope: nil parent task context")
	}

	value, ok := parent.Value(taskContextKey{}).(taskContextValue)
	if !ok || value.taskID == "" {
		panic("ctxscope: parent context does not belong to a task")
	}
	if value.scopeID != scope.scopeID {
		panic("ctxscope: parent context belongs to a different scope")
	}

	return value.taskID
}
