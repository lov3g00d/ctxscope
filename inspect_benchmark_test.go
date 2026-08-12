package ctxscope

import (
	"context"
	"testing"
	"time"
)

func BenchmarkInspectCooperativeWorker(b *testing.B) {
	for b.Loop() {
		ready := make(chan struct{})

		report, err := Inspect(
			context.Background(),
			func(ctx context.Context) {
				go func() {
					close(ready)
					<-ctx.Done()
				}()

				<-ready
			},
			WithGrace(time.Second),
			WithPollInterval(time.Millisecond),
		)
		if err != nil {
			b.Fatalf("Inspect returned an error: %v", err)
		}

		if !report.Passed() {
			b.Fatalf("Inspect reported %d survivor samples", len(report.Survivors))
		}
	}
}

func BenchmarkInspectScopedCooperativeTask(b *testing.B) {
	for b.Loop() {
		ready := make(chan struct{})

		report, err := InspectScoped(
			context.Background(),
			func(scope *Scope) {
				scope.Go("worker", func(ctx context.Context) {
					close(ready)
					<-ctx.Done()
				})

				<-ready
			},
			WithGrace(time.Second),
			WithPollInterval(time.Millisecond),
		)
		if err != nil {
			b.Fatalf("InspectScoped returned an error: %v", err)
		}

		if !report.Passed() {
			b.Fatalf(
				"InspectScoped reported %d survivor samples",
				len(report.Survivors),
			)
		}
	}
}

func BenchmarkInspectSurvivingWorker(b *testing.B) {
	benchmarks := []struct {
		name            string
		maximumInterval time.Duration
	}{
		{
			name:            "adaptive polling",
			maximumInterval: 8 * time.Millisecond,
		},
		{
			name:            "fixed 1ms polling",
			maximumInterval: time.Millisecond,
		},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			for b.Loop() {
				ready := make(chan struct{})
				release := make(chan struct{})
				done := make(chan struct{})

				report, err := Inspect(
					context.Background(),
					func(context.Context) {
						go func() {
							defer close(done)
							close(ready)
							<-release
						}()
						<-ready
					},
					WithGrace(20*time.Millisecond),
					WithPollInterval(time.Millisecond),
					WithMaxPollInterval(benchmark.maximumInterval),
				)

				close(release)
				<-done

				if err != nil {
					b.Fatalf("Inspect returned an error: %v", err)
				}

				if report.Passed() {
					b.Fatal("Inspect did not report the surviving worker")
				}
			}
		})
	}
}
