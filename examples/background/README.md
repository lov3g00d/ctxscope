# Named background tasks

`Scope.Go` is the convenient form of `go scope.Task(...)()`. It registers a
named task, starts it in a new goroutine, and passes the inspection context to
the task function.

The happy-path test starts three independent background components and waits
until all three are ready. When `InspectScoped` cancels the operation, every
component observes `ctx.Done()`, returns, and is reported as `completed`.

The non-happy-path test models a common ownership bug: a named task starts a
child goroutine and returns without arranging for that child to stop. Because
goroutines inherit pprof labels, ctxscope attributes the survivor to the
completed parent task and emits `task_descendant_survived`.

Run the examples:

```bash
go test -count=1 -v ./examples/background
go test -race -count=1 ./examples/background
```
