//go:build ctxscope_demo_failure

package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func TestFailureDemoPrintsSurvivorStack(t *testing.T) {
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	defer func() {
		close(release)
		<-done
	}()

	ctxscope.Verify(
		t,
		func(context.Context) {
			go func() {
				defer close(done)
				close(ready)
				<-release
			}()

			<-ready
		},
		ctxscope.WithName("stuck message consumer"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
}

func TestScopedFailureDemoPrintsTaskDiagnostics(t *testing.T) {
	jobs := make(chan func())
	ready := make(chan struct{})
	release := make(chan struct{})
	poolDone := make(chan struct{})

	go func() {
		defer close(poolDone)
		(<-jobs)()
	}()

	defer func() {
		close(release)
		<-poolDone
	}()

	ctxscope.VerifyScoped(
		t,
		func(scope *ctxscope.Scope) {
			jobs <- scope.Task(
				"stuck cache refresh",
				func(context.Context) {
					close(ready)
					<-release
				},
			)
			<-ready
		},
		ctxscope.WithName("cache shutdown"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
}
