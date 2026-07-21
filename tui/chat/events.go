package chat

import (
	"time"

	"github.com/xujian519/mady/tui/component"
)

type EventSubscriber interface {
	On(eventType ChatEventType, handler func(ChatEvent))
}

type Subscriber interface {
	Subscribe(sub EventSubscriber)
}

type ChatEventType string

const (
	ChatEventAgentStart      ChatEventType = "agent_start"
	ChatEventAgentEnd        ChatEventType = "agent_end"
	ChatEventAgentError      ChatEventType = "agent_error"
	ChatEventTurnStart       ChatEventType = "turn_start"
	ChatEventTurnEnd         ChatEventType = "turn_end"
	ChatEventMessageDelta    ChatEventType = "message_delta"
	ChatEventToolCallStart   ChatEventType = "tool_call_start"
	ChatEventToolCallEnd     ChatEventType = "tool_call_end"
	ChatEventHandoffStart    ChatEventType = "handoff_start"
	ChatEventHandoffEnd      ChatEventType = "handoff_end"
	ChatEventCompactionStart ChatEventType = "compaction_start"
	ChatEventCompactionEnd   ChatEventType = "compaction_end"
	ChatEventAutoRetry       ChatEventType = "auto_retry"
	ChatEventAgentInterrupt  ChatEventType = "agent_interrupt"
	ChatEventApprovalPrompt  ChatEventType = "approval_prompt"
)

type ChatEvent interface {
	ChatEventKind() ChatEventType
}

type AgentStartChatEvent struct {
	AgentName string
	Input     string
}

func (AgentStartChatEvent) ChatEventKind() ChatEventType { return ChatEventAgentStart }

type AgentEndChatEvent struct {
	AgentName string
	Output    string
}

func (AgentEndChatEvent) ChatEventKind() ChatEventType { return ChatEventAgentEnd }

// AgentInterruptChatEvent carries the reason an agent paused for human
// review (e.g. disclosure review_gate, or an ApprovalGate keyword trigger).
// Reason.Data may hold gate-specific context (gate name, report_id) that the
// TUI uses to render a tailored guidance prompt.
type AgentInterruptChatEvent struct {
	Reason string
	Data   map[string]any
}

func (AgentInterruptChatEvent) ChatEventKind() ChatEventType { return ChatEventAgentInterrupt }

// ReviewGatePayload carries the structured data for the review gate overlay.
// This is a data-only type (no callbacks) used for cross-layer transfer.
type ReviewGatePayload struct {
	Title      string
	Judgment   string
	Confidence float64
	Evidences  []component.ReviewEvidence
	Checklist  []component.ReviewCheckItem
	Risks      []string
}

// JudgmentSummary carries structured judgment data for the TUI's
// judgment-bar summary at the top of the chat view. It represents the
// agent's current "判断 + 置信度 + 仍待确认" in a compact form.
//
//   - Phase: task phase label, e.g. "分析阶段", "草案阶段", "复核阶段"
//   - Judgment: one-line conclusion text
//   - Confidence: 0.0-1.0, maps to 0-100 bar; <0 means hide the bar
//   - Pending: still-to-confirm items (only the first 3 are shown)
type JudgmentSummary struct {
	Phase      string
	Judgment   string
	Confidence float64
	Pending    []string
}

// ApprovalPromptChatEvent 是 ApprovalGate 触发人工审核时发射的事件。
// TUI 的 onApprovalPrompt 将其渲染为含 DomainMsg (approval_prompt) 的 ChatMessage。
// Data 字段携带可选的复核门结构化数据（ReviewGatePayload）。
type ApprovalPromptChatEvent struct {
	Content string
	Data    *ReviewGatePayload
}

func (ApprovalPromptChatEvent) ChatEventKind() ChatEventType { return ChatEventApprovalPrompt }

type AgentErrorChatEvent struct {
	Err error
}

func (AgentErrorChatEvent) ChatEventKind() ChatEventType { return ChatEventAgentError }

type TurnStartChatEvent struct {
	Turn int64
}

func (TurnStartChatEvent) ChatEventKind() ChatEventType { return ChatEventTurnStart }

type TurnEndChatEvent struct {
	Turn  int64
	Usage TokenUsage
}

func (TurnEndChatEvent) ChatEventKind() ChatEventType { return ChatEventTurnEnd }

type MessageDeltaChatEvent struct {
	Delta string
	Kind  string // "text" or "thinking"
}

func (MessageDeltaChatEvent) ChatEventKind() ChatEventType { return ChatEventMessageDelta }

type ToolCallInfo struct {
	ID        string
	Name      string
	Arguments string
}

type ToolCallStartChatEvent struct {
	ToolCall ToolCallInfo
}

func (ToolCallStartChatEvent) ChatEventKind() ChatEventType { return ChatEventToolCallStart }

type ToolCallEndChatEvent struct {
	ToolCallID string
	ToolName   string
	Result     string
	Err        error
	Duration   time.Duration
}

func (ToolCallEndChatEvent) ChatEventKind() ChatEventType { return ChatEventToolCallEnd }

type HandoffStartChatEvent struct {
	SourceAgent string
	TargetAgent string
	Mode        string
	Context     string
	Invisible   bool
}

func (HandoffStartChatEvent) ChatEventKind() ChatEventType { return ChatEventHandoffStart }

type HandoffEndChatEvent struct {
	TargetAgent string
	Output      string
	Duration    time.Duration
	Err         error
	Invisible   bool
}

func (HandoffEndChatEvent) ChatEventKind() ChatEventType { return ChatEventHandoffEnd }

type CompactionStartChatEvent struct {
	TokensBefore  int64
	ContextWindow int64
}

func (CompactionStartChatEvent) ChatEventKind() ChatEventType { return ChatEventCompactionStart }

type CompactionEndChatEvent struct {
	TokensBefore int64
	TokensAfter  int64
	MessagesCut  int64
	Duration     time.Duration
}

func (CompactionEndChatEvent) ChatEventKind() ChatEventType { return ChatEventCompactionEnd }

type AutoRetryChatEvent struct {
	Attempt    int64
	MaxRetries int64
	Delay      time.Duration
	Err        error
}

func (AutoRetryChatEvent) ChatEventKind() ChatEventType { return ChatEventAutoRetry }

type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}
