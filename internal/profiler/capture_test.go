package profiler

import (
	"context"
	"runtime/pprof"
	"slices"
	"strings"
	"testing"
)

func TestCaptureScope(t *testing.T) {
	const scopeID = "capture-scope-test"

	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	defer func() {
		close(release)
		<-done
	}()

	pprof.Do(
		context.Background(),
		pprof.Labels(ScopeLabel, scopeID),
		func(context.Context) {
			go func() {
				defer close(done)
				close(ready)

				<-release
			}()
		},
	)
	<-ready

	got, err := CaptureScope(scopeID)
	if err != nil {
		t.Fatalf("capture scope: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d profile samples, want 1", len(got))
	}

	survivor := got[0]

	if survivor.Count != 1 {
		t.Errorf("got sample count: %d, want 1", survivor.Count)
	}

	if !slices.Contains(survivor.Labels[ScopeLabel], scopeID) {
		t.Errorf(
			"scope labels %v do not contain %q",
			survivor.Labels[ScopeLabel],
			scopeID,
		)
	}

	if !containsFunction(survivor.Frames, "TestCaptureScope") {
		t.Errorf(
			"captured frames do not contain TestCaptureScope: %+v",
			survivor.Frames,
		)
	}
}

func TestCaptureScopeReturnsEmptyForUnknownScope(t *testing.T) {
	got, err := CaptureScope("scope-that-does-not-exist")
	if err != nil {
		t.Fatalf("capture scope: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("got %d profile samples, want 0", len(got))
	}
}

func containsFunction(frames []Frame, fragment string) bool {
	for _, frame := range frames {
		if strings.Contains(frame.Function, fragment) {
			return true
		}
	}

	return false
}
