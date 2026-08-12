# Design

`ctxscope` identifies goroutines by scope rather than by comparing the global
goroutine count before and after a test.

## Inspection lifecycle

```text
Inspect(parent, start)
        |
        v
create cancellable context
        |
        v
apply unique pprof scope label
        |
        v
run start(ctx) and inherit label in child goroutines
        |
        v
cancel ctx
        |
        v
poll profile and registered tasks until empty or grace deadline
        |
        v
return Report
```

`runtime/pprof.Do` associates labels with the goroutine executing `start`.
Goroutines created from that execution inherit its labels. Each inspection uses
an atomic process-local sequence number as the label value.

After `start` returns, `Inspect` cancels the child context with
`ErrProbeCancellation`. It captures the binary goroutine profile and parses it
with `github.com/google/pprof/profile`. Samples whose scope label matches the
inspection are converted into the public report model.

## Scoped tasks

`InspectScoped` creates a task registry alongside the labeled context. Calling
`Scope.Task` records pending work and its registration stack before returning a
one-shot function. When a worker invokes that function, `pprof.Do` reapplies
the scope label and adds task labels to the worker goroutine for the duration
of the call.

The `Scope` retains the operation context without the inspection's temporary
scope label. Task wrappers use that context as the parent of their own
`pprof.Do` call. When the task returns, `pprof.Do` restores the parent labels
instead of leaving the ctxscope label attached to the reusable pool worker.

The registry closes a gap in profile-only detection: queued work has no
goroutine stack until a worker begins executing it. Polling ends only when both
the profile contains no scope survivors and every registered task has
completed.

Task callbacks receive a context containing an unexported identity for their
scope and task ID. `GoChild` and `TaskChild` validate that identity before
registering a child. The child wrapper derives its execution context from the
supplied parent context, records the parent ID, and replaces the parent task's
pprof task label with its own. Context values, deadlines, and cancellation
still propagate, while surviving goroutines are attributed to the deepest
registered task that owns them. A context from another inspection is rejected,
which prevents cross-scope task trees.

Profile polling starts at `WithPollInterval` and doubles up to
`WithMaxPollInterval`. This keeps detection responsive for fast shutdown while
reducing repeated whole-process profile captures for longer grace periods.

The final polling observation snapshots registered task state before capturing
the goroutine profile. A failed deadline observation is accepted only when that
observation began at or after the grace deadline. If a profile capture begins
before the deadline and finishes after it, ctxscope discards that capture and
immediately performs a fresh observation. This prevents pre-deadline stacks
from being combined with post-capture task state in the final report.

## Why profiles

The runtime profile provides both the labels needed for selection and the stack
frames needed for diagnostics. A process-wide goroutine count cannot identify
ownership and is easily disturbed by unrelated test or runtime activity.

Profiles can combine goroutines with identical stacks and labels. For that
reason, `Report.Survivors` contains profile samples and each sample has a
`Count`.

## API layers

- `Verify` integrates with `testing.TB` and turns a failed inspection into a
  test failure.
- `VerifyScoped` adds named task lifecycle tracking for pools and queues.
- `Inspect` returns structured data for custom assertions and tooling.
- `InspectScoped` returns task reports and typed lifecycle violations.
- `internal/profiler` contains runtime profile capture and parsing so the
  public API does not expose pprof parser types.

## Concurrency properties

Scope IDs come from `atomic.Uint64`, so concurrent inspections receive distinct
IDs. Each inspection has its own context, deadline, and profile filter. Runtime
profile capture is process-wide, but filtering keeps reports scoped to their
own labels. `TestInspectIsolatesConcurrentScopes` runs overlapping inspections
and verifies that each report contains only its own labeled worker.

`TestConcurrentScopedInspectionsSharePoolWithoutCrossTalk` proves the same
property for registered tasks running through one shared pool, and
`TestScopedTaskRestoresWorkerLabels` verifies that subsequent unwrapped work on
the same worker does not retain the completed inspection's label.
