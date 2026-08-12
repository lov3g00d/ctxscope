package ctxscope

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type taskRegistry struct {
	mu       sync.Mutex
	sequence uint64
	tasks    []*taskRecord
}

type taskRecord struct {
	id                string
	name              string
	state             TaskState
	registeredAt      time.Time
	registrationStack []Frame
	startedAt         time.Time
	completedAt       time.Time
}

func (registry *taskRegistry) register(
	name string,
	registrationStack []Frame,
) *taskRecord {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.sequence++
	record := &taskRecord{
		id:                fmt.Sprintf("%d", registry.sequence),
		name:              name,
		state:             TaskPending,
		registeredAt:      time.Now(),
		registrationStack: registrationStack,
	}

	registry.tasks = append(registry.tasks, record)
	return record
}

func (registry *taskRegistry) start(record *taskRecord) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	record.state = TaskRunning
	record.startedAt = time.Now()
}

func (registry *taskRegistry) complete(record *taskRecord) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	record.state = TaskCompleted
	record.completedAt = time.Now()
}

func (registry *taskRegistry) active() int {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	active := 0
	for _, task := range registry.tasks {
		if task.state != TaskCompleted {
			active++
		}
	}

	return active
}

func (registry *taskRegistry) snapshot() []TaskReport {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	tasks := make([]TaskReport, len(registry.tasks))
	for index, task := range registry.tasks {
		tasks[index] = TaskReport{
			ID:                task.id,
			Name:              task.name,
			State:             task.state,
			RegisteredAt:      task.registeredAt,
			RegistrationStack: append([]Frame(nil), task.registrationStack...),
			StartedAt:         task.startedAt,
			CompletedAt:       task.completedAt,
		}
	}

	return tasks
}

func captureRegistrationFrames() []Frame {
	pcs := make([]uintptr, 32)
	count := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:count])

	var captured []Frame
	for {
		frame, more := frames.Next()
		captured = append(captured, Frame{
			Function: frame.Function,
			File:     frame.File,
			Line:     frame.Line,
		})

		if !more {
			break
		}
	}

	return captured
}
