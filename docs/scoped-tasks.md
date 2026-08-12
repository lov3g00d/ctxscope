# Scoped tasks

Goroutines created while `Inspect` runs its start function inherit the
inspection's pprof label. Work submitted to a pre-existing worker pool crosses
that inheritance boundary: the worker goroutine already exists and therefore
cannot inherit a label when it is created.

`InspectScoped` and `VerifyScoped` handle this case with explicit, named task
registration.

## Worker-pool example

```go
ctxscope.VerifyScoped(
	t,
	func(scope *ctxscope.Scope) {
		pool.Submit(
			scope.Task("refresh cache", func(ctx context.Context) {
				refreshCache(ctx)
			}),
		)
	},
	ctxscope.WithName("cache shutdown"),
	ctxscope.WithStartupTimeout(time.Second),
	ctxscope.WithGrace(250*time.Millisecond),
)
```

Calling `scope.Task` registers the work immediately. The returned function is a
one-shot wrapper that:

1. Changes the task from `pending` to `running`.
2. Reapplies the operation label to the goroutine executing the wrapper.
3. Adds task ID and task name pprof labels.
4. Calls the task with the operation context.
5. Marks the task `completed` when it returns.
6. Removes the inspection labels from the pool worker before it accepts its
   next job.

Because registration happens before submission, ctxscope can report a task
that remains in a queue and never obtains a worker. A goroutine profile alone
cannot report such work because a pending function is not a goroutine yet.

## Starting a named goroutine

Use `Scope.Go` when a separate queue is not needed:

```go
ctxscope.VerifyScoped(t, func(scope *ctxscope.Scope) {
	scope.Go("message consumer", func(ctx context.Context) {
		consumeMessages(ctx)
	})
})
```

This provides task lifecycle information and a registration stack in addition
to ordinary survivor stacks.

## Failure categories

`Report.Violations` contains typed failures:

| Kind | Meaning |
| --- | --- |
| `startup_timeout` | The start function did not return before the startup deadline. |
| `shutdown_timeout` | Operation work survived the shutdown grace period. |
| `task_never_started` | A registered task remained pending. |
| `task_still_running` | A task function was still executing. |
| `task_descendant_survived` | A task returned but one of its labeled descendants remained. |

`Report.Tasks` includes registration, start, and completion timestamps. A task
that has attributable survivors also contains them in `TaskReport.Survivors`.
The top-level `Report.Survivors` remains available for callers that do not need
task-level grouping.

All report fields are exported, so reports can be encoded directly with
`encoding/json` for CI artifacts or other tooling. Duration fields use explicit
`*_ns` JSON names and contain nanoseconds. `schema_version` allows integrations
to detect future report-format changes.

## Task contract

The function returned by `Scope.Task` is one-shot. Invoking it more than once
panics because a second execution would make one task ID represent multiple
independent lifecycles. Register a new task for every queued execution.

Task functions should use the context passed to them rather than retaining a
different application context:

```go
scope.Task("flush", func(ctx context.Context) {
	flush(ctx)
})
```

Canceling the context does not forcibly terminate a task. The task remains
responsible for observing `ctx.Done()` and returning.

## Repeated verification

Use `StressScoped` when a shutdown race is intermittent:

```go
stress, err := ctxscope.StressScoped(
	t.Context(),
	100,
	func(iteration int) ctxscope.ScopedStartFunc {
		pool := newPool()
		return func(scope *ctxscope.Scope) {
			pool.Submit(scope.Task("flush", pool.Flush))
		}
	},
	ctxscope.WithGrace(100*time.Millisecond),
)
```

Each factory call must return startup logic with fresh operation state. The
runner executes inspections sequentially and reports failure counts plus min,
p50, p95, and max post-cancellation latency. Use `Stress` for the ordinary
context-only API.
