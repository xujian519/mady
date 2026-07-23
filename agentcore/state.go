package agentcore

import (
	"encoding/json"
	"log/slog"
	"sync"
)

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

func NewState() *AgentState {
	return &AgentState{status: StatusIdle}
}

func (s *AgentState) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

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
	case StatusFinished, StatusError:
		return false // terminal states
	case StatusInterrupted:
		return to == StatusRunning // resume allowed
	}
	return false
}

func (s *AgentState) Messages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]Message, len(s.messages))
	for i, m := range s.messages {
		cp[i] = m.Clone()
	}
	return cp
}

// messagesNoClone returns a deep-copied message slice. Every Message
// value is individually cloned so callers cannot race on reference-type
// fields (ToolCalls, Blocks, Metadata, CacheControl) after release of the
// read lock.
func (s *AgentState) messagesNoClone() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]Message, len(s.messages))
	for i, m := range s.messages {
		cp[i] = m.Clone()
	}
	return cp
}

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

func (s *AgentState) Turn() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turn
}

func (s *AgentState) NextTurn() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turn++
	return s.turn
}

func (s *AgentState) SetPendingHandoff(h *PendingHandoff) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingHandoff = h
}

func (s *AgentState) PendingHandoff() *PendingHandoff {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pendingHandoff == nil {
		return nil
	}
	cp := *s.pendingHandoff
	return &cp
}

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

// Snapshot serializes the current state for persistence / resume.
type StateSnapshot struct {
	Status          Status           `json:"status"`
	Messages        []Message        `json:"messages"`
	Turn            int64            `json:"turn"`
	TotalUsage      TokenUsage       `json:"total_usage"`
	InterruptReason *InterruptReason `json:"interrupt_reason,omitempty"`
}

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

func (s *AgentState) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Snapshot())
}

func (s *AgentState) UnmarshalJSON(data []byte) error {
	var snap StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Restore(snap)
	return nil
}
