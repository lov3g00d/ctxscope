// Package ctxscope checks whether goroutines started for an operation stop
// after that operation's context is canceled.
//
// Use [Verify] in tests that only need a pass or fail result. Use [Inspect]
// when callers need to examine surviving goroutines and their stack frames.
// Use [VerifyScoped] or [InspectScoped] to track named work submitted through
// pre-existing worker pools and queues. [Scope.GoChild] and [Scope.TaskChild]
// preserve parent-child relationships between registered tasks.
// Use [Stress] or [StressScoped] to repeat an inspection with fresh operation
// state and summarize intermittent failures and shutdown latency.
package ctxscope
