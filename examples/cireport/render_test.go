package cireport

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func TestRenderMarkdownMatchesGoldenFile(t *testing.T) {
	report := exampleReport()
	want, err := os.ReadFile(filepath.Join("testdata", "failure.golden.md"))
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	wantText := strings.ReplaceAll(string(want), "\r\n", "\n")
	if got := RenderMarkdown(report); got != wantText {
		t.Fatalf("rendered summary differs from golden file\n--- got ---\n%s\n--- want ---\n%s", got, wantText)
	}
}

func TestWriteArtifactsAppendsSummaryAndWritesJSON(t *testing.T) {
	directory := t.TempDir()
	summaryPath := filepath.Join(directory, "summary.md")
	reportPath := filepath.Join(directory, "ctxscope-report.json")

	if err := os.WriteFile(summaryPath, []byte("existing summary\n\n"), 0o600); err != nil {
		t.Fatalf("seed summary: %v", err)
	}
	if err := WriteArtifacts(summaryPath, reportPath, exampleReport()); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}

	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.HasPrefix(string(summary), "existing summary\n\n## ❌") {
		t.Fatalf("summary was not appended:\n%s", summary)
	}

	jsonData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read JSON report: %v", err)
	}
	var decoded ctxscope.Report
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("decode JSON report: %v", err)
	}
	if decoded.SchemaVersion != ctxscope.ReportSchemaVersion {
		t.Errorf("schema version = %d, want %d", decoded.SchemaVersion, ctxscope.ReportSchemaVersion)
	}
	if decoded.Passed() {
		t.Fatal("decoded report unexpectedly passed")
	}
}

func TestRenderMarkdownPassedReport(t *testing.T) {
	summary := RenderMarkdown(ctxscope.Report{
		SchemaVersion: ctxscope.ReportSchemaVersion,
		ScopeID:       "scope-7",
		Name:          "healthy shutdown",
		Grace:         time.Second,
		Elapsed:       5 * time.Millisecond,
	})

	if !strings.Contains(summary, "## ✅ ctxscope cancellation report") {
		t.Fatalf("summary does not show a passed report:\n%s", summary)
	}
	if strings.Contains(summary, "### Violations") {
		t.Fatalf("passed report contains a violations section:\n%s", summary)
	}
}

func TestRenderMarkdownEscapesTableValues(t *testing.T) {
	summary := RenderMarkdown(ctxscope.Report{
		ScopeID: "scope-8",
		Name:    "cache | <shutdown>",
		Grace:   time.Second,
	})

	if !strings.Contains(summary, `cache \| &lt;shutdown&gt;`) {
		t.Fatalf("operation name is not escaped for a Markdown table:\n%s", summary)
	}
}

func TestRenderMarkdownFromRealInspection(t *testing.T) {
	report, err := ctxscope.InspectScoped(
		t.Context(),
		func(scope *ctxscope.Scope) {
			scope.Task("queued email", func(ctx context.Context) {})
		},
		ctxscope.WithName("email delivery"),
		ctxscope.WithGrace(20*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("inspect operation: %v", err)
	}

	summary := RenderMarkdown(report)
	for _, fragment := range []string{
		"ctxscope cancellation report",
		"email delivery",
		"task_never_started",
		"queued email",
		"Task registration stacks",
	} {
		if !strings.Contains(summary, fragment) {
			t.Errorf("summary does not contain %q:\n%s", fragment, summary)
		}
	}
}

func TestRenderMarkdownOrdersTaskHierarchy(t *testing.T) {
	summary := RenderMarkdown(ctxscope.Report{
		ScopeID: "scope-tree",
		Grace:   time.Second,
		Tasks: []ctxscope.TaskReport{
			{ID: "3", ParentID: "2", Name: "grandchild", State: ctxscope.TaskCompleted},
			{ID: "1", Name: "root", State: ctxscope.TaskCompleted},
			{ID: "2", ParentID: "1", Name: "child", State: ctxscope.TaskCompleted},
		},
	})

	root := strings.Index(summary, "| root |")
	child := strings.Index(summary, "↳ child | root |")
	grandchild := strings.Index(summary, "↳ grandchild | child |")
	if root < 0 || child < root || grandchild < child {
		t.Fatalf("tasks are not rendered in hierarchy order:\n%s", summary)
	}
}

func exampleReport() ctxscope.Report {
	survivor := ctxscope.Goroutine{
		Count: 1,
		Frames: []ctxscope.Frame{
			{
				Function: "example.com/service.watchCache",
				File:     "/workspace/cache.go",
				Line:     88,
			},
		},
	}

	return ctxscope.Report{
		SchemaVersion: ctxscope.ReportSchemaVersion,
		ScopeID:       "scope-42",
		Name:          "cache shutdown",
		Grace:         250 * time.Millisecond,
		Elapsed:       250 * time.Millisecond,
		Survivors:     []ctxscope.Goroutine{survivor},
		Tasks: []ctxscope.TaskReport{
			{
				ID:    "task-1",
				Name:  "refresh pipeline",
				State: ctxscope.TaskCompleted,
			},
			{
				ID:       "task-2",
				ParentID: "task-1",
				Name:     "refresh cache",
				State:    ctxscope.TaskCompleted,
				RegistrationStack: []ctxscope.Frame{
					{
						Function: "example.com/service.submitRefresh",
						File:     "/workspace/cache_test.go",
						Line:     41,
					},
				},
				Survivors: []ctxscope.Goroutine{survivor},
			},
		},
		Violations: []ctxscope.Violation{
			{Kind: ctxscope.ViolationShutdownTimeout},
			{
				Kind:     ctxscope.ViolationTaskDescendantSurvived,
				TaskID:   "task-2",
				TaskName: "refresh cache",
			},
		},
	}
}
