package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/skill"
)

// EventType 是 Agent 生命周期事件类型的字符串别名。
type EventType string

const (
	EventAgentStart      EventType = "agent_start"
	EventAgentEnd        EventType = "agent_end"
	EventAgentError      EventType = "agent_error"
	EventSkillLoaded     EventType = "skill_loaded"
	EventSkillsReloaded  EventType = "skills_reloaded"
	EventTurnStart       EventType = "turn_start"
	EventTurnEnd         EventType = "turn_end"
	EventMessageDelta    EventType = "message_delta"
	EventToolCallStart   EventType = "tool_call_start"
	EventToolCallEnd     EventType = "tool_call_end"
	EventHandoffStart    EventType = "handoff_start"
	EventHandoffEnd      EventType = "handoff_end"
	EventCompactionStart EventType = "compaction_start"
	EventCompactionEnd   EventType = "compaction_end"
	EventAutoRetry       EventType = "auto_retry"
	EventAgentInterrupt  EventType = "agent_interrupt"
	EventApprovalPrompt  EventType = "approval_prompt"
	EventA2UI            EventType = "a2ui"
)

// Event is the common interface for all agent lifecycle events.
type Event interface {
	EventKind() EventType
	EventTime() time.Time
}

type baseEvent struct {
	Kind EventType `json:"type"`
	At   time.Time `json:"timestamp"`
}

func (e baseEvent) EventKind() EventType { return e.Kind }
func (e baseEvent) EventTime() time.Time { return e.At }

func newBase(t EventType) baseEvent {
	return baseEvent{Kind: t, At: time.Now()}
}

// AgentStartEvent 是 Agent 开始执行时触发的事件。
type AgentStartEvent struct {
	baseEvent
	AgentName string `json:"agent_name,omitempty"`
	Input     string `json:"input,omitempty"`
}

// NewAgentStartEvent 构造一个 AgentStartEvent。
func NewAgentStartEvent(agentName, input string) *AgentStartEvent {
	return &AgentStartEvent{baseEvent: newBase(EventAgentStart), AgentName: agentName, Input: input}
}

// AgentEndEvent 是 Agent 执行完成时触发的事件。
type AgentEndEvent struct {
	baseEvent
	AgentName string `json:"agent_name,omitempty"`
	Output    string `json:"output"`
}

// NewAgentEndEvent 构造一个 AgentEndEvent。
func NewAgentEndEvent(agentName, output string) *AgentEndEvent {
	return &AgentEndEvent{baseEvent: newBase(EventAgentEnd), AgentName: agentName, Output: output}
}

// AgentErrorEvent 是 Agent 执行错误时触发的事件。
type AgentErrorEvent struct {
	baseEvent
	Err error `json:"error"`
}

// NewAgentErrorEvent 构造一个 AgentErrorEvent。
func NewAgentErrorEvent(err error) *AgentErrorEvent {
	return &AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: err}
}

// AgentInterruptEvent 是 Agent 被中断时触发的事件。
type AgentInterruptEvent struct {
	baseEvent
	AgentName string           `json:"agent_name,omitempty"`
	Reason    *InterruptReason `json:"reason,omitempty"`
}

// NewAgentInterruptEvent 构造一个 AgentInterruptEvent。
func NewAgentInterruptEvent(agentName string, reason *InterruptReason) *AgentInterruptEvent {
	return &AgentInterruptEvent{baseEvent: newBase(EventAgentInterrupt), AgentName: agentName, Reason: reason}
}

// SkillLoadedEvent 是技能加载完成时触发的事件。
type SkillLoadedEvent struct {
	baseEvent
	SkillName string `json:"skill_name"`
	Path      string `json:"path,omitempty"`
	Source    string `json:"source"`
	Arguments string `json:"arguments,omitempty"`
}

// NewSkillLoadedEvent 构造一个 SkillLoadedEvent。
func NewSkillLoadedEvent(skillName, path, source, arguments string) *SkillLoadedEvent {
	return &SkillLoadedEvent{baseEvent: newBase(EventSkillLoaded), SkillName: skillName, Path: path, Source: source, Arguments: arguments}
}

