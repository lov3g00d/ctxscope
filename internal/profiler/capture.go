package profiler

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/pprof"
	"slices"

	profilepkg "github.com/google/pprof/profile"
)

// ScopeLabel is the pprof label key used to identify an inspection.
const ScopeLabel = "ctxscope.scope"

// TaskLabel and TaskNameLabel identify a registered task within a scope.
const (
	TaskLabel     = "ctxscope.task"
	TaskNameLabel = "ctxscope.task_name"
)

// ErrGoroutineProfileUnavailable indicates that the runtime did not expose a
// goroutine profile.
var ErrGoroutineProfileUnavailable = errors.New(
	"goroutine profile unavailable",
)

// Frame identifies one function call in a profile sample.
type Frame struct {
	Function string
	File     string
	Line     int
}

// Goroutine describes one labeled goroutine profile sample.
type Goroutine struct {
	Count  int64
	Labels map[string][]string
	Frames []Frame
}

// CaptureScope returns goroutine profile samples with the requested scope ID.
func CaptureScope(scopeID string) ([]Goroutine, error) {
	runtimeProfile := pprof.Lookup("goroutine")
	if runtimeProfile == nil {
		return nil, ErrGoroutineProfileUnavailable
	}

	var buffer bytes.Buffer

	if err := runtimeProfile.WriteTo(&buffer, 0); err != nil {
		return nil, fmt.Errorf("write goroutine profile: %w", err)
	}

	parsedProfile, err := profilepkg.ParseData(buffer.Bytes())
	if err != nil {
		return nil, fmt.Errorf("parse goroutine profile: %w", err)
	}

	var goroutines []Goroutine

	for _, sample := range parsedProfile.Sample {
		if !slices.Contains(sample.Label[ScopeLabel], scopeID) {
			continue
		}

		goroutines = append(goroutines, Goroutine{
			Count:  sampleCount(sample),
			Labels: cloneLabels(sample.Label),
			Frames: sampleFrames(sample),
		})
	}

	return goroutines, nil
}

func sampleCount(sample *profilepkg.Sample) int64 {
	if len(sample.Value) == 0 {
		return 1
	}

	return sample.Value[0]
}

func sampleFrames(sample *profilepkg.Sample) []Frame {
	var frames []Frame

	for _, location := range sample.Location {
		for _, line := range location.Line {
			if line.Function == nil {
				continue
			}

			frames = append(frames, Frame{
				Function: line.Function.Name,
				File:     line.Function.Filename,
				Line:     int(line.Line),
			})
		}
	}
	return frames
}

func cloneLabels(source map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(source))

	for key, values := range source {
		cloned[key] = slices.Clone(values)
	}

	return cloned
}
