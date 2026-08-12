package ctxscope

import (
	"bytes"
	"context"
	"runtime/pprof"
	"strings"
	"testing"
)

func TestChildGoroutineInheritsScopeLabels(t *testing.T) {
	const (
		labelKey = "ctxscope.scope"
		scopeID  = "ctxscope-inheritance-proof"
	)

	ready := make(chan struct{})
	release := make(chan struct{})

	defer close(release)

	pprof.Do(
		context.Background(),
		pprof.Labels(labelKey, scopeID),
		func(context.Context) {
			go func() {
				close(ready)
				<-release
			}()
		},
	)

	<-ready

	var profile bytes.Buffer

	if err := pprof.Lookup("goroutine").WriteTo(&profile, 1); err != nil {
		t.Fatalf("capture goroutine profile: %v", err)
	}

	if !strings.Contains(profile.String(), labelKey) {
		t.Fatalf("profile does not contain label key %q", labelKey)
	}

	if !strings.Contains(profile.String(), scopeID) {
		t.Fatalf("profile does not contain scope id %q", scopeID)
	}
}
