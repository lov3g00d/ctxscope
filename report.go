package ctxscope

import "time"

// ReportSchemaVersion is the version of the JSON-compatible report model.
const ReportSchemaVersion = 2

// Frame identifies one function call in a captured goroutine stack.
type Frame struct {
	// Function is the fully qualified function name.
	Function string `json:"function"`
	// File is the source file path recorded in the profile.
	File string `json:"file"`
	// Line is the source line recorded in the profile.
	Line int `json:"line"`
}

// Goroutine describes one goroutine profile sample. Count can be greater than
// one when the profile combines goroutines with identical stacks and labels.
type Goroutine struct {
	// Count is the number of goroutines represented by this sample.
	Count int64 `json:"count"`
	// Labels contains the pprof labels attached to the sample.
	Labels map[string][]string `json:"labels,omitempty"`
	// Frames contains the captured call stack.
	Frames []Frame `json:"frames,omitempty"`
}

// TaskState describes the latest observed state of a registered task.
type TaskState string

const (
	// TaskPending means the task was registered but has not started.
	TaskPending TaskState = "pending"
	// TaskRunning means the task function is executing.
	TaskRunning TaskState = "running"
	// TaskCompleted means the task function returned.
	TaskCompleted TaskState = "completed"
)

// TaskReport describes a named task registered through Scope.
type TaskReport struct {
	// ID uniquely identifies the task within its inspection.
	ID string `json:"id"`
	// ParentID identifies the task that registered this task. It is empty for a
	// root task.
	ParentID string `json:"parent_id,omitempty"`
	// Name is the human-readable task name.
	Name string `json:"name,omitempty"`
	// State is the latest observed lifecycle state.
	State TaskState `json:"state"`
	// RegisteredAt is when the task was registered.
	RegisteredAt time.Time `json:"registered_at"`
	// RegistrationStack is the stack captured when the task was registered.
	RegistrationStack []Frame `json:"registration_stack,omitempty"`
	// StartedAt is when the task function began executing.
	StartedAt time.Time `json:"started_at,omitzero"`
	// CompletedAt is when the task function returned.
	CompletedAt time.Time `json:"completed_at,omitzero"`
	// Survivors contains labeled goroutines attributed to this task.
	Survivors []Goroutine `json:"survivors,omitempty"`
}

// ViolationKind identifies a cancellation-contract failure.
type ViolationKind string

const (
	// ViolationStartupTimeout means the start function exceeded its deadline.
	ViolationStartupTimeout ViolationKind = "startup_timeout"
	// ViolationShutdownTimeout means operation work survived the grace period.
	ViolationShutdownTimeout ViolationKind = "shutdown_timeout"
	// ViolationTaskNeverStarted means a registered task remained pending.
	ViolationTaskNeverStarted ViolationKind = "task_never_started"
	// ViolationTaskStillRunning means a registered task remained active.
	ViolationTaskStillRunning ViolationKind = "task_still_running"
	// ViolationTaskDescendantSurvived means a completed task left a descendant.
	ViolationTaskDescendantSurvived ViolationKind = "task_descendant_survived"
)

// Violation describes one failed lifecycle contract.
type Violation struct {
	Kind     ViolationKind `json:"kind"`
	TaskID   string        `json:"task_id,omitempty"`
	TaskName string        `json:"task_name,omitempty"`
}

// Report is the result of an inspection.
type Report struct {
	// SchemaVersion identifies the machine-readable report schema.
	SchemaVersion int `json:"schema_version"`
	// ScopeID uniquely identifies this inspection within the current process.
	ScopeID string `json:"scope_id"`
	// Name is the optional human-readable name supplied with WithName.
	Name string `json:"name,omitempty"`
	// Grace is the configured cancellation grace period.
	Grace time.Duration `json:"grace_ns"`
	// StartupElapsed is the time from inspection start until cancellation.
	StartupElapsed time.Duration `json:"startup_elapsed_ns"`
	// Elapsed is the time spent waiting after cancellation.
	Elapsed time.Duration `json:"elapsed_ns"`
	// CanceledAt is when Inspect canceled the operation context.
	CanceledAt time.Time `json:"canceled_at"`
	// Survivors contains goroutine samples still present after the grace period.
	Survivors []Goroutine `json:"survivors,omitempty"`
	// Tasks contains lifecycle information for tasks registered through Scope.
	Tasks []TaskReport `json:"tasks,omitempty"`
	// Violations describes lifecycle contracts that were not satisfied.
	Violations []Violation `json:"violations,omitempty"`
}

// Passed reports whether the inspection found no survivors or lifecycle
// violations.
func (report Report) Passed() bool {
	return len(report.Survivors) == 0 && len(report.Violations) == 0
}
