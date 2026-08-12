# Comparison with other Go concurrency-testing tools

`ctxscope` is an operation-scoped cancellation-contract checker. It answers:

> Did the goroutines owned by this operation stop within the allowed time after
> its context was canceled?

That is narrower than general goroutine leak detection. The tools below are
complementary rather than interchangeable.

| Tool | Scope | What it checks | Best fit |
| --- | --- | --- | --- |
| `ctxscope` | Labeled operation goroutines and explicitly registered tasks | They start and stop within lifecycle deadlines | Cancellation contracts across goroutines, pools, and queues |
| [`goleak`](https://github.com/uber-go/goleak) | Unexpected goroutines in the test process | No unignored goroutines remain | Package-wide leak safety net |
| [`testing/synctest`](https://pkg.go.dev/testing/synctest) | Goroutines inside an isolated bubble | Bubble goroutines exit without deadlock | Deterministic, self-contained concurrency tests |
| [Gomega `gleak`](https://onsi.github.io/gomega/#gleak-experimental) | Process goroutines selected by matchers and snapshots | No matching goroutines remain | Ginkgo and Gomega suites |
| [`leaktest`](https://github.com/fortytw2/leaktest) | Goroutines added after a snapshot | Newly observed goroutines disappear | Existing suites already using `leaktest` |
| Go `goroutineleak` profile | Provably permanently blocked goroutines | A blocked goroutine cannot possibly wake | Runtime and production diagnostics |

## The same cancellation bug

Assume `runWorker` starts a goroutine that ignores `ctx.Done()` and remains
blocked.

With `ctxscope`, the ownership boundary and shutdown deadline are explicit:

```go
ctxscope.Verify(
	t,
	func(ctx context.Context) {
		go runWorker(ctx)
		<-workerReady
	},
	ctxscope.WithName("message worker"),
	ctxscope.WithGrace(100*time.Millisecond),
)
```

The failure contains only survivors carrying this inspection's label, along
with their captured stack frames.

For a pre-existing worker pool, `Scope.Task` registers work before submission
and reapplies the label inside the worker. This also exposes pending work that
has not become a goroutine and therefore cannot appear in a process profile.

With `goleak`, the equivalent safety check observes the whole process:

```go
func TestWorker(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	go runWorker(ctx)
	<-workerReady
	cancel()
}
```

This is concise and mature. Its per-test checker cannot associate goroutines
with individual tests running through `t.Parallel`; `VerifyTestMain` is the
recommended package-wide alternative in that situation.

With `testing/synctest`, self-contained concurrent code can run in a
deterministic bubble:

```go
synctest.Test(t, func(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	go func() {
		defer close(done)
		runWorker(ctx)
	}()

	cancel()
	synctest.Wait()

	select {
	case <-done:
	default:
		t.Fatal("worker ignored cancellation")
	}
})
```

`synctest` also provides fake time and deadlock detection. It is usually the
better choice when the whole test can live inside its isolated model. Tests
that depend on real networking, external processes, or other non-bubble work
may instead need an ordinary integration environment.

## Why the runtime leak profile is different

The [Go 1.26 release notes](https://go.dev/doc/go1.26) describe the experimental
`goroutineleak` pprof profile, enabled with
`GOEXPERIMENT=goroutineleakprofile`. It detects a class of goroutines blocked
on concurrency primitives that can no longer be reached by code capable of
unblocking them.

A goroutine that ignores cancellation but will wake after a 30-second timer is
not necessarily permanently leaked. It can still violate an operation's
100-millisecond shutdown requirement. `ctxscope` checks that bounded shutdown
contract; the runtime profile checks whether a blocked goroutine can ever wake.

The [Go 1.27 draft release notes](https://go.dev/doc/go1.27) say the profile is
planned to become generally available in Go 1.27.

## Recommended combinations

- Use `ctxscope` for an operation whose context-cancellation behavior is part
  of its contract.
- Use `testing/synctest` for deterministic tests that fit inside a bubble.
- Use `goleak.VerifyTestMain` as a broad package-level safety net.
- Use runtime profiles to investigate goroutines in CI or production.

`ctxscope` should not be presented as a replacement for all leak detectors. Its
specific value is attributing survivors to one labeled operation and enforcing
that operation's cancellation deadline.