// SkillsReloadedEvent 是技能热重载完成时触发的事件。
type SkillsReloadedEvent struct {
	baseEvent
	SkillPaths         []string           `json:"skill_paths,omitempty"`
	TotalSkills        int                `json:"total_skills"`
	VisibleSkills      int                `json:"visible_skills"`
	HiddenSkills       int                `json:"hidden_skills"`
	DiagnosticsCount   int                `json:"diagnostics_count"`
	AddedSkills        []string           `json:"added_skills,omitempty"`
	RemovedSkills      []string           `json:"removed_skills,omitempty"`
	UpdatedSkills      []string           `json:"updated_skills,omitempty"`
	AddedDiagnostics   []skill.Diagnostic `json:"added_diagnostics,omitempty"`
	RemovedDiagnostics []skill.Diagnostic `json:"removed_diagnostics,omitempty"`
}

func NewSkillsReloadedEvent(
	skillPaths []string,
	totalSkills, visibleSkills, hiddenSkills, diagnosticsCount int,
	addedSkills, removedSkills, updatedSkills []string,
	addedDiagnostics, removedDiagnostics []skill.Diagnostic,
) SkillsReloadedEvent {
	return SkillsReloadedEvent{
		baseEvent:          newBase(EventSkillsReloaded),
		SkillPaths:         append([]string(nil), skillPaths...),
		TotalSkills:        totalSkills,
		VisibleSkills:      visibleSkills,
		HiddenSkills:       hiddenSkills,
		DiagnosticsCount:   diagnosticsCount,
		AddedSkills:        append([]string(nil), addedSkills...),
		RemovedSkills:      append([]string(nil), removedSkills...),
		UpdatedSkills:      append([]string(nil), updatedSkills...),
		AddedDiagnostics:   append([]skill.Diagnostic(nil), addedDiagnostics...),
		RemovedDiagnostics: append([]skill.Diagnostic(nil), removedDiagnostics...),
	}
}

// TurnStartEvent 是 Agent 内部循环开始一轮时触发的事件。
type TurnStartEvent struct {
	baseEvent
	Turn int64 `json:"turn"`
}

// NewTurnStartEvent 构造一个 TurnStartEvent。
func NewTurnStartEvent(turn int64) *TurnStartEvent {
	return &TurnStartEvent{baseEvent: newBase(EventTurnStart), Turn: turn}
}

// TurnEndEvent 是 Agent 内部循环结束一轮时触发的事件。
type TurnEndEvent struct {
	baseEvent
	Turn  int64      `json:"turn"`
	Usage TokenUsage `json:"usage"`
}

// NewTurnEndEvent 构造一个 TurnEndEvent。
func NewTurnEndEvent(turn int64, usage TokenUsage) *TurnEndEvent {
	return &TurnEndEvent{baseEvent: newBase(EventTurnEnd), Turn: turn, Usage: usage}
}

// MessageDeltaEvent 是 LLM 流式输出消息片段时触发的事件。
type MessageDeltaEvent struct {
	baseEvent
	Delta string    `json:"delta"`
	Kind  BlockKind `json:"kind,omitempty"`
}

// NewMessageDeltaEvent 构造一个 MessageDeltaEvent。
func NewMessageDeltaEvent(delta string, kind BlockKind) *MessageDeltaEvent {
	return &MessageDeltaEvent{baseEvent: newBase(EventMessageDelta), Delta: delta, Kind: kind}
}

// ToolCallStartEvent 是工具调用开始时触发的事件。
type ToolCallStartEvent struct {
	baseEvent
	ToolCall ToolCall `json:"tool_call"`
}

// NewToolCallStartEvent 构造一个 ToolCallStartEvent。
func NewToolCallStartEvent(toolCall ToolCall) *ToolCallStartEvent {
	return &ToolCallStartEvent{baseEvent: newBase(EventToolCallStart), ToolCall: toolCall}
}

