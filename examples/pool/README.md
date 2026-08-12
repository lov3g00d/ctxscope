# Reusable worker pool

This example uses `Scope.Task` to carry ctxscope ownership across a job queue.
The pool workers are started before inspection, so ordinary goroutine
inheritance cannot associate them with the operation under test.

`scope.Task` solves that boundary in two steps:

1. It registers the named task before the function enters the queue.
2. When a worker invokes the wrapper, it applies the operation and task labels
   to that worker for the duration of the task.

The passing test submits three named tasks to three reusable workers. Every
task waits for `ctx.Done()` and becomes `completed` when ctxscope cancels the
operation.

The non-happy-path tests demonstrate all pool-specific lifecycle failures:

- `queued email` remains `pending` because a previous job occupies the worker;
- `stuck export` remains `running` because it ignores cancellation;
- `index documents` becomes `completed` but leaves a child goroutine behind.

All tests release their deliberately blocked goroutines after inspection, so
the normal suite does not leak.

Run the examples:

```bash
go test -count=1 -v ./examples/pool
go test -race -count=1 ./examples/pool
```

Run the opt-in live failure demo to see `VerifyScoped` print the task
registration stack and the surviving descendant stack:

```bash
go test \
  -tags=ctxscope_demo_failure \
  -run '^TestFailureDemoReportsCompletedTaskDescendant$' \
  -count=1 \
  -v \
  ./examples/pool
```

The demo is expected to exit with status `1`.
