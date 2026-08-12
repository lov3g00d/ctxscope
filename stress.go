package ctxscope

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// StartFactory creates fresh operation startup logic for one stress iteration.
type StartFactory func(iteration int) StartFunc

// ScopedStartFactory creates fresh scoped startup logic for one stress
// iteration.
type ScopedStartFactory func(iteration int) ScopedStartFunc

// LatencyStats summarizes observed post-cancellation inspection durations.
type LatencyStats struct {
	Min time.Duration `json:"min_ns"`
	P50 time.Duration `json:"p50_ns"`
	P95 time.Duration `json:"p95_ns"`
	Max time.Duration `json:"max_ns"`
}

// StressReport contains the result of repeated sequential inspections.
type StressReport struct {
	SchemaVersion int          `json:"schema_version"`
	Runs          int          `json:"runs"`
	PassedRuns    int          `json:"passed_runs"`
	FailedRuns    int          `json:"failed_runs"`
	Latency       LatencyStats `json:"latency"`
	Reports       []Report     `json:"reports"`
}

// Passed reports whether every stress iteration passed.
func (report StressReport) Passed() bool {
	return report.Runs > 0 && report.FailedRuns == 0
}

// Stress runs sequential inspections using fresh startup logic from factory.
// It is useful for cancellation behavior that fails intermittently.
func Stress(
	parent context.Context,
	runs int,
	factory StartFactory,
	options ...Option,
) (StressReport, error) {
	if factory == nil {
		return StressReport{}, errors.New("ctxscope: nil start factory")
	}

	return runStress(runs, func(iteration int) (Report, error) {
		start := factory(iteration)
		if start == nil {
			return Report{}, fmt.Errorf(
				"ctxscope: start factory returned nil for iteration %d",
				iteration,
			)
		}

		return Inspect(parent, start, options...)
	})
}

// StressScoped runs sequential scoped inspections using fresh startup logic
// from factory.
func StressScoped(
	parent context.Context,
	runs int,
	factory ScopedStartFactory,
	options ...Option,
) (StressReport, error) {
	if factory == nil {
		return StressReport{}, errors.New("ctxscope: nil scoped start factory")
	}

	return runStress(runs, func(iteration int) (Report, error) {
		start := factory(iteration)
		if start == nil {
			return Report{}, fmt.Errorf(
				"ctxscope: scoped start factory returned nil for iteration %d",
				iteration,
			)
		}

		return InspectScoped(parent, start, options...)
	})
}

type stressInspection func(iteration int) (Report, error)

func runStress(
	runs int,
	inspect stressInspection,
) (StressReport, error) {
	if runs <= 0 {
		return StressReport{}, fmt.Errorf(
			"ctxscope: stress runs must be greater than zero: %d",
			runs,
		)
	}

	report := StressReport{
		SchemaVersion: ReportSchemaVersion,
		Runs:          runs,
		Reports:       make([]Report, 0, runs),
	}
	latencies := make([]time.Duration, 0, runs)

	for iteration := range runs {
		inspection, err := inspect(iteration)
		if err != nil {
			report.Runs = iteration
			report.Latency = summarizeLatencies(latencies)
			return report, fmt.Errorf(
				"ctxscope: stress iteration %d: %w",
				iteration,
				err,
			)
		}

		report.Reports = append(report.Reports, inspection)
		latencies = append(latencies, inspection.Elapsed)
		if inspection.Passed() {
			report.PassedRuns++
		} else {
			report.FailedRuns++
		}
	}

	report.Latency = summarizeLatencies(latencies)
	return report, nil
}

func summarizeLatencies(latencies []time.Duration) LatencyStats {
	if len(latencies) == 0 {
		return LatencyStats{}
	}

	sorted := append([]time.Duration(nil), latencies...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left] < sorted[right]
	})

	return LatencyStats{
		Min: sorted[0],
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		Max: sorted[len(sorted)-1],
	}
}

func percentile(sorted []time.Duration, percent int) time.Duration {
	index := (percent*len(sorted)+99)/100 - 1
	return sorted[index]
}