// ToolCallEndEvent 是工具调用结束时触发的事件。
type ToolCallEndEvent struct {
	baseEvent
	ToolCallID string        `json:"tool_call_id"`
	ToolName   string        `json:"tool_name"`
	Result     string        `json:"result"`
	Err        error         `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// NewToolCallEndEvent 构造一个 ToolCallEndEvent。
func NewToolCallEndEvent(toolCallID, toolName, result string, err error, duration time.Duration) *ToolCallEndEvent {
	return &ToolCallEndEvent{baseEvent: newBase(EventToolCallEnd), ToolCallID: toolCallID, ToolName: toolName, Result: result, Err: err, Duration: duration}
}

// HandoffStartEvent 是 Handoff 交接开始时触发的事件。
type HandoffStartEvent struct {
	baseEvent
	SourceAgent string `json:"source_agent"`
	TargetAgent string `json:"target_agent"`
	Mode        string `json:"mode"`
	Context     string `json:"context"`
	Invisible   bool   `json:"invisible,omitempty"`
}

// NewHandoffStartEvent 构造一个 HandoffStartEvent。
func NewHandoffStartEvent(sourceAgent, targetAgent, mode, context string, invisible bool) *HandoffStartEvent {
	return &HandoffStartEvent{baseEvent: newBase(EventHandoffStart), SourceAgent: sourceAgent, TargetAgent: targetAgent, Mode: mode, Context: context, Invisible: invisible}
}

// HandoffEndEvent 是 Handoff 交接结束时触发的事件。
type HandoffEndEvent struct {
	baseEvent
	TargetAgent string        `json:"target_agent"`
	Output      string        `json:"output"`
	Duration    time.Duration `json:"duration"`
	Err         error         `json:"error,omitempty"`
	Invisible   bool          `json:"invisible"`
}

// NewHandoffEndEvent 构造一个 HandoffEndEvent。
func NewHandoffEndEvent(targetAgent, output string, duration time.Duration, err error, invisible bool) *HandoffEndEvent {
	return &HandoffEndEvent{baseEvent: newBase(EventHandoffEnd), TargetAgent: targetAgent, Output: output, Duration: duration, Err: err, Invisible: invisible}
}

// CompactionStartEvent 是上下文压缩开始时触发的事件。
type CompactionStartEvent struct {
	baseEvent
	TokensBefore  int64 `json:"tokens_before"`
	ContextWindow int64 `json:"context_window"`
}

// NewCompactionStartEvent 构造一个 CompactionStartEvent。
func NewCompactionStartEvent(tokensBefore, contextWindow int64) *CompactionStartEvent {
	return &CompactionStartEvent{baseEvent: newBase(EventCompactionStart), TokensBefore: tokensBefore, ContextWindow: contextWindow}
}

// CompactionEndEvent 是上下文压缩结束时触发的事件。
type CompactionEndEvent struct {
	baseEvent
	TokensBefore int64         `json:"tokens_before"`
	TokensAfter  int64         `json:"tokens_after"`
	MessagesCut  int64         `json:"messages_cut"`
	Duration     time.Duration `json:"duration"`
}

// NewCompactionEndEvent 构造一个 CompactionEndEvent。
func NewCompactionEndEvent(tokensBefore, tokensAfter, messagesCut int64, duration time.Duration) *CompactionEndEvent {
	return &CompactionEndEvent{baseEvent: newBase(EventCompactionEnd), TokensBefore: tokensBefore, TokensAfter: tokensAfter, MessagesCut: messagesCut, Duration: duration}
}

// AutoRetryEvent 是自动重试发生时触发的事件。
type AutoRetryEvent struct {
	baseEvent
	Attempt    int64         `json:"attempt"`
	MaxRetries int64         `json:"max_retries"`
	Delay      time.Duration `json:"delay"`
	Err        error         `json:"error"`
}

// NewAutoRetryEvent 构造一个 AutoRetryEvent。
func NewAutoRetryEvent(attempt, maxRetries int64, delay time.Duration, err error) *AutoRetryEvent {
	return &AutoRetryEvent{baseEvent: newBase(EventAutoRetry), Attempt: attempt, MaxRetries: maxRetries, Delay: delay, Err: err}
}

// A2UIEvent 是 Agent 发射 A2UI 声明式 UI 信封时触发的事件。该事件携带
// 完整的 A2UI 信封数据（以 map[string]any 形式），通过 AG-UI 的 CUSTOM
// 事件通道（name: "a2ui"）传输给前端渲染器。
type A2UIEvent struct {
	baseEvent
	// Envelope 是 A2UI 协议信封的 JSON 兼容表示，包含 createSurface、
	// updateComponents、updateDataModel 或 deleteSurface 之一。
	Envelope map[string]any `json:"envelope"`
}

// NewA2UIEvent 构造一个 A2UIEvent。
func NewA2UIEvent(envelope map[string]any) *A2UIEvent {
	return &A2UIEvent{baseEvent: newBase(EventA2UI), Envelope: envelope}
}

// ApprovalPromptEvent 是 ApprovalGate 触发人工审核时发射的事件。
// TUI 适配层监听此事件并在聊天中渲染审批卡片（approval_card 组件）。
// Data 字段携带可选的复核门结构化数据（如 judgment/confidence/evidences 等）。
type ApprovalPromptEvent struct {
	baseEvent
	ToolCalls []ToolCall     `json:"tool_calls,omitempty"`
	Content   string         `json:"content"`
	AgentName string         `json:"agent_name,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// NewApprovalPromptEvent 构造一个 ApprovalPromptEvent。
func NewApprovalPromptEvent(agentName, content string, toolCalls []ToolCall) *ApprovalPromptEvent {
	return &ApprovalPromptEvent{
		baseEvent: newBase(EventApprovalPrompt),
		AgentName: agentName,
		Content:   content,
		ToolCalls: toolCalls,
	}
}

// --- JSON serialization for events with error fields ---

func (e AgentErrorEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type      EventType `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		Error     string    `json:"error"`
		ErrorType string    `json:"error_type,omitempty"`
	}{e.Kind, e.At, util.ErrorString(e.Err), errorType(e.Err)})
}

func (e *AgentErrorEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type      EventType `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		Error     string    `json:"error"`
		ErrorType string    `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

func (e ToolCallEndEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		ToolCallID string        `json:"tool_call_id"`
		ToolName   string        `json:"tool_name"`
		Result     string        `json:"result"`
		Duration   time.Duration `json:"duration"`
		Error      string        `json:"error,omitempty"`
		ErrorType  string        `json:"error_type,omitempty"`
	}{e.Kind, e.At, e.ToolCallID, e.ToolName, e.Result, e.Duration, util.ErrorString(e.Err), errorType(e.Err)})
}

func (e *ToolCallEndEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		ToolCallID string        `json:"tool_call_id"`
		ToolName   string        `json:"tool_name"`
		Result     string        `json:"result"`
		Duration   time.Duration `json:"duration"`
		Error      string        `json:"error,omitempty"`
		ErrorType  string        `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	e.ToolCallID = raw.ToolCallID
	e.ToolName = raw.ToolName
	e.Result = raw.Result
	e.Duration = raw.Duration
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

func (e HandoffEndEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        EventType     `json:"type"`
		Timestamp   time.Time     `json:"timestamp"`
		TargetAgent string        `json:"target_agent"`
		Output      string        `json:"output"`
		Duration    time.Duration `json:"duration"`
		Error       string        `json:"error,omitempty"`
		ErrorType   string        `json:"error_type,omitempty"`
	}{e.Kind, e.At, e.TargetAgent, e.Output, e.Duration, util.ErrorString(e.Err), errorType(e.Err)})
}

