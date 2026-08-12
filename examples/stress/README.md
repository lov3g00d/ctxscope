# Stress inspection

`Stress` repeats an inspection with fresh state from a factory. It is useful
when a shutdown race appears only under a particular scheduling interleaving.
`StressScoped` provides the same repetition for named tasks and queued work.

This example demonstrates:

- five healthy shutdowns and ordered min/p50/p95/max latency statistics;
- a deterministic stand-in for an intermittent failure on iterations 2 and 5;
- scoped jobs that are deliberately dropped before execution on alternating
  iterations.

The failure examples use `Inspect`-style reports rather than failing the test.
Every deliberately stuck goroutine is released before the test returns.

Factories must create new channels, workers, and other mutable state for each
iteration. Reusing state can make one run influence later results.

Run the examples:

```bash
go test -count=1 -v ./examples/stress
go test -race -count=1 ./examples/stress
```
