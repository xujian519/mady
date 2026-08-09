package agui

// EventType identifies the kind of AGUI event.
type EventType string

const (
	// EventRunStarted is emitted when an agent run begins.
	EventRunStarted EventType = "RUN_STARTED"
	// EventRunFinished is emitted when an agent run completes.
	EventRunFinished EventType = "RUN_FINISHED"
	// EventRunError is emitted when an agent run encounters a fatal error.
	EventRunError EventType = "RUN_ERROR"
	// EventStepStarted is emitted when a new turn within a run begins.
	EventStepStarted EventType = "STEP_STARTED"
	// EventStepFinished is emitted when a turn completes.
	EventStepFinished EventType = "STEP_FINISHED"
	// EventTextMessageStart is emitted at the start of a text message segment.
	EventTextMessageStart EventType = "TEXT_MESSAGE_START"
	// EventTextMessageContent carries a chunk of text message content.
	EventTextMessageContent EventType = "TEXT_MESSAGE_CONTENT"
	// EventTextMessageEnd is emitted when a text message segment ends.
	EventTextMessageEnd EventType = "TEXT_MESSAGE_END"
	// EventThinkingStart is emitted when the agent begins a thinking block.
	EventThinkingStart EventType = "THINKING_START"
	// EventThinkingTextMessageStart is emitted at the start of a thinking text message.
	EventThinkingTextMessageStart EventType = "THINKING_TEXT_MESSAGE_START"
	// EventThinkingTextMessageContent carries a chunk of thinking text content.
	EventThinkingTextMessageContent EventType = "THINKING_TEXT_MESSAGE_CONTENT"
	// EventThinkingTextMessageEnd is emitted when a thinking text message ends.
	EventThinkingTextMessageEnd EventType = "THINKING_TEXT_MESSAGE_END"
	// EventThinkingEnd is emitted when a thinking block ends.
	EventThinkingEnd EventType = "THINKING_END"
	// EventToolCallStart is emitted when the agent begins a tool call.
	EventToolCallStart EventType = "TOOL_CALL_START"
	// EventToolCallArgs carries the arguments of a tool call.
	EventToolCallArgs EventType = "TOOL_CALL_ARGS"
	// EventToolCallEnd is emitted when a tool call completes.
	EventToolCallEnd EventType = "TOOL_CALL_END"
	// EventToolCallResult carries the result of a tool call.
	EventToolCallResult EventType = "TOOL_CALL_RESULT"
	// EventStateSnapshot carries a full snapshot of agent state.
	EventStateSnapshot EventType = "STATE_SNAPSHOT"
	// EventStateDelta carries an incremental state delta.
	EventStateDelta EventType = "STATE_DELTA"
	// EventMessagesSnapshot carries a full snapshot of all messages.
	EventMessagesSnapshot EventType = "MESSAGES_SNAPSHOT"
	// EventCustom is emitted for custom or uncategorized events.
	EventCustom EventType = "CUSTOM"
	// EventRaw is emitted for raw protocol events.
	EventRaw EventType = "RAW"
	// EventContextUsage reports token usage context.
	EventContextUsage EventType = "CONTEXT_USAGE"
)

// BaseEvent is embedded in all AGUI events and carries the type and timestamp.
type BaseEvent struct {
	Type      EventType `json:"type"`
	Timestamp float64   `json:"timestamp,omitempty"`
	RawEvent  any       `json:"rawEvent,omitempty"`
}

// GetType returns the event type.
func (b BaseEvent) GetType() EventType { return b.Type }

// RunStartedEvent is emitted when an agent run begins.
type RunStartedEvent struct {
	BaseEvent
	ThreadID    string `json:"threadId"`
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
}

// RunFinishedEvent is emitted when an agent run completes.
type RunFinishedEvent struct {
	BaseEvent
	ThreadID string              `json:"threadId"`
	RunID    string              `json:"runId"`
	Result   any                 `json:"result,omitempty"`
	Outcome  *RunFinishedOutcome `json:"outcome,omitempty"`
	// FinishReason 是模型收尾轮次的结束原因（"stop"/"length"/"error" 等）。
	// "length" 表示输出触达 max_tokens 上限可能被截断；"error" 表示流异常
	// 终止。前端应据此提示用户输出可能不完整。
	FinishReason string `json:"finishReason,omitempty"`
}