func (e *HandoffEndEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type        EventType     `json:"type"`
		Timestamp   time.Time     `json:"timestamp"`
		TargetAgent string        `json:"target_agent"`
		Output      string        `json:"output"`
		Duration    time.Duration `json:"duration"`
		Error       string        `json:"error,omitempty"`
		ErrorType   string        `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	e.TargetAgent = raw.TargetAgent
	e.Output = raw.Output
	e.Duration = raw.Duration
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

func (e AutoRetryEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		Attempt    int64         `json:"attempt"`
		MaxRetries int64         `json:"max_retries"`
		Delay      time.Duration `json:"delay"`
		Error      string        `json:"error"`
		ErrorType  string        `json:"error_type,omitempty"`
	}{e.Kind, e.At, e.Attempt, e.MaxRetries, e.Delay, util.ErrorString(e.Err), errorType(e.Err)})
}

func (e *AutoRetryEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type       EventType     `json:"type"`
		Timestamp  time.Time     `json:"timestamp"`
		Attempt    int64         `json:"attempt"`
		MaxRetries int64         `json:"max_retries"`
		Delay      time.Duration `json:"delay"`
		Error      string        `json:"error"`
		ErrorType  string        `json:"error_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Kind = raw.Type
	e.At = raw.Timestamp
	e.Attempt = raw.Attempt
	e.MaxRetries = raw.MaxRetries
	e.Delay = raw.Delay
	if raw.Error != "" {
		e.Err = reconstructError(raw.Error, raw.ErrorType)
	}
	return nil
}

