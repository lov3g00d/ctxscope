package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/lov3g00d/ctxscope"
)

func TestWriteReportProducesVersionedJSON(t *testing.T) {
	var output bytes.Buffer
	if err := writeReport(&output); err != nil {
		t.Fatalf("write report: %v", err)
	}

	var report ctxscope.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, output.String())
	}

	if report.SchemaVersion != ctxscope.ReportSchemaVersion {
		t.Errorf(
			"schema version = %d, want %d",
			report.SchemaVersion,
			ctxscope.ReportSchemaVersion,
		)
	}
	if report.Name != "email delivery" {
		t.Errorf("operation name = %q, want %q", report.Name, "email delivery")
	}
	if report.Passed() {
		t.Fatal("expected the JSON report to contain a lifecycle violation")
	}
	if len(report.Tasks) != 1 || report.Tasks[0].Name != "queued email" {
		t.Fatalf("unexpected tasks: %+v", report.Tasks)
	}
	if !hasViolation(report, ctxscope.ViolationShutdownTimeout) ||
		!hasViolation(report, ctxscope.ViolationTaskNeverStarted) {
		t.Fatalf("unexpected violations: %+v", report.Violations)
	}
}

func hasViolation(report ctxscope.Report, kind ctxscope.ViolationKind) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind {
			return true
		}
	}

	return false
}
