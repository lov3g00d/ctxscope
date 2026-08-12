# Task hierarchy

Task hierarchy records why registered work exists, not only whether it stopped.
A root task uses `Scope.Go` or `Scope.Task`. Work registered by that task uses
`Scope.GoChild` or `Scope.TaskChild`.

```text
HTTP request
└── refresh account
    └── write cache
```

## Immediate and queued children

`GoChild` starts the child in a new goroutine:

```go
scope.Go("HTTP request", func(ctx context.Context) {
	scope.GoChild(ctx, "refresh account", refreshAccount)
})
```

`TaskChild` returns a one-shot wrapper for a queue or worker pool:

```go
scope.Go("HTTP request", func(ctx context.Context) {
	pool.Submit(
		scope.TaskChild(ctx, "write cache", writeCache),
	)
})
```

Both methods register the child before returning. A queued child is therefore
visible as `pending` even if no worker ever starts it.

## Parent-context contract

Pass the context received by the parent task, or a context derived from it:

```go
scope.Go("request", func(ctx context.Context) {
	childContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	scope.GoChild(childContext, "bounded lookup", lookup)
})
```

The task context carries an internal scope ID and task ID. Child registration
panics when the context is nil, is only the operation context returned by
`Scope.Context`, or belongs to another inspection. Rejecting invalid parents is
preferable to silently producing a misleading root task.

A child inherits values, deadlines, and cancellation from the supplied parent
context. Its pprof task labels replace the parent's labels for the duration of
the child callback, so raw goroutines created by the child are attributed to
that child.

## Report model

Every `TaskReport` has an inspection-local `ID`. Nested tasks also contain
`ParentID`; root tasks leave it empty. Reports use a normalized list rather
than recursively embedding children, which keeps JSON compact and avoids
duplicating lifecycle and survivor data.

```json
{
  "schema_version": 2,
  "tasks": [
    {"id": "1", "name": "HTTP request", "state": "completed"},
    {
      "id": "2",
      "parent_id": "1",
      "name": "refresh account",
      "state": "completed"
    }
  ]
}
```

Task IDs are meaningful only inside one report. Consumers should join
`parent_id` to `id` within that report and should not persist IDs as global
identifiers.

## Lifecycle and attribution

Parent and child lifecycle states are independent. A parent callback may return
while a queued or running child remains active. ctxscope continues polling until
every registered task completes or the grace deadline is observed.

A registered child that leaves a raw goroutine behind receives the
`task_descendant_survived` violation. Its completed parent remains visible in
the hierarchy but does not receive the same survivor. Terminal failures and the
GitHub Actions renderer display the relationship so the causal path remains
clear.

See the [hierarchy example](../examples/hierarchy) for a cooperative three-level
tree and a detached-descendant failure.
