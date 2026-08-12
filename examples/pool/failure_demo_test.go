//go:build ctxscope_demo_failure

package pool_test

import (
	"context"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
	"github.com/lov3g00d/ctxscope/examples/pool"
)

func TestFailureDemoReportsCompletedTaskDescendant(t *testing.T) {
	workers := pool.New(1, 1)
	childReady := make(chan struct{})
	childDone := make(chan struct{})
	taskDone := make(chan struct{})
	releaseChild := make(chan struct{})

	defer func() {
		close(releaseChild)
		<-childDone
		workers.Close()
	}()

	ctxscope.VerifyScoped(
		t,
		func(scope *ctxscope.Scope) {
			if err := workers.Submit(
				scope.Context(),
				scope.Task("detached audit write", func(context.Context) {
					defer close(taskDone)
					go func() {
						defer close(childDone)
						close(childReady)
						<-releaseChild
					}()
				}),
			); err != nil {
				panic(err)
			}

			<-childReady
			<-taskDone
		},
		ctxscope.WithName("audit shutdown"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
}