// RunFinishedOutcome describes how the run finished.
type RunFinishedOutcome struct {
	Type       EventType   `json:"type"`
	Interrupts []Interrupt `json:"interrupts,omitempty"`
}

// Interrupt describes a human-in-the-loop interrupt that paused execution.
type Interrupt struct {
	ID             string         `json:"id"`
	Reason         string         `json:"reason"`
	Message        string         `json:"message,omitempty"`
	ToolCallID     string         `json:"toolCallId,omitempty"`
	ResponseSchema any            `json:"responseSchema,omitempty"`
	ExpiresAt      string         `json:"expiresAt,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// RunErrorEvent is emitted when an agent run encounters a fatal error.
type RunErrorEvent struct {
	BaseEvent
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
	Message  string `json:"message"`
	Code     string `json:"code,omitempty"`
}

// StepStartedEvent is emitted when a new turn within a run begins.
type StepStartedEvent struct {
	BaseEvent
	StepName string `json:"stepName"`
}

// StepFinishedEvent is emitted when a turn completes.
type StepFinishedEvent struct {
	BaseEvent
	StepName string `json:"stepName"`
}

// TextMessageStartEvent is emitted at the start of a text message segment.
type TextMessageStartEvent struct {
	BaseEvent
	MessageID string `json:"messageId"`
	Role      string `json:"role"`
}

// TextMessageContentEvent carries a chunk of text message content.
type TextMessageContentEvent struct {
	BaseEvent
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

// TextMessageEndEvent is emitted when a text message segment ends.
type TextMessageEndEvent struct {
	BaseEvent
	MessageID string `json:"messageId"`
}

// ThinkingStartEvent is emitted when the agent begins a thinking block.
type ThinkingStartEvent struct {
	BaseEvent
	ThinkingID string `json:"thinkingId"`
	Title      string `json:"title,omitempty"`
}

// ThinkingTextMessageStartEvent is emitted at the start of a thinking text message.
type ThinkingTextMessageStartEvent struct {
	BaseEvent
	ThinkingID string `json:"thinkingId"`
	MessageID  string `json:"messageId"`
}

// ThinkingTextMessageContentEvent carries a chunk of thinking text content.
type ThinkingTextMessageContentEvent struct {
	BaseEvent
	ThinkingID string `json:"thinkingId"`
	MessageID  string `json:"messageId"`
	Delta      string `json:"delta"`
}

// ThinkingTextMessageEndEvent is emitted when a thinking text message ends.
type ThinkingTextMessageEndEvent struct {
	BaseEvent
	ThinkingID string `json:"thinkingId"`
	MessageID  string `json:"messageId"`
}

// ThinkingEndEvent is emitted when a thinking block ends.
type ThinkingEndEvent struct {
	BaseEvent
	ThinkingID string `json:"thinkingId"`
}

// ContextUsageEvent 报告当前上下文窗口使用情况。
// 在每次 TurnEnd 时由 Converter 自动发出。
type ContextUsageEvent struct {
	BaseEvent
	TokenUsage struct {
		PromptTokens     int64 `json:"promptTokens"`
		CompletionTokens int64 `json:"completionTokens"`
		TotalTokens      int64 `json:"totalTokens"`
	} `json:"tokenUsage"`
	ContextWindow int64   `json:"contextWindow"`
	UsagePercent  float64 `json:"usagePercent"`
}

// ToolCallStartEvent is emitted when the agent begins a tool call.
type ToolCallStartEvent struct {
	BaseEvent
	ToolCallID      string `json:"toolCallId"`
	ToolCallName    string `json:"toolCallName"`
	ParentMessageID string `json:"parentMessageId,omitempty"`
}

// ToolCallArgsEvent carries the arguments of a tool call.
type ToolCallArgsEvent struct {
	BaseEvent
	ToolCallID string `json:"toolCallId"`
	Delta      string `json:"delta"`
}

// ToolCallEndEvent is emitted when a tool call completes.
type ToolCallEndEvent struct {
	BaseEvent
	ToolCallID string `json:"toolCallId"`
}

// ToolCallResultEvent carries the result of a tool call.
type ToolCallResultEvent struct {
	BaseEvent
	MessageID  string `json:"messageId"`
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content"`
	Role       string `json:"role,omitempty"`
}

