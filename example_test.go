package ctxscope_test

import (
	"context"
	"fmt"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func ExampleInspect() {
	ready := make(chan struct{})
	stopped := make(chan struct{})

	report, err := ctxscope.Inspect(
		context.Background(),
		func(ctx context.Context) {
			go func() {
				defer close(stopped)
				close(ready)
				<-ctx.Done()
			}()

			<-ready
		},
		ctxscope.WithName("worker"),
		ctxscope.WithGrace(time.Second),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	<-stopped
	fmt.Println(report.Passed())

	// Output:
	// true
}

func ExampleInspect_survivor() {
	ready := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})

	report, err := ctxscope.Inspect(
		context.Background(),
		func(context.Context) {
			go func() {
				defer close(stopped)
				close(ready)
				<-release
			}()

			<-ready
		},
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)

	close(release)
	<-stopped

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(report.Passed())
	fmt.Println(len(report.Survivors) > 0)

	// Output:
	// false
	// true
}

func ExampleInspectScoped_workerPool() {
	jobs := make(chan func())
	poolDone := make(chan struct{})
	ready := make(chan struct{})

	// The pool goroutine exists before the inspection starts.
	go func() {
		defer close(poolDone)
		(<-jobs)()
	}()

	report, err := ctxscope.InspectScoped(
		context.Background(),
		func(scope *ctxscope.Scope) {
			jobs <- scope.Task(
				"refresh cache",
				func(ctx context.Context) {
					close(ready)
					<-ctx.Done()
				},
			)
			<-ready
		},
		ctxscope.WithGrace(time.Second),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	<-poolDone
	message := fmt.Sprintf(
		"passed=%t task=%s state=%s",
		report.Passed(),
		report.Tasks[0].Name,
		report.Tasks[0].State,
	)
	fmt.Println(message)

	// Output:
	// passed=true task=refresh cache state=completed
}

func ExampleScope_GoChild() {
	ready := make(chan struct{}, 2)

	report, err := ctxscope.InspectScoped(
		context.Background(),
		func(scope *ctxscope.Scope) {
			scope.Go("request", func(ctx context.Context) {
				ready <- struct{}{}
				scope.GoChild(ctx, "refresh cache", func(ctx context.Context) {
					ready <- struct{}{}
					<-ctx.Done()
				})
				<-ctx.Done()
			})

			<-ready
			<-ready
		},
		ctxscope.WithGrace(time.Second),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	if len(report.Tasks) != 2 {
		fmt.Printf("unexpected task count: %d\n", len(report.Tasks))
		return
	}

	root := report.Tasks[0]
	child := report.Tasks[1]
	fmt.Printf(
		"passed=%t parent-linked=%t",
		report.Passed(),
		child.ParentID == root.ID,
	)

	// Output:
	// passed=true parent-linked=true
}
