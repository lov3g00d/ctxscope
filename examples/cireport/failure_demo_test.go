//go:build ctxscope_demo_failure

package cireport

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func TestFailureDemoRendersGitHubSummary(t *testing.T) {
	childReady := make(chan struct{})
	childDone := make(chan struct{})
	childTaskDone := make(chan struct{})
	parentTaskDone := make(chan struct{})
	releaseChild := make(chan struct{})

	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			scope.Go("audit pipeline", func(ctx context.Context) {
				defer close(parentTaskDone)
				scope.GoChild(ctx, "detached audit write", func(context.Context) {
					defer close(childTaskDone)
					go func() {
						defer close(childDone)
						close(childReady)
						<-releaseChild
					}()
				})
				<-childTaskDone
			})

			<-childReady
			<-parentTaskDone
		},
		ctxscope.WithName("audit shutdown"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	close(releaseChild)
	<-childDone

	if err != nil {
		t.Fatalf("inspect audit shutdown: %v", err)
	}

	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	reportPath := os.Getenv("CTXSCOPE_REPORT_PATH")
	if summaryPath == "" || reportPath == "" {
		directory := t.TempDir()
		if summaryPath == "" {
			summaryPath = filepath.Join(directory, "ctxscope-summary.md")
		}
		if reportPath == "" {
			reportPath = filepath.Join(directory, "ctxscope-report.json")
		}
	}

	if err := WriteArtifacts(summaryPath, reportPath, report); err != nil {
		t.Fatalf("write CI report: %v", err)
	}
	t.Logf("Markdown summary: %s", summaryPath)
	t.Logf("JSON report: %s", reportPath)

	if !report.Passed() {
		t.Fatal("cancellation contract failed; see the ctxscope job summary")
	}
}