// MessagesSnapshotEvent carries a full snapshot of all messages.
type MessagesSnapshotEvent struct {
	BaseEvent
	Messages []Message `json:"messages"`
}

// StateSnapshotEvent carries a full snapshot of agent state.
type StateSnapshotEvent struct {
	BaseEvent
	Snapshot any `json:"snapshot"`
}

// StateDeltaEvent carries an incremental state delta (JSON patch ops).
type StateDeltaEvent struct {
	BaseEvent
	Delta []jsonPatchOp `json:"delta"`
}

type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// CustomEvent is emitted for custom or uncategorized events.
type CustomEvent struct {
	BaseEvent
	Name  string `json:"name"`
	Value any    `json:"value,omitempty"`
}

// MessageRole identifies the role of a message sender.
type MessageRole string

const (
	// MessageRoleUser represents a user message.
	MessageRoleUser MessageRole = "user"
	// MessageRoleAssistant represents an assistant message.
	MessageRoleAssistant MessageRole = "assistant"
	// MessageRoleSystem represents a system message.
	MessageRoleSystem MessageRole = "system"
	// MessageRoleTool represents a tool result message.
	MessageRoleTool MessageRole = "tool"
	// MessageRoleDeveloper represents a developer message.
	MessageRoleDeveloper MessageRole = "developer"
)

// Message represents a single message in the conversation.
type Message struct {
	ID             string      `json:"id"`
	Role           MessageRole `json:"role"`
	Content        string      `json:"content,omitempty"`
	Name           string      `json:"name,omitempty"`
	ToolCalls      []ToolCall  `json:"toolCalls,omitempty"`
	ToolCallID     string      `json:"toolCallId,omitempty"`
	Error          string      `json:"error,omitempty"`
	EncryptedValue string      `json:"encryptedValue,omitempty"`
}

// ToolCall represents a function call invoked by the agent.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc describes the function being called.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// RunAgentInput is the request body for running an agent.
type RunAgentInput struct {
	ThreadID       string         `json:"threadId,omitempty"`
	RunID          string         `json:"runId,omitempty"`
	ParentRunID    string         `json:"parentRunId,omitempty"`
	Messages       []Message      `json:"messages,omitempty"`
	Tools          []ToolDef      `json:"tools,omitempty"`
	Context        []ContextEntry `json:"context,omitempty"`
	State          any            `json:"state,omitempty"`
	ForwardedProps any            `json:"forwardedProps,omitempty"`
	Resume         []ResumeEntry  `json:"resume,omitempty"`
}

// ResumeEntry describes a resolved interrupt to resume from.
type ResumeEntry struct {
	InterruptID string `json:"interruptId"`
	Status      string `json:"status"`
	Payload     any    `json:"payload,omitempty"`
}

