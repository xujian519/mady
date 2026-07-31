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

// ChatEventType 是聊天事件的类型标识符（int 枚举）。
type ChatEventType int

const (
	ChatEventAgentStart ChatEventType = iota
	ChatEventAgentEnd
	ChatEventAgentError
	ChatEventTurnStart
	ChatEventTurnEnd
	ChatEventMessageDelta
	ChatEventToolCallStart
	ChatEventToolCallEnd
	ChatEventHandoffStart
	ChatEventHandoffEnd
	ChatEventCompactionStart
	ChatEventCompactionEnd
	ChatEventAutoRetry
	ChatEventAgentInterrupt
	ChatEventApprovalPrompt
	ChatEventTaskCreated
	ChatEventTaskUpdated
	ChatEventSkillLoaded
	ChatEventSkillsReloaded
	ChatEventA2UI
	ChatEventPlanTaskStatusChanged
	ChatEventPlanTaskFeedbackAdded
	ChatEventPlanTaskInterrupted
)

// chatEventTypeNames maps ChatEventType to descriptive strings for
// debugging and formatted output (e.g. test %q, log %v).
var chatEventTypeNames = map[ChatEventType]string{
	ChatEventAgentStart:            "agent_start",
	ChatEventAgentEnd:              "agent_end",
	ChatEventAgentError:            "agent_error",
	ChatEventTurnStart:             "turn_start",
	ChatEventTurnEnd:               "turn_end",
	ChatEventMessageDelta:          "message_delta",
	ChatEventToolCallStart:         "tool_call_start",
	ChatEventToolCallEnd:           "tool_call_end",
	ChatEventHandoffStart:          "handoff_start",
	ChatEventHandoffEnd:            "handoff_end",
	ChatEventCompactionStart:       "compaction_start",
	ChatEventCompactionEnd:         "compaction_end",
	ChatEventAutoRetry:             "auto_retry",
	ChatEventAgentInterrupt:        "agent_interrupt",
	ChatEventApprovalPrompt:        "approval_prompt",
	ChatEventTaskCreated:           "task_created",
	ChatEventTaskUpdated:           "task_updated",
	ChatEventSkillLoaded:           "skill_loaded",
	ChatEventSkillsReloaded:        "skills_reloaded",
	ChatEventA2UI:                  "a2ui",
	ChatEventPlanTaskStatusChanged: "plantask_status_changed",
	ChatEventPlanTaskFeedbackAdded: "plantask_feedback_added",
	ChatEventPlanTaskInterrupted:   "plantask_interrupted",
}

func (t ChatEventType) String() string {
	if name, ok := chatEventTypeNames[t]; ok {
		return name
	}
	return "unknown"
}

// GoString implements fmt.GoStringer for test output compatibility.
func (t ChatEventType) GoString() string { return t.String() }

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

// TaskInfo 是 agentcore.Task 的 TUI 层投影，避免 tui/chat 直接依赖 agentcore。
// 由 agentadapter 层负责从 agentcore.Task 转换。
type TaskInfo struct {
	ID       string
	Subject  string
	Status   string
	Priority string
}

// TaskCreatedChatEvent 在任务创建时触发，供 TodoPanel 实时刷新。
type TaskCreatedChatEvent struct {
	Task *TaskInfo
}

func (TaskCreatedChatEvent) ChatEventKind() ChatEventType { return ChatEventTaskCreated }

// TaskUpdatedChatEvent 在任务更新时触发。
type TaskUpdatedChatEvent struct {
	Task *TaskInfo
}

func (TaskUpdatedChatEvent) ChatEventKind() ChatEventType { return ChatEventTaskUpdated }

// SkillLoadedChatEvent is emitted when a skill is loaded.
// tui/chat does not import agentcore; this is the projection layer.
type SkillLoadedChatEvent struct {
	SkillName string
	Path      string
	Source    string
	Arguments string
}

func (SkillLoadedChatEvent) ChatEventKind() ChatEventType { return ChatEventSkillLoaded }

// SkillsReloadedChatEvent is emitted when skills are hot-reloaded.
type SkillsReloadedChatEvent struct {
	SkillPaths       []string
	TotalSkills      int
	VisibleSkills    int
	HiddenSkills     int
	DiagnosticsCount int
	AddedSkills      []string
	RemovedSkills    []string
	UpdatedSkills    []string
}

func (SkillsReloadedChatEvent) ChatEventKind() ChatEventType { return ChatEventSkillsReloaded }

// A2UIChatEvent carries the A2UI envelope map, used by the TUI to render
// declarative UI components (surfaces, dynamic bindings, etc.).
type A2UIChatEvent struct {
	Envelope map[string]any
}

func (A2UIChatEvent) ChatEventKind() ChatEventType { return ChatEventA2UI }

// PlanTaskStatusChangedChatEvent 在 HCL 会话状态迁移时触发。
type PlanTaskStatusChangedChatEvent struct {
	SessionID  string
	CaseID     string
	FromStatus string
	ToStatus   string
}

func (PlanTaskStatusChangedChatEvent) ChatEventKind() ChatEventType {
	return ChatEventPlanTaskStatusChanged
}

// PlanTaskFeedbackAddedChatEvent 在用户反馈注入时触发。
type PlanTaskFeedbackAddedChatEvent struct {
	SessionID string
	Text      string
	StepID    string
}

func (PlanTaskFeedbackAddedChatEvent) ChatEventKind() ChatEventType {
	return ChatEventPlanTaskFeedbackAdded
}

// PlanTaskInterruptedChatEvent 在执行中断时触发。
type PlanTaskInterruptedChatEvent struct {
	SessionID string
	StepID    string
	Reason    string
}

func (PlanTaskInterruptedChatEvent) ChatEventKind() ChatEventType {
	return ChatEventPlanTaskInterrupted
}
