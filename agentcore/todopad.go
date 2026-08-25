package agentcore

import "sync"

// TodoStep is a single step in the ordered todo scratchpad.
type TodoStep struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

const (
	TodoStatusPending   = "pending"
	TodoStatusCompleted = "completed"
)

// TodoPad is an ordered, in-memory scratchpad for a single agent run.
// Created fresh per Agent.Run() and Reset() on return. Not persisted.
type TodoPad struct {
	mu    sync.Mutex
	Steps []TodoStep
}

// NewTodoPad 创建一个空的任务待办板。
func NewTodoPad() *TodoPad {
	return &TodoPad{}
}

// Setup replaces steps with the given list, all pending.
func (t *TodoPad) Setup(steps []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Steps = make([]TodoStep, len(steps))
	for i, s := range steps {
		t.Steps[i] = TodoStep{Text: s, Status: TodoStatusPending}
	}
}

// Tick marks the step at index as completed. No-op if out of range.
func (t *TodoPad) Tick(index int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if index < 0 || index >= len(t.Steps) {
		return
	}
	t.Steps[index].Status = TodoStatusCompleted
}

// List returns a copy of the current steps.
func (t *TodoPad) List() []TodoStep {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TodoStep, len(t.Steps))
	copy(out, t.Steps)
	return out
}

// Reset clears all steps.
func (t *TodoPad) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Steps = nil
}

// Completed returns the number of completed steps.
func (t *TodoPad) Completed() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	var n int
	for _, s := range t.Steps {
		if s.Status == TodoStatusCompleted {
			n++
		}
	}
	return n
}

// Snapshot returns (completed, total).
func (t *TodoPad) Snapshot() (completed, total int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.Steps {
		if s.Status == TodoStatusCompleted {
			completed++
		}
		total++
	}
	return
}

// IsActive returns true if the pad has any steps set up.
func (t *TodoPad) IsActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.Steps) > 0
}
