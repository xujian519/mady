package acp

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the current ACP protocol version number.
const ProtocolVersion = 1

// JSONRPCRequest represents a JSON-RPC 2.0 request sent by the client.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response sent by the server.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError carries structured error information per JSON-RPC 2.0.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("acp error %d: %s", e.Code, e.Message)
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification (no id).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Implementation identifies the client or server software.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities describes the capabilities the client advertises during initialize.
type ClientCapabilities struct {
	FS       *FileSystemCapability `json:"fs,omitempty"`
	Terminal bool                  `json:"terminal,omitempty"`
}

// FileSystemCapability describes the client's filesystem operations.
type FileSystemCapability struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

// AgentCapabilities describes what the server's agent supports.
type AgentCapabilities struct {
	LoadSession         bool                 `json:"loadSession,omitempty"`
	PromptCapabilities  *PromptCapabilities  `json:"promptCapabilities,omitempty"`
	SessionCapabilities *SessionCapabilities `json:"sessionCapabilities,omitempty"`
}

// PromptCapabilities describes which content types the prompt endpoint accepts.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// SessionCapabilities describes session lifecycle operations the server supports.
type SessionCapabilities struct {
	Fork   *SessionForkCapabilities   `json:"fork,omitempty"`
	List   *SessionListCapabilities   `json:"list,omitempty"`
	Resume *SessionResumeCapabilities `json:"resume,omitempty"`
}

// SessionForkCapabilities indicates the server supports session forking.
type SessionForkCapabilities struct{}

// SessionListCapabilities indicates the server supports session listing.
type SessionListCapabilities struct{}

// SessionResumeCapabilities indicates the server supports session resumption.
type SessionResumeCapabilities struct{}

// AuthMethodAgent describes an agent-based authentication method.
type AuthMethodAgent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// TerminalAuthMethod describes a terminal-based authentication method.
type TerminalAuthMethod struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Args        []string `json:"args,omitempty"`
}

// InitializeParams carries initialization parameters from the client.
type InitializeParams struct {
	ProtocolVersion    int                 `json:"protocolVersion,omitempty"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitempty"`
	ClientInfo         *Implementation     `json:"clientInfo,omitempty"`
}

// InitializeResult carries the server's initialization response.
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []any             `json:"authMethods,omitempty"`
}

// AuthenticateParams carries credentials for the authenticate handshake.
type AuthenticateParams struct {
	MethodID string `json:"methodId"`
	// Token 是 methodId 为 "token" 时携带的静态令牌（扩展字段，向后兼容）。
	Token string `json:"token,omitempty"`
}

// AuthenticateResult is an empty acknowledgment of successful authentication.
type AuthenticateResult struct{}

// ModelInfo describes a model available for agent execution.
type ModelInfo struct {
	ModelID     string `json:"modelId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SessionMode describes an execution mode available to the agent.
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SessionModeState captures the current mode and available modes.
type SessionModeState struct {
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

// SessionModelState captures the current model and available models.
type SessionModelState struct {
	AvailableModels []ModelInfo `json:"availableModels"`
	CurrentModelID  string      `json:"currentModelId"`
}

// NewSessionParams carries parameters for creating a new session.
type NewSessionParams struct {
	CWD        string `json:"cwd"`
	MCPServers []any  `json:"mcpServers,omitempty"`
}

// NewSessionResult carries the response for a new session request.
type NewSessionResult struct {
	SessionID string             `json:"sessionId"`
	Models    *SessionModelState `json:"models,omitempty"`
	Modes     *SessionModeState  `json:"modes,omitempty"`
}

// LoadSessionParams carries parameters for loading an existing session.
type LoadSessionParams struct {
	CWD       string `json:"cwd"`
	SessionID string `json:"sessionId"`
}

// LoadSessionResult carries the response for a load session request.
type LoadSessionResult struct {
	Models *SessionModelState `json:"models,omitempty"`
	Modes  *SessionModeState  `json:"modes,omitempty"`
}

// ResumeSessionParams carries parameters for resuming a session.
type ResumeSessionParams struct {
	CWD       string `json:"cwd"`
	SessionID string `json:"sessionId"`
}

// ResumeSessionResult carries the response for a resume session request.
type ResumeSessionResult struct {
	Models *SessionModelState `json:"models,omitempty"`
	Modes  *SessionModeState  `json:"modes,omitempty"`
}

// ForkSessionParams carries parameters for forking a session.
type ForkSessionParams struct {
	CWD       string `json:"cwd"`
	SessionID string `json:"sessionId"`
}

// ForkSessionResult carries the response for a fork session request.
type ForkSessionResult struct {
	SessionID string             `json:"sessionId"`
	Models    *SessionModelState `json:"models,omitempty"`
	Modes     *SessionModeState  `json:"modes,omitempty"`
}

// ListSessionsParams carries session listing filter parameters.
type ListSessionsParams struct {
	Cursor string `json:"cursor,omitempty"`
	CWD    string `json:"cwd,omitempty"`
}

// SessionInfo summarizes a single session for list responses.
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// ListSessionsResult carries the paginated session list response.
type ListSessionsResult struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// TextContentBlock is a text content block in an ACP prompt.
type TextContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ImageContentBlock is an image content block in an ACP prompt.
type ImageContentBlock struct {
	Type     string `json:"type"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}

