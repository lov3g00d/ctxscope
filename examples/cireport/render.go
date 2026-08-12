// Package cireport demonstrates how to render ctxscope reports for GitHub
// Actions without adding CI-specific behavior to the ctxscope library.
package cireport

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"strings"

	"github.com/lov3g00d/ctxscope"
)

// RenderMarkdown converts a report into a GitHub job summary.
func RenderMarkdown(report ctxscope.Report) string {
	var output strings.Builder
	statusIcon := "✅"
	statusText := "Passed"
	if !report.Passed() {
		statusIcon = "❌"
		statusText = "Failed"
	}

	fmt.Fprintf(&output, "## %s ctxscope cancellation report\n\n", statusIcon)
	output.WriteString("| Field | Value |\n")
	output.WriteString("| --- | --- |\n")
	fmt.Fprintf(&output, "| Operation | %s |\n", tableValue(report.Name))
	fmt.Fprintf(&output, "| Scope | `%s` |\n", tableValue(report.ScopeID))
	fmt.Fprintf(&output, "| Status | **%s** |\n", statusText)
	fmt.Fprintf(&output, "| Grace period | `%s` |\n", report.Grace)
	fmt.Fprintf(&output, "| Shutdown observation | `%s` |\n", report.Elapsed)
	fmt.Fprintf(&output, "| Registered tasks | %d |\n", len(report.Tasks))
	fmt.Fprintf(&output, "| Surviving goroutines | %d |\n", survivorCount(report.Survivors))

	if len(report.Violations) > 0 {
		output.WriteString("\n### Violations\n\n")
		output.WriteString("| Kind | Task |\n")
		output.WriteString("| --- | --- |\n")
		for _, violation := range report.Violations {
			fmt.Fprintf(
				&output,
				"| `%s` | %s |\n",
				tableValue(string(violation.Kind)),
				tableValue(violation.TaskName),
			)
		}
	}

	if len(report.Tasks) > 0 {
		output.WriteString("\n### Tasks\n\n")
		output.WriteString("| Task hierarchy | Parent | State | Attributed survivors |\n")
		output.WriteString("| --- | --- | --- | ---: |\n")
		tasksByID := indexTasks(report.Tasks)
		for _, node := range orderedTasks(report.Tasks) {
			task := node.task
			fmt.Fprintf(
				&output,
				"| %s | %s | `%s` | %d |\n",
				taskTreeValue(task.Name, node.depth),
				parentTableValue(task.ParentID, tasksByID),
				tableValue(string(task.State)),
				survivorCount(task.Survivors),
			)
		}
	}

	writeTaskDiagnostics(&output, report.Tasks)
	writeSurvivors(&output, report.Survivors)

	return strings.TrimRight(output.String(), "\n") + "\n"
}

// WriteJSON writes the complete machine-readable report with indentation.
func WriteJSON(output io.Writer, report ctxscope.Report) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteArtifacts appends Markdown to summaryPath and replaces reportPath with
// the complete JSON report. GitHub exposes summaryPath as GITHUB_STEP_SUMMARY.
func WriteArtifacts(
	summaryPath string,
	reportPath string,
	report ctxscope.Report,
) error {
	summary, err := os.OpenFile(
		summaryPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open job summary: %w", err)
	}

	if _, err := io.WriteString(summary, RenderMarkdown(report)); err != nil {
		summary.Close()
		return fmt.Errorf("write job summary: %w", err)
	}
	if err := summary.Close(); err != nil {
		return fmt.Errorf("close job summary: %w", err)
	}

	jsonFile, err := os.OpenFile(
		reportPath,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open JSON report: %w", err)
	}

	if err := WriteJSON(jsonFile, report); err != nil {
		jsonFile.Close()
		return fmt.Errorf("write JSON report: %w", err)
	}
	if err := jsonFile.Close(); err != nil {
		return fmt.Errorf("close JSON report: %w", err)
	}

	return nil
}

