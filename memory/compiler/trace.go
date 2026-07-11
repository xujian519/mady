package compiler

import "time"

// Outcome describes the result of a turn's execution.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomePartial Outcome = "partial"
	OutcomeAborted Outcome = "aborted"
)

// IsPositive returns true for success or partial outcomes.
func (o Outcome) IsPositive() bool {
	return o == OutcomeSuccess || o == OutcomePartial
}

// ExecutionTrace records what happened during a single turn.
type ExecutionTrace struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	StrategyID  string    `json:"strategy_id"`
	Outcome     Outcome   `json:"outcome"`
	ToolCalls   int       `json:"tool_calls"`
	ToolErrors  int       `json:"tool_errors"`
	TurnNumber  int64     `json:"turn_number"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	DurationMs  int64     `json:"duration_ms"`
}

// NewTrace creates a trace for a new turn.
func NewTrace(id, goal, strategyID string, turn int64) ExecutionTrace {
	return ExecutionTrace{
		ID:         id,
		Goal:       goal,
		StrategyID: strategyID,
		TurnNumber: turn,
		StartedAt:  time.Now(),
	}
}

// Complete finalizes the trace with the outcome and tool stats.
func (t *ExecutionTrace) Complete(outcome Outcome, toolCalls, toolErrors int) {
	t.Outcome = outcome
	t.ToolCalls = toolCalls
	t.ToolErrors = toolErrors
	t.CompletedAt = time.Now()
	t.DurationMs = time.Since(t.StartedAt).Milliseconds()
}
