# Worker examples

This package demonstrates both sides of a cancellation contract.

## Happy path

`TestHappyPathWorkerStopsWhenCanceled` starts a worker that selects on
`ctx.Done()`. `ctxscope.Verify` cancels the context, observes the worker exit,
and leaves the test passing.

```go
ctxscope.Verify(
	t,
	func(ctx context.Context) {
		go service.Run(ctx)
		<-service.Ready()
	},
	ctxscope.WithName("job worker"),
)
```

## Non-happy path

`TestNonHappyPathWorkerThatIgnoresCancellationIsReported` starts a deliberately
broken worker that blocks on its own channel and ignores `ctx.Done()`.

The example uses `Inspect` instead of `Verify` because the surviving goroutine
is expected. It asserts that the report fails and contains the worker's `Run`
stack frame. The test then releases the worker with `defer`, so the example does
not leak a real goroutine.

In an application test using `Verify`, the same behavior would fail the test
and print the survivor stack.

## Live failure demo

`failure_demo_test.go` is guarded by the `ctxscope_demo_failure` build tag, so
normal tests and CI remain green. Run it explicitly to see `Verify` fail with a
real survivor stack trace:

```bash
go test \
  -tags=ctxscope_demo_failure \
  -run '^TestFailureDemoPrintsSurvivorStack$' \
  -count=1 \
  -v \
  ./examples/worker
```

The command is expected to exit with status `1`. Its report identifies the
operation, grace period, survivor count, blocking runtime calls, application
function, source file, and source line.

Representative output:

```text
ctxscope: operation "stuck message consumer" left 1 goroutine(s) after a 20ms grace period
scope ID: 1

violations:
  - operation work exceeded the shutdown grace period

survivor sample 1 (goroutines: 1)
  runtime.gopark
    .../runtime/proc.go:462
  runtime.chanrecv
    .../runtime/chan.go:667
  github.com/lov3g00d/ctxscope/examples/worker_test.TestFailureDemoPrintsSurvivorStack.func2.1
    .../examples/worker/failure_demo_test.go:29
```

The application frame points to `<-release`, the receive operation keeping the
goroutine alive.

Run the scoped task demo to see both the queue submission stack and the current
worker stack:

```bash
go test \
  -tags=ctxscope_demo_failure \
  -run '^TestScopedFailureDemoPrintsTaskDiagnostics$' \
  -count=1 \
  -v \
  ./examples/worker
```

This command also exits with status `1`. The report names `stuck cache refresh`,
marks it as `running`, shows where `Scope.Task` registered it, and attributes
the surviving pre-existing pool goroutine to the task.

```bash
go test -count=1 -v ./examples/worker
go test -race -count=1 ./examples/worker
```
