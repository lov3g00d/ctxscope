package ctxscope

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportPassed(t *testing.T) {
	tests := []struct {
		name       string
		survivors  []Goroutine
		violations []Violation
		wantPassed bool
	}{
		{
			name:       "no survivors",
			wantPassed: true,
		},
		{
			name: "one survivor",
			survivors: []Goroutine{
				{Count: 1},
			},
			wantPassed: false,
		},
		{
			name: "lifecycle violation without survivor",
			violations: []Violation{
				{Kind: ViolationTaskNeverStarted},
			},
			wantPassed: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := Report{
				Survivors:  test.survivors,
				Violations: test.violations,
			}

			if got := report.Passed(); got != test.wantPassed {
				t.Errorf(
					"Passed() = %t, want %t",
					got,
					test.wantPassed,
				)
			}
		})
	}
}

func TestReportJSONUsesStableFieldNames(t *testing.T) {
	report := Report{
		SchemaVersion: ReportSchemaVersion,
		ScopeID:       "42",
		Tasks: []TaskReport{
			{
				ID:       "2",
				ParentID: "1",
				Name:     "queued task",
				State:    TaskPending,
			},
		},
		Violations: []Violation{
			{
				Kind:     ViolationTaskNeverStarted,
				TaskID:   "1",
				TaskName: "queued task",
			},
		},
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	for _, fragment := range []string{
		`"schema_version":2`,
		`"scope_id":"42"`,
		`"tasks"`,
		`"parent_id":"1"`,
		`"state":"pending"`,
		`"kind":"task_never_started"`,
	} {
		if !strings.Contains(string(encoded), fragment) {
			t.Errorf("JSON does not contain %q: %s", fragment, encoded)
		}
	}
}