// errorType returns a stable type identifier for an error.
func errorType(err error) string {
	if err == nil {
		return ""
	}
	switch err {
	case context.Canceled:
		return "context.Canceled"
	case context.DeadlineExceeded:
		return "context.DeadlineExceeded"
	}
	return ""
}

// reconstructError rebuilds an error from its string representation and type hint.
func reconstructError(msg, errType string) error {
	switch errType {
	case "context.Canceled":
		return context.Canceled
	case "context.DeadlineExceeded":
		return context.DeadlineExceeded
	}
	return errors.New(msg)
}

// EventHandler is a callback invoked when an event is emitted.
type EventHandler func(Event)

// EventBus provides async pub/sub for agent lifecycle events.
// Events are dispatched via an internal Broker for fan-out delivery.
// Event ordering is preserved — a single goroutine processes events sequentially.
//
// Delivery semantics:
//   - Emit: best-effort, non-blocking. Events are dropped when the internal
//     buffer is full (with a warning log and drop counter). Suitable for
//     high-frequency streaming deltas.
//   - EmitMustDeliver: bounded-blocking (50ms per-subscriber timeout). Suitable
//     for terminal events (finish, tool result, error, cancel).
//
// Handlers are keyed by a monotonic ID so they can be removed individually
// (Go func values can't be compared with ==). This matters for long-lived
// agents whose EventBus is reused across requests: a per-request handler
// (e.g. an SSE writer closure) must be unregistered when the request ends,
// otherwise it leaks and keeps writing to a stale sink on subsequent requests.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType]map[uint64]EventHandler
	global   map[uint64]EventHandler
	nextID   atomic.Uint64
	broker   *Broker[Event]
	done     chan struct{}
	closed   bool
}

func NewEventBus() *EventBus {
	eb := &EventBus{
		handlers: make(map[EventType]map[uint64]EventHandler),
		global:   make(map[uint64]EventHandler),
		broker:   NewBroker[Event](),
		done:     make(chan struct{}),
	}
	ready := make(chan struct{})
	go eb.dispatch(ready)
	<-ready
	return eb
}

func (eb *EventBus) dispatch(ready chan<- struct{}) {
	defer close(eb.done)

	ctx := context.Background()
	ch := eb.broker.Subscribe(ctx)
	close(ready) // signal that the subscriber is registered

	for e := range ch {
		if ds, ok := e.(*drainSentinel); ok {
			close(ds.ack)
			continue
		}

		eb.mu.RLock()
		globals := make([]EventHandler, 0, len(eb.global))
		for _, h := range eb.global {
			globals = append(globals, h)
		}
		typedMap := eb.handlers[e.EventKind()]
		typed := make([]EventHandler, 0, len(typedMap))
		for _, h := range typedMap {
			typed = append(typed, h)
		}
		eb.mu.RUnlock()

		for _, h := range globals {
			eb.safeCall(h, e)
		}
		for _, h := range typed {
			eb.safeCall(h, e)
		}
	}
}

// safeCall invokes a handler with panic recovery. A panicking handler is
// logged to stderr but does NOT kill the dispatch goroutine — without this
// guard, a single buggy handler would permanently take down the event bus
// (dispatch exits, close(done) fires, all subsequent events are silently
// dropped once the channel fills). Other handlers and subsequent events
// must continue to flow.
func (eb *EventBus) safeCall(h EventHandler, e Event) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "agentcore: event handler panicked (event=%s): %v\n%s\n", e.EventKind(), r, debugStack())
		}
	}()
	h(e)
}

