package chat

import (
	"time"

	"github.com/xujian519/mady/tui/component"
)

// EventSubscriber 是聊天事件的订阅者接口：调用方通过 On 注册某类
// ChatEventType 的处理函数，由事件源在事件发生时回调。
type EventSubscriber interface {
	On(eventType ChatEventType, handler func(ChatEvent))
}

// Subscriber 是事件源接口：暴露 Subscribe 让 EventSubscriber 注册自身，
// 解耦事件生产方（chat host）与消费方（agentadapter 等）。
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

// ChatEvent 是所有聊天事件的标记接口：每个具体事件类型必须实现
// ChatEventKind 返回自身的 ChatEventType，供分发器路由。
type ChatEvent interface {
	ChatEventKind() ChatEventType
}

// AgentStartChatEvent 在 agent 开始处理一轮输入时发射，携带 agent 名与原始输入。
type AgentStartChatEvent struct {
	AgentName string
	Input     string
}

func (AgentStartChatEvent) ChatEventKind() ChatEventType { return ChatEventAgentStart }

// AgentEndChatEvent 在 agent 完成一轮时发射，携带输出文本与结束原因。
// FinishReason 为 "length"/"error" 等时需提示用户输出可能不完整。
type AgentEndChatEvent struct {
	AgentName string
	Output    string
	// FinishReason 是模型收尾轮次的结束原因（如 "stop"/"length"/"error"）。
	// "length" 表示输出触达 max_tokens 上限可能被截断；"error" 表示流异常终止；
	// 空值表示正常结束或原因未知。
	FinishReason string
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

// AgentErrorChatEvent 在 agent 执行出错时发射，Err 为具体错误。
type AgentErrorChatEvent struct {
	Err error
}

func (AgentErrorChatEvent) ChatEventKind() ChatEventType { return ChatEventAgentError }

// TurnStartChatEvent 在单个推理轮次开始时发射，Turn 为轮次序号。
type TurnStartChatEvent struct {
	Turn int64
}

func (TurnStartChatEvent) ChatEventKind() ChatEventType { return ChatEventTurnStart }

// TurnEndChatEvent 在单个推理轮次结束时发射，携带轮次序号与 token 用量。
type TurnEndChatEvent struct {
	Turn  int64
	Usage TokenUsage
}

func (TurnEndChatEvent) ChatEventKind() ChatEventType { return ChatEventTurnEnd }

// MessageDeltaChatEvent 在流式输出产生增量文本时发射，
// Delta 为增量内容，Kind 为 "text" 或 "thinking"。
type MessageDeltaChatEvent struct {
	Delta string
	Kind  string // "text" or "thinking"
}

func (MessageDeltaChatEvent) ChatEventKind() ChatEventType { return ChatEventMessageDelta }

// ToolCallInfo 描述一次工具调用的标识信息，用于工具卡片渲染。
type ToolCallInfo struct {
	ID        string
	Name      string
	Arguments string
}

// ToolCallStartChatEvent 在工具调用开始时发射。
type ToolCallStartChatEvent struct {
	ToolCall ToolCallInfo
}

func (ToolCallStartChatEvent) ChatEventKind() ChatEventType { return ChatEventToolCallStart }

// ToolCallEndChatEvent 在工具调用结束时发射，携带结果、错误与耗时。
type ToolCallEndChatEvent struct {
	ToolCallID string
	ToolName   string
	Result     string
	Err        error
	Duration   time.Duration
}

func (ToolCallEndChatEvent) ChatEventKind() ChatEventType { return ChatEventToolCallEnd }

// HandoffStartChatEvent 在 agent 间移交开始时发射，携带源/目标 agent、
// 移交模式与上下文；Invisible 为 true 时不向用户展示移交过程。
type HandoffStartChatEvent struct {
	SourceAgent string
	TargetAgent string
	Mode        string
	Context     string
	Invisible   bool
}

func (HandoffStartChatEvent) ChatEventKind() ChatEventType { return ChatEventHandoffStart }

// HandoffEndChatEvent 在 agent 间移交结束时发射，携带目标 agent 输出、
// 耗时与错误；Invisible 为 true 时不向用户展示。
type HandoffEndChatEvent struct {
	TargetAgent string
	Output      string
	Duration    time.Duration
	Err         error
	Invisible   bool
}

func (HandoffEndChatEvent) ChatEventKind() ChatEventType { return ChatEventHandoffEnd }

// CompactionStartChatEvent 在上下文压缩开始时发射，携带压缩前 token 数
// 与上下文窗口大小。
type CompactionStartChatEvent struct {
	TokensBefore  int64
	ContextWindow int64
}

func (CompactionStartChatEvent) ChatEventKind() ChatEventType { return ChatEventCompactionStart }

// CompactionEndChatEvent 在上下文压缩结束时发射，携带压缩前后 token 数、
// 裁剪消息数与耗时。
type CompactionEndChatEvent struct {
	TokensBefore int64
	TokensAfter  int64
	MessagesCut  int64
	Duration     time.Duration
}

func (CompactionEndChatEvent) ChatEventKind() ChatEventType { return ChatEventCompactionEnd }

// AutoRetryChatEvent 在 agent 自动重试时发射，携带当前尝试次数、
// 最大重试次数、重试延迟与触发错误。
type AutoRetryChatEvent struct {
	Attempt    int64
	MaxRetries int64
	Delay      time.Duration
	Err        error
}

func (AutoRetryChatEvent) ChatEventKind() ChatEventType { return ChatEventAutoRetry }

// TokenUsage 描述一次轮次或请求的 token 消耗明细（prompt/completion/total）。
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