// ToolDef describes a tool definition for capability reporting.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ContextEntry provides additional context to the agent.
type ContextEntry struct {
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`
}

// AgentCapabilities describes all capabilities of the agent.
type AgentCapabilities struct {
	Identity       *IdentityCapabilities       `json:"identity,omitempty"`
	Transport      *TransportCapabilities      `json:"transport,omitempty"`
	Tools          *ToolsCapabilities          `json:"tools,omitempty"`
	Output         *OutputCapabilities         `json:"output,omitempty"`
	State          *StateCapabilities          `json:"state,omitempty"`
	MultiAgent     *MultiAgentCapabilities     `json:"multiAgent,omitempty"`
	Reasoning      *ReasoningCapabilities      `json:"reasoning,omitempty"`
	Multimodal     *MultimodalCapabilities     `json:"multimodal,omitempty"`
	Execution      *ExecutionCapabilities      `json:"execution,omitempty"`
	HumanInTheLoop *HumanInTheLoopCapabilities `json:"humanInTheLoop,omitempty"`
	Custom         map[string]any              `json:"custom,omitempty"`
}

// IdentityCapabilities describes the agent's identity information.
type IdentityCapabilities struct {
	Name             string         `json:"name,omitempty"`
	Type             string         `json:"type,omitempty"`
	Description      string         `json:"description,omitempty"`
	Version          string         `json:"version,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	DocumentationURL string         `json:"documentationUrl,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// TransportCapabilities describes the transport mechanisms the agent supports.
type TransportCapabilities struct {
	Streaming         bool `json:"streaming,omitempty"`
	Websocket         bool `json:"websocket,omitempty"`
	HTTPBinary        bool `json:"httpBinary,omitempty"`
	PushNotifications bool `json:"pushNotifications,omitempty"`
	Resumable         bool `json:"resumable,omitempty"`
}

// ToolsCapabilities describes tool execution capabilities.
type ToolsCapabilities struct {
	Supported      bool      `json:"supported,omitempty"`
	Items          []ToolDef `json:"items,omitempty"`
	ParallelCalls  bool      `json:"parallelCalls,omitempty"`
	ClientProvided bool      `json:"clientProvided,omitempty"`
}

// OutputCapabilities describes output format capabilities.
type OutputCapabilities struct {
	StructuredOutput   bool     `json:"structuredOutput,omitempty"`
	SupportedMIMETypes []string `json:"supportedMimeTypes,omitempty"`
}

// StateCapabilities describes state persistence capabilities.
type StateCapabilities struct {
	Snapshots       bool `json:"snapshots,omitempty"`
	Deltas          bool `json:"deltas,omitempty"`
	Memory          bool `json:"memory,omitempty"`
	PersistentState bool `json:"persistentState,omitempty"`
}

// MultiAgentCapabilities describes multi-agent and handoff capabilities.
type MultiAgentCapabilities struct {
	Supported  bool                 `json:"supported,omitempty"`
	Delegation bool                 `json:"delegation,omitempty"`
	Handoffs   bool                 `json:"handoffs,omitempty"`
	SubAgents  []SubAgentDescriptor `json:"subAgents,omitempty"`
}

// SubAgentDescriptor describes a sub-agent available for delegation.
type SubAgentDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ReasoningCapabilities describes reasoning/thinking capabilities.
type ReasoningCapabilities struct {
	Supported bool `json:"supported,omitempty"`
	Streaming bool `json:"streaming,omitempty"`
	Encrypted bool `json:"encrypted,omitempty"`
}

// MultimodalCapabilities describes multimodal I/O capabilities.
type MultimodalCapabilities struct {
	Input  *MultimodalInputCapabilities  `json:"input,omitempty"`
	Output *MultimodalOutputCapabilities `json:"output,omitempty"`
}

// MultimodalInputCapabilities describes supported input modalities.
type MultimodalInputCapabilities struct {
	Image bool `json:"image,omitempty"`
	Audio bool `json:"audio,omitempty"`
	Video bool `json:"video,omitempty"`
	PDF   bool `json:"pdf,omitempty"`
	File  bool `json:"file,omitempty"`
}

// MultimodalOutputCapabilities describes supported output modalities.
type MultimodalOutputCapabilities struct {
	Image bool `json:"image,omitempty"`
	Audio bool `json:"audio,omitempty"`
}

// ExecutionCapabilities describes execution environment capabilities.
type ExecutionCapabilities struct {
	CodeExecution    bool  `json:"codeExecution,omitempty"`
	Sandboxed        bool  `json:"sandboxed,omitempty"`
	MaxIterations    int64 `json:"maxIterations,omitempty"`
	MaxExecutionTime int64 `json:"maxExecutionTime,omitempty"`
}

// HumanInTheLoopCapabilities describes human-in-the-loop interaction capabilities.
type HumanInTheLoopCapabilities struct {
	Supported        bool `json:"supported,omitempty"`
	Approvals        bool `json:"approvals,omitempty"`
	Interventions    bool `json:"interventions,omitempty"`
	Feedback         bool `json:"feedback,omitempty"`
	Interrupts       bool `json:"interrupts,omitempty"`
	ApproveWithEdits bool `json:"approveWithEdits,omitempty"`
}