// PromptResponse is the server's response to a session/prompt request.
type PromptResponse struct {
	StopReason string `json:"stopReason"`
	Usage      *Usage `json:"usage,omitempty"`
	// FinishReason 是模型收尾轮次的结束原因（"stop"/"length"/"error" 等）。
	// "length" 表示输出触达 max_tokens 上限可能被截断；"error" 表示流异常
	// 终止。客户端应据此提示用户输出可能不完整。
	FinishReason string `json:"finishReason,omitempty"`
}

// Usage reports token consumption for a prompt execution.
type Usage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	TotalTokens      int `json:"totalTokens"`
	ThoughtTokens    int `json:"thoughtTokens,omitempty"`
	CachedReadTokens int `json:"cachedReadTokens,omitempty"`
}

// CancelParams carries the session ID for cancellation.
type CancelParams struct {
	SessionID string `json:"sessionId"`
}

// SetSessionModeParams carries the new mode for a session.
type SetSessionModeParams struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

// SetSessionModelParams carries the new model for a session.
type SetSessionModelParams struct {
	SessionID string `json:"sessionId"`
	ModelID   string `json:"modelId"`
}

// SessionNotification is the params object for a "session/update" notification.
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate is the discriminated update payload, keyed by "sessionUpdate".
// Variants: user_message_chunk, agent_message_chunk, agent_thought_chunk,
// tool_call, tool_call_update, plan, available_commands_update,
// current_mode_update.
type SessionUpdate struct {
	SessionUpdate string `json:"sessionUpdate"`

	// *_message_chunk / agent_thought_chunk
	Content any `json:"content,omitempty"`

	// tool_call / tool_call_update
	ToolCallID string `json:"toolCallId,omitempty"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`
	RawInput   any    `json:"rawInput,omitempty"`
	RawOutput  any    `json:"rawOutput,omitempty"`

	// plan
	Entries []PlanEntry `json:"entries,omitempty"`

	// available_commands_update
	AvailableCommands []AvailableCommand `json:"availableCommands,omitempty"`

	// current_mode_update
	CurrentModeID string `json:"currentModeId,omitempty"`
}

// PlanEntry is a single step in the agent's execution plan.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// AvailableCommand describes a command the client can propose to the agent.
type AvailableCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputHint   string `json:"inputHint,omitempty"`
}

// --- Permission (agent -> client request: "session/request_permission") ---

// RequestPermissionParams carries the tool call and options for a permission request.
type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  PermissionToolCall `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionToolCall describes a tool invocation awaiting user permission.
type PermissionToolCall struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`
	RawInput   any    `json:"rawInput,omitempty"`
}

// PermissionOption represents one choice presented to the user for a permission request.
// Kind is one of: allow_once, allow_always, reject_once, reject_always.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// RequestPermissionResult wraps the user's permission outcome.
type RequestPermissionResult struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// PermissionOutcome is the user's decision on a permission request.
// Outcome is "selected" or "canceled".
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}
