package agentcore

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// Status represents the lifecycle state of an agent.
type Status string

// Status values for the Agent lifecycle state machine.
const (
	StatusIdle        Status = "idle"
	StatusRunning     Status = "running"
	StatusFinished    Status = "finished"
	StatusError       Status = "error"
	StatusInterrupted Status = "interrupted"
)

// AgentState holds the mutable conversation state across turns.
type AgentState struct {
	mu             sync.RWMutex
	status         Status
	messages       []Message
	turn           int64
	pendingHandoff *PendingHandoff
	totalUsage     TokenUsage
	interrupt      *InterruptReason
}

// NewState creates a new AgentState with the initial status set to idle.
func NewState() *AgentState {
	return &AgentState{status: StatusIdle}
}

// Status returns the current lifecycle status of the agent.
func (s *AgentState) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatus transitions the agent to the given status, logging illegal transitions.
func (s *AgentState) SetStatus(st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !isValidTransition(s.status, st) {
		slog.Warn("state: illegal status transition", "from", s.status, "to", st)
	}
	s.status = st
}

func isValidTransition(from, to Status) bool {
	switch from {
	case StatusIdle:
		return to == StatusRunning
	case StatusRunning:
		return to == StatusFinished || to == StatusError || to == StatusInterrupted
	case StatusFinished:
		// Agent 设计为可跨多次 Run() 调用复用，允许结束状态重新进入运行。
		return to == StatusRunning
	case StatusError:
		// 与 StatusFinished 一样允许重新进入运行——可恢复的错误（如瞬态 API 故障）
		// 可以通过重试解决，不必重建 Agent 实例。
		return to == StatusRunning
	case StatusInterrupted:
		return to == StatusRunning // resume allowed
	}
	return false
}

// Messages returns a deep-copied slice of all conversation messages.
func (s *AgentState) Messages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]Message, len(s.messages))
	for i, m := range s.messages {
		cp[i] = m.Clone()
	}
	return cp
}

// messagesReadOnly returns a deep-copied message slice. Every Message
// value is individually cloned so callers cannot race on reference-type
// fields (ToolCalls, Blocks, Metadata, CacheControl) after release of the
// read lock.
func (s *AgentState) messagesReadOnly() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]Message, len(s.messages))
	for i, m := range s.messages {
		cp[i] = m.Clone()
	}
	return cp
}

// AddMessage appends a message to the conversation, or replaces an existing
// message with a matching non-empty ID.
func (s *AgentState) AddMessage(m Message) {
	m = m.Clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.ID != "" {
		for i := range s.messages {
			if s.messages[i].ID == m.ID {
				s.messages[i] = m
				return
			}
		}
	}
	s.messages = append(s.messages, m)
}

// HasSystemPrompt returns true if the conversation history already contains a
// system prompt message. Used by Agent.Run to avoid appending duplicate system
// prompts when reusing an agent across multiple calls.
func (s *AgentState) HasSystemPrompt() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.messages {
		if m.Role == RoleSystem {
			return true
		}
	}
	return false
}

// ReplaceMessages atomically replaces the entire message history.
// Used by compaction to swap old messages with a summary.
func (s *AgentState) ReplaceMessages(msgs []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = make([]Message, len(msgs))
	for i, m := range msgs {
		s.messages[i] = m.Clone()
	}
}

// Turn returns the current turn number.
func (s *AgentState) Turn() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turn
}

// NextTurn increments the turn counter and returns the new value.
func (s *AgentState) NextTurn() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turn++
	return s.turn
}

// SetPendingHandoff stores a pending handoff for transfer-mode delegation.
func (s *AgentState) SetPendingHandoff(h *PendingHandoff) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingHandoff = h
}

// PendingHandoff returns a copy of the pending handoff, or nil if none.
func (s *AgentState) PendingHandoff() *PendingHandoff {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pendingHandoff == nil {
		return nil
	}
	cp := *s.pendingHandoff
	return &cp
}

// ClearPendingHandoff removes any pending handoff from the state.
func (s *AgentState) ClearPendingHandoff() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingHandoff = nil
}

// AddUsage accumulates token usage across turns.
func (s *AgentState) AddUsage(usage TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalUsage.PromptTokens += usage.PromptTokens
	s.totalUsage.CompletionTokens += usage.CompletionTokens
	s.totalUsage.TotalTokens += usage.TotalTokens
}

// TotalUsage returns the accumulated token usage across all turns.
func (s *AgentState) TotalUsage() TokenUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalUsage
}

// StateSnapshot is a serializable snapshot of the agent's state for persistence and resume.
type StateSnapshot struct {
	Status          Status           `json:"status"`
	Messages        []Message        `json:"messages"`
	Turn            int64            `json:"turn"`
	TotalUsage      TokenUsage       `json:"total_usage"`
	InterruptReason *InterruptReason `json:"interrupt_reason,omitempty"`
}

// Snapshot captures the current agent state as a serializable StateSnapshot.
func (s *AgentState) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := make([]Message, len(s.messages))
	for i, m := range s.messages {
		msgs[i] = m.Clone()
	}
	ir := s.interrupt
	if ir != nil {
		c := *ir
		ir = &c
	}
	return StateSnapshot{
		Status:          s.status,
		Messages:        msgs,
		Turn:            s.turn,
		TotalUsage:      s.totalUsage,
		InterruptReason: ir,
	}
}

// Restore replaces the current agent state with the given snapshot.
func (s *AgentState) Restore(snap StateSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = snap.Status
	// Deep-copy messages to prevent aliasing: if we assign the slice
	// header directly, a subsequent AddMessage that triggers append with
	// spare capacity would write through to the snapshot's backing array,
	// corrupting checkpoint history.
	msgs := make([]Message, len(snap.Messages))
	for i, m := range snap.Messages {
		msgs[i] = m.Clone()
	}
	s.messages = msgs
	s.turn = snap.Turn
	s.totalUsage = snap.TotalUsage
	if snap.InterruptReason != nil {
		c := *snap.InterruptReason
		s.interrupt = &c
	} else {
		s.interrupt = nil
	}
}

// SetInterruptReason records why the agent was interrupted.
func (s *AgentState) SetInterruptReason(r *InterruptReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupt = r
}

// GetInterruptReason returns the interrupt reason, if any.
func (s *AgentState) GetInterruptReason() *InterruptReason {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.interrupt == nil {
		return nil
	}
	c := *s.interrupt
	return &c
}

// ClearInterruptReason removes the interrupt reason.
func (s *AgentState) ClearInterruptReason() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupt = nil
}

// MarshalJSON serializes the agent state as a JSON snapshot.
func (s *AgentState) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Snapshot())
}

// UnmarshalJSON deserializes the agent state from a JSON snapshot.
func (s *AgentState) UnmarshalJSON(data []byte) error {
	var snap StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Restore(snap)
	return nil
}