// debugStack returns a short stack trace for panic diagnostics.
func debugStack() string {
	buf := make([]byte, 2048)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// On registers a handler for a specific event type and returns a function
// that removes the handler when called. Callers that attach scoped handlers
// (e.g. per-request SSE writers on a long-lived agent) MUST invoke the
// returned function when their scope ends — otherwise the handler stays
// registered on a reused agent and leaks, writing to a dead/stale sink.
func (eb *EventBus) On(t EventType, h EventHandler) func() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if eb.closed {
		return nil
	}
	id := eb.nextID.Add(1)
	if eb.handlers[t] == nil {
		eb.handlers[t] = make(map[uint64]EventHandler)
	}
	eb.handlers[t][id] = h
	return func() { eb.offID(t, id) }
}

// OnAll registers a handler that receives every event and returns a function
// that removes the handler when called. See On for the scoping contract.
func (eb *EventBus) OnAll(h EventHandler) func() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if eb.closed {
		return nil
	}
	id := eb.nextID.Add(1)
	eb.global[id] = h
	return func() { eb.offAllID(id) }
}

// offID removes a typed handler by its registration ID. Idempotent.
func (eb *EventBus) offID(t EventType, id uint64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.handlers[t], id)
}

// offAllID removes a global handler by its registration ID. Idempotent.
func (eb *EventBus) offAllID(id uint64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.global, id)
}

// Emit dispatches an event to the async processing goroutine.
//
// Delivery is non-blocking and lossy: if the internal buffer is full the
// event is dropped (with a warning log and drop counter). This is the right
// choice for high-frequency streaming deltas. Use EmitMustDeliver for
// terminal events that must not be silently dropped.
func (eb *EventBus) Emit(e Event) {
	eb.mu.Lock()
	if eb.closed {
		eb.mu.Unlock()
		return
	}
	eb.mu.Unlock()
	eb.broker.Publish(e)
}

// EmitMustDeliver dispatches an event with bounded-blocking semantics.
//
// For each subscriber it first attempts a non-blocking send, then falls back
// to a blocking send with a 50ms timeout. Use this for terminal events
// (finish, tool result, error, cancel) that must reach handlers.
func (eb *EventBus) EmitMustDeliver(ctx context.Context, e Event) {
	eb.mu.Lock()
	if eb.closed {
		eb.mu.Unlock()
		return
	}
	eb.mu.Unlock()
	eb.broker.PublishMustDeliver(ctx, e)
}

// DropCount returns the cumulative number of events dropped by Emit
// because the internal buffer was full.
func (eb *EventBus) DropCount() uint64 {
	return eb.broker.DropCount()
}

// MustDeliverDropCount returns the cumulative number of events dropped by
// EmitMustDeliver after the per-subscriber timeout expired.
func (eb *EventBus) MustDeliverDropCount() uint64 {
	return eb.broker.MustDeliverDropCount()
}

// Subscribe returns a channel that receives raw events from the event bus.
// The channel is closed when ctx is canceled or the event bus is shut down.
// Use this for consumers that want direct channel-based delivery (e.g. SSE
// writers) instead of callback-based handlers.
func (eb *EventBus) Subscribe(ctx context.Context) <-chan Event {
	return eb.broker.Subscribe(ctx)
}

// Close shuts down the event bus. All queued events are processed before Close returns.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	if eb.closed {
		eb.mu.Unlock()
		return
	}
	eb.closed = true
	eb.mu.Unlock()
	eb.broker.Shutdown()
	<-eb.done
}

// Drain blocks until all currently queued events have been processed.
// Uses PublishMustDeliver to ensure the drain sentinel reaches the dispatch
// goroutine even when the internal buffer is full, with a timeout guard to
// prevent hanging if the broker was shut down concurrently.
func (eb *EventBus) Drain() {
	ack := make(chan struct{})
	ds := &drainSentinel{baseEvent: newBase("drain"), ack: ack}
	eb.broker.PublishMustDeliver(context.Background(), ds)

	// If the broker was shut down after we published (TOCTOU with Close),
	// the sentinel was not delivered. Check and return immediately.
	eb.mu.Lock()
	closed := eb.closed
	eb.mu.Unlock()
	if closed {
		return
	}

	// Wait for the drain sentinel to be processed, with a generous
	// timeout to prevent hanging if the dispatch goroutine's channel
	// is persistently full (PublishMustDeliver timeout).
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ack:
	case <-timer.C:
	}
}

// drainSentinel is a special event used by Drain to synchronize.
type drainSentinel struct {
	baseEvent
	ack chan struct{}
}