func writeTaskDiagnostics(output *strings.Builder, tasks []ctxscope.TaskReport) {
	tasksByID := indexTasks(tasks)
	wroteHeading := false
	wroteTask := false
	for _, task := range tasks {
		if len(task.RegistrationStack) == 0 ||
			(task.State == ctxscope.TaskCompleted && len(task.Survivors) == 0) {
			continue
		}

		if !wroteHeading {
			output.WriteString("\n### Task registration stacks\n\n")
			wroteHeading = true
		}
		if wroteTask {
			output.WriteString("\n")
		}

		output.WriteString("<details>\n<summary>")
		fmt.Fprintf(
			output,
			"%s — <code>%s</code>",
			detailValue(task.Name),
			html.EscapeString(string(task.State)),
		)
		if task.ParentID != "" {
			parentName := task.ParentID
			if parent, exists := tasksByID[task.ParentID]; exists && parent.Name != "" {
				parentName = parent.Name
			}
			fmt.Fprintf(output, " — child of %s", detailValue(parentName))
		}
		output.WriteString("</summary>\n\n")
		writeFrames(output, task.RegistrationStack)
		output.WriteString("\n</details>\n")
		wroteTask = true
	}
}

func writeSurvivors(output *strings.Builder, survivors []ctxscope.Goroutine) {
	if len(survivors) == 0 {
		return
	}

	output.WriteString("\n### Survivor stacks\n\n")
	for index, survivor := range survivors {
		if index > 0 {
			output.WriteString("\n")
		}
		fmt.Fprintf(
			output,
			"<details>\n<summary>Sample %d — %d goroutine(s)</summary>\n\n",
			index+1,
			survivor.Count,
		)
		writeFrames(output, survivor.Frames)
		output.WriteString("\n</details>\n")
	}
}

func writeFrames(output *strings.Builder, frames []ctxscope.Frame) {
	output.WriteString("```text\n")
	if len(frames) == 0 {
		output.WriteString("stack unavailable\n")
	}
	for _, frame := range frames {
		fmt.Fprintf(output, "%s\n  %s:%d\n", frame.Function, frame.File, frame.Line)
	}
	output.WriteString("```")
}

func survivorCount(survivors []ctxscope.Goroutine) int64 {
	var count int64
	for _, survivor := range survivors {
		count += survivor.Count
	}
	return count
}

func tableValue(value string) string {
	if value == "" {
		return "—"
	}

	value = html.EscapeString(value)
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", "<br>")
}

func detailValue(value string) string {
	if value == "" {
		return "unnamed task"
	}
	return html.EscapeString(value)
}

type renderedTask struct {
	task  ctxscope.TaskReport
	depth int
}

func orderedTasks(tasks []ctxscope.TaskReport) []renderedTask {
	byID := indexTasks(tasks)
	children := make(map[string][]int, len(tasks))
	var roots []int
	for index, task := range tasks {
		_, parentExists := byID[task.ParentID]
		if task.ParentID == "" || !parentExists || task.ParentID == task.ID {
			roots = append(roots, index)
			continue
		}
		children[task.ParentID] = append(children[task.ParentID], index)
	}

	ordered := make([]renderedTask, 0, len(tasks))
	visited := make(map[int]bool, len(tasks))
	var visit func(int, int)
	visit = func(index, depth int) {
		if visited[index] {
			return
		}
		visited[index] = true
		task := tasks[index]
		ordered = append(ordered, renderedTask{task: task, depth: depth})
		for _, childIndex := range children[task.ID] {
			visit(childIndex, depth+1)
		}
	}

	for _, root := range roots {
		visit(root, 0)
	}
	for index := range tasks {
		visit(index, 0)
	}

	return ordered
}

func indexTasks(tasks []ctxscope.TaskReport) map[string]ctxscope.TaskReport {
	byID := make(map[string]ctxscope.TaskReport, len(tasks))
	for _, task := range tasks {
		if task.ID != "" {
			byID[task.ID] = task
		}
	}
	return byID
}

func taskTreeValue(name string, depth int) string {
	if name == "" {
		name = "unnamed task"
	}
	prefix := strings.Repeat("&nbsp;&nbsp;", depth)
	if depth > 0 {
		prefix += "↳ "
	}
	return prefix + tableValue(name)
}

func parentTableValue(
	parentID string,
	tasksByID map[string]ctxscope.TaskReport,
) string {
	if parentID == "" {
		return "—"
	}
	if parent, exists := tasksByID[parentID]; exists && parent.Name != "" {
		return tableValue(parent.Name)
	}
	return "`" + tableValue(parentID) + "`"
}
