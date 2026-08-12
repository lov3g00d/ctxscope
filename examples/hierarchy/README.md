# Task hierarchy example

This example shows how a scoped task can register children while preserving the
causal path in the report.

- `TestTaskHierarchyStopsCleanly` builds a three-level hierarchy containing a
  queued grandchild. Every task observes cancellation and completes.
- `TestTaskHierarchyAttributesDetachedDescendant` leaves a raw goroutine under
  a child task. The report attributes the survivor to that child and retains
  its parent relationship.

Run both paths:

```bash
go test -v ./examples/hierarchy
```

Use `GoChild` for a child that should start immediately and `TaskChild` for a
child submitted to a worker pool or queue. Both require the context passed to
the parent task, or a context derived from it.
