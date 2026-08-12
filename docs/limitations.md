# Limitations

`ctxscope` is a test-time cancellation checker, not a production goroutine
monitor.

## Start function contract

The start function must create the operation's goroutines with the supplied
context and then return. If the start function blocks forever, `Inspect` cannot
reach its cancellation phase.

Wait for an explicit readiness signal when startup is asynchronous. This makes
tests deterministic and ensures the goroutine exists before cancellation.

`WithStartupTimeout` prevents the inspection call from waiting forever and
reports `ViolationStartupTimeout`. Go cannot forcibly stop the blocked start
function, so the test must still arrange cleanup for an intentionally broken
example.

## Label inheritance

Only goroutines that inherit the inspection's pprof label can be reported. Work
started outside the labeled call tree is intentionally excluded. Code that
replaces runtime labels before starting descendants can also remove the scope
label.

Use `Scope.Task` when work is handed to a goroutine that already exists, such
as a worker pool. The returned wrapper is one-shot and must be registered
before the inspection can finish; ctxscope cannot discover arbitrary functions
retained by external systems without explicit registration.

Go does not expose the current goroutine's label set. After a scoped task,
`pprof.Do` restores labels from the inspection's parent context. This guarantees
that ctxscope's operation label does not remain on an ordinary pool worker. A
custom pool that maintains unrelated goroutine-local labels not represented in
that context should verify its own label-management behavior.

## Cooperative cancellation

Canceling a context does not terminate a goroutine. The goroutine must select
or receive from `ctx.Done()` and return. `ctxscope` observes that contract; it
does not force shutdown.

## Profile behavior

Goroutine profiles are runtime diagnostics. Identical stacks may be aggregated
into a single sample with `Count > 1`, and exact stack details can vary across
Go versions and compiler optimizations. Tests should assert meaningful function
fragments instead of entire stack traces.

## Timing

The grace period is a deadline, not an exact duration. Profile capture and
scheduler activity can make `Report.Elapsed` slightly longer. Use a grace period
that reflects the expected shutdown time and avoid values so small that normal
CI scheduling becomes significant.

The final failing observation begins at or after the grace deadline. Because
capturing the process goroutine profile takes time, `Report.Elapsed` includes
that final capture and can exceed the configured grace period. Registered task
states in the report are the states observed at the beginning of that capture.

Profile polling uses adaptive backoff. A successful shutdown may therefore be
observed up to the current polling interval after the final goroutine exits.

## Parallel tests

Concurrent inspections use distinct labels and are designed not to select each
other's goroutines. They still share the runtime profiler, so large amounts of
parallel profile capture can add test overhead.
