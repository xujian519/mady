package agui

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// Converter translates agentcore events into AGUI protocol events.
type Converter struct {
	threadID            string
	runID               string
	parentRunID         string
	msgSeq              atomic.Int64
	thinkingSeq         atomic.Int64
	activeMsgID         atomic.Value
	activeMsgRole       atomic.Value
	activeThinkingID    atomic.Value
	activeThinkingMsgID atomic.Value
	contextWindow       int64
	totalTokens         int64
}

// NewConverter 创建 AGUI 事件转换器。contextWindow 为模型上下文窗口大小（token），
// 设为 0 时默认使用 128K（用于缓存未配置场景）。
func NewConverter(threadID, runID string, contextWindow ...int64) *Converter {
	cw := int64(128000)
	if len(contextWindow) > 0 && contextWindow[0] > 0 {
		cw = contextWindow[0]
	}
	c := &Converter{
		threadID:      threadID,
		runID:         runID,
		contextWindow: cw,
	}
	c.activeMsgID.Store("")
	c.activeMsgRole.Store("")
	c.activeThinkingID.Store("")
	c.activeThinkingMsgID.Store("")
	return c
}

// NewConverterWithParent 创建带父 run ID 的转换器。
func NewConverterWithParent(threadID, runID, parentRunID string, contextWindow ...int64) *Converter {
	cw := int64(128000)
	if len(contextWindow) > 0 && contextWindow[0] > 0 {
		cw = contextWindow[0]
	}
	c := &Converter{
		threadID:      threadID,
		runID:         runID,
		parentRunID:   parentRunID,
		contextWindow: cw,
	}
	c.activeMsgID.Store("")
	c.activeMsgRole.Store("")
	c.activeThinkingID.Store("")
	c.activeThinkingMsgID.Store("")
	return c
}

func (c *Converter) nextMsgID() string {
	return fmt.Sprintf("msg_%d", c.msgSeq.Add(1))
}

func (c *Converter) nextThinkingID() string {
	return fmt.Sprintf("thinking_%d", c.thinkingSeq.Add(1))
}

func tsNano(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e6
}

func baseEvent(typ EventType, t time.Time) BaseEvent {
	return BaseEvent{
		Type:      typ,
		Timestamp: tsNano(t),
	}
}

// RunStarted creates a RunStartedEvent for the current run.
func (c *Converter) RunStarted(t time.Time) RunStartedEvent {
	return RunStartedEvent{
		BaseEvent:   baseEvent(EventRunStarted, t),
		ThreadID:    c.threadID,
		RunID:       c.runID,
		ParentRunID: c.parentRunID,
	}
}

// RunFinished creates a RunFinishedEvent for a normal completion.
func (c *Converter) RunFinished(t time.Time) RunFinishedEvent {
	return RunFinishedEvent{
		BaseEvent: baseEvent(EventRunFinished, t),
		ThreadID:  c.threadID,
		RunID:     c.runID,
	}
}

// RunFinishedWithInterrupts creates a RunFinishedEvent with interrupt outcomes.
func (c *Converter) RunFinishedWithInterrupts(t time.Time, interrupts []Interrupt) RunFinishedEvent {
	return RunFinishedEvent{
		BaseEvent: baseEvent(EventRunFinished, t),
		ThreadID:  c.threadID,
		RunID:     c.runID,
		Outcome: &RunFinishedOutcome{
			Type:       "interrupt",
			Interrupts: interrupts,
		},
	}
}

// RunFinishedWithSuccess creates a RunFinishedEvent with a successful outcome.
// finishReason 为模型结束原因（"stop"/"length"/"error" 等），写入事件供
// 前端判断输出是否可能不完整。
func (c *Converter) RunFinishedWithSuccess(t time.Time, result any, finishReason string) RunFinishedEvent {
	return RunFinishedEvent{
		BaseEvent:    baseEvent(EventRunFinished, t),
		ThreadID:     c.threadID,
		RunID:        c.runID,
		Result:       result,
		Outcome:      &RunFinishedOutcome{Type: "success"},
		FinishReason: finishReason,
	}
}

// RunError creates a RunErrorEvent for an error.
func (c *Converter) RunError(t time.Time, err error) RunErrorEvent {
	msg := "unknown error"
	if err != nil {
		msg = err.Error()
	}
	return RunErrorEvent{
		BaseEvent: baseEvent(EventRunError, t),
		ThreadID:  c.threadID,
		RunID:     c.runID,
		Message:   msg,
	}
}

// CloseMessage closes the active text message and returns the end event.
func (c *Converter) CloseMessage(t time.Time) []any {
	prevID, _ := c.activeMsgID.Load().(string)
	if prevID == "" {
		return nil
	}
	c.activeMsgID.Store("")
	c.activeMsgRole.Store("")
	return []any{TextMessageEndEvent{
		BaseEvent: baseEvent(EventTextMessageEnd, t),
		MessageID: prevID,
	}}
}

// CloseThinking closes the active thinking block and returns the end events.
func (c *Converter) CloseThinking(t time.Time) []any {
	prevID, _ := c.activeThinkingID.Load().(string)
	if prevID == "" {
		return nil
	}
	msgID, _ := c.activeThinkingMsgID.Load().(string)
	c.activeThinkingID.Store("")
	c.activeThinkingMsgID.Store("")
	var events []any
	if msgID != "" {
		events = append(events, ThinkingTextMessageEndEvent{
			BaseEvent:  baseEvent(EventThinkingTextMessageEnd, t),
			ThinkingID: prevID,
			MessageID:  msgID,
		})
	}
	events = append(events, ThinkingEndEvent{
		BaseEvent:  baseEvent(EventThinkingEnd, t),
		ThinkingID: prevID,
	})
	return events
}

func (c *Converter) closeAll(t time.Time) []any {
	events := make([]any, 0, 2)
	if tail := c.CloseMessage(t); tail != nil {
		events = append(events, tail...)
	}
	if tail := c.CloseThinking(t); tail != nil {
		events = append(events, tail...)
	}
	return events
}

// StateSnapshot creates a StateSnapshotEvent with the given state.
func (c *Converter) StateSnapshot(t time.Time, state any) StateSnapshotEvent {
	return StateSnapshotEvent{
		BaseEvent: baseEvent(EventStateSnapshot, t),
		Snapshot:  state,
	}
}

// StateDelta creates a StateDeltaEvent with JSON patch operations.
func (c *Converter) StateDelta(t time.Time, ops []jsonPatchOp) StateDeltaEvent {
	return StateDeltaEvent{
		BaseEvent: baseEvent(EventStateDelta, t),
		Delta:     ops,
	}
}

// accumulateUsage 累加本轮 Token 用量到 totalTokens。
func (c *Converter) accumulateUsage(usage agentcore.TokenUsage, _ time.Time) {
	c.totalTokens += usage.TotalTokens
}

// buildContextUsage 构造 CONTEXT_USAGE 事件。
func (c *Converter) buildContextUsage(t time.Time) ContextUsageEvent {
	percent := float64(0)
	if c.contextWindow > 0 {
		percent = float64(c.totalTokens) / float64(c.contextWindow) * 100
	}
	return ContextUsageEvent{
		BaseEvent:     baseEvent(EventContextUsage, t),
		ContextWindow: c.contextWindow,
		UsagePercent:  percent,
		TokenUsage: struct {
			PromptTokens     int64 `json:"promptTokens"`
			CompletionTokens int64 `json:"completionTokens"`
			TotalTokens      int64 `json:"totalTokens"`
		}{
			TotalTokens: c.totalTokens,
		},
	}
}

// Convert translates an agentcore event into one or more AGUI events.
func (c *Converter) Convert(e agentcore.Event) []any {
	switch ev := e.(type) {
	case agentcore.AgentStartEvent:
		return []any{c.RunStarted(ev.EventTime())}
	case *agentcore.AgentStartEvent:
		return []any{c.RunStarted(ev.EventTime())}

	case agentcore.AgentEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.RunFinishedWithSuccess(ev.EventTime(), ev.Output, ev.FinishReason))
		return events
	case *agentcore.AgentEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.RunFinishedWithSuccess(ev.EventTime(), ev.Output, ev.FinishReason))
		return events

	case agentcore.AgentErrorEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.RunError(ev.EventTime(), ev.Err))
		return events
	case *agentcore.AgentErrorEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.RunError(ev.EventTime(), ev.Err))
		return events

	case agentcore.TurnStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, StepStartedEvent{
			BaseEvent: baseEvent(EventStepStarted, ev.EventTime()),
			StepName:  fmt.Sprintf("turn_%d", ev.Turn),
		})
		return events
	case *agentcore.TurnStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, StepStartedEvent{
			BaseEvent: baseEvent(EventStepStarted, ev.EventTime()),
			StepName:  fmt.Sprintf("turn_%d", ev.Turn),
		})
		return events

	case agentcore.TurnEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+2)
		events = append(events, closeEvents...)
		events = append(events, StepFinishedEvent{
			BaseEvent: baseEvent(EventStepFinished, ev.EventTime()),
			StepName:  fmt.Sprintf("turn_%d", ev.Turn),
		})
		c.accumulateUsage(ev.Usage, ev.EventTime())
		if c.totalTokens > 0 {
			events = append(events, c.buildContextUsage(ev.EventTime()))
		}
		return events
	case *agentcore.TurnEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+2)
		events = append(events, closeEvents...)
		events = append(events, StepFinishedEvent{
			BaseEvent: baseEvent(EventStepFinished, ev.EventTime()),
			StepName:  fmt.Sprintf("turn_%d", ev.Turn),
		})
		c.accumulateUsage(ev.Usage, ev.EventTime())
		if c.totalTokens > 0 {
			events = append(events, c.buildContextUsage(ev.EventTime()))
		}
		return events

	case agentcore.MessageDeltaEvent:
		return c.convertMessageDelta(ev.EventTime(), ev.Delta, ev.Kind)
	case *agentcore.MessageDeltaEvent:
		return c.convertMessageDelta(ev.EventTime(), ev.Delta, ev.Kind)

	case agentcore.ToolCallStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+2)
		events = append(events, closeEvents...)
		events = append(events, c.convertToolCallStart(ev.EventTime(), ev.ToolCall)...)
		return events
	case *agentcore.ToolCallStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+2)
		events = append(events, closeEvents...)
		events = append(events, c.convertToolCallStart(ev.EventTime(), ev.ToolCall)...)
		return events

	case agentcore.ToolCallEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+2)
		events = append(events, closeEvents...)
		events = append(events, c.convertToolCallEnd(ev.EventTime(), ev.ToolCallID, ev.ToolName, ev.Result, ev.Err)...)
		return events
	case *agentcore.ToolCallEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+2)
		events = append(events, closeEvents...)
		events = append(events, c.convertToolCallEnd(ev.EventTime(), ev.ToolCallID, ev.ToolName, ev.Result, ev.Err)...)
		return events

	case agentcore.HandoffStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertHandoffStart(ev)...)
		return events
	case *agentcore.HandoffStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertHandoffStart(*ev)...)
		return events

	case agentcore.HandoffEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertHandoffEnd(ev)...)
		return events
	case *agentcore.HandoffEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertHandoffEnd(*ev)...)
		return events

	case agentcore.CompactionStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertCompactionStart(ev)...)
		return events
	case *agentcore.CompactionStartEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertCompactionStart(*ev)...)
		return events

	case agentcore.CompactionEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertCompactionEnd(ev)...)
		return events
	case *agentcore.CompactionEndEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertCompactionEnd(*ev)...)
		return events

	case agentcore.AutoRetryEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertAutoRetry(ev)...)
		return events
	case *agentcore.AutoRetryEvent:
		closeEvents := c.closeAll(ev.EventTime())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, c.convertAutoRetry(*ev)...)
		return events

	case *agentcore.A2UIEvent:
		return []any{CustomEvent{
			BaseEvent: baseEvent(EventCustom, ev.EventTime()),
			Name:      "a2ui",
			Value:     ev.Envelope,
		}}

	default:
		closeEvents := c.closeAll(time.Now())
		events := make([]any, 0, len(closeEvents)+1)
		events = append(events, closeEvents...)
		events = append(events, CustomEvent{
			BaseEvent: baseEvent(EventCustom, time.Now()),
			Name:      string(e.EventKind()),
			Value:     e,
		})
		return events
	}
}

func (c *Converter) convertMessageDelta(t time.Time, delta string, kind agentcore.BlockKind) []any {
	if kind == agentcore.BlockKindThinking {
		return c.convertThinkingDelta(t, delta)
	}
	var events []any
	if tail := c.CloseThinking(t); tail != nil {
		events = append(events, tail...)
	}
	prevID, _ := c.activeMsgID.Load().(string)
	if prevID == "" {
		msgID := c.nextMsgID()
		c.activeMsgID.Store(msgID)
		c.activeMsgRole.Store("assistant")
		events = append(events,
			TextMessageStartEvent{
				BaseEvent: baseEvent(EventTextMessageStart, t),
				MessageID: msgID,
				Role:      "assistant",
			},
			TextMessageContentEvent{
				BaseEvent: baseEvent(EventTextMessageContent, t),
				MessageID: msgID,
				Delta:     delta,
			},
		)
		return events
	}
	events = append(events, TextMessageContentEvent{
		BaseEvent: baseEvent(EventTextMessageContent, t),
		MessageID: prevID,
		Delta:     delta,
	})
	return events
}

func (c *Converter) convertThinkingDelta(t time.Time, delta string) []any {
	prevID, _ := c.activeThinkingID.Load().(string)
	if prevID == "" {
		thinkingID := c.nextThinkingID()
		msgID := c.nextMsgID()
		c.activeThinkingID.Store(thinkingID)
		c.activeThinkingMsgID.Store(msgID)
		return []any{
			ThinkingStartEvent{
				BaseEvent:  baseEvent(EventThinkingStart, t),
				ThinkingID: thinkingID,
			},
			ThinkingTextMessageStartEvent{
				BaseEvent:  baseEvent(EventThinkingTextMessageStart, t),
				ThinkingID: thinkingID,
				MessageID:  msgID,
			},
			ThinkingTextMessageContentEvent{
				BaseEvent:  baseEvent(EventThinkingTextMessageContent, t),
				ThinkingID: thinkingID,
				MessageID:  msgID,
				Delta:      delta,
			},
		}
	}
	msgID, _ := c.activeThinkingMsgID.Load().(string)
	if msgID == "" {
		msgID = c.nextMsgID()
	}
	return []any{ThinkingTextMessageContentEvent{
		BaseEvent:  baseEvent(EventThinkingTextMessageContent, t),
		ThinkingID: prevID,
		MessageID:  msgID,
		Delta:      delta,
	}}
}

func (c *Converter) convertToolCallStart(t time.Time, tc agentcore.ToolCall) []any {
	return []any{
		ToolCallStartEvent{
			BaseEvent:    baseEvent(EventToolCallStart, t),
			ToolCallID:   tc.ID,
			ToolCallName: tc.Name,
		},
		ToolCallArgsEvent{
			BaseEvent:  baseEvent(EventToolCallArgs, t),
			ToolCallID: tc.ID,
			Delta:      tc.Arguments,
		},
	}
}

func (c *Converter) convertToolCallEnd(t time.Time, toolCallID, _ string, result string, err error) []any {
	events := make([]any, 0, 2)
	events = append(events, ToolCallEndEvent{
		BaseEvent:  baseEvent(EventToolCallEnd, t),
		ToolCallID: toolCallID,
	})
	if err != nil {
		result = err.Error()
	}
	events = append(events, ToolCallResultEvent{
		BaseEvent:  baseEvent(EventToolCallResult, t),
		MessageID:  fmt.Sprintf("tool_result_%s", toolCallID),
		ToolCallID: toolCallID,
		Content:    result,
		Role:       "tool",
	})
	return events
}

// convertHandoffStart converts a HandoffStartEvent (value or dereferenced pointer)
// into a CustomEvent with handoff metadata.
func (c *Converter) convertHandoffStart(ev agentcore.HandoffStartEvent) []any {
	return []any{
		CustomEvent{
			BaseEvent: baseEvent(EventCustom, ev.EventTime()),
			Name:      "handoff_start",
			Value: map[string]any{
				"source_agent": ev.SourceAgent,
				"target_agent": ev.TargetAgent,
				"mode":         ev.Mode,
				"context":      ev.Context,
			},
		},
	}
}

// convertHandoffEnd converts a HandoffEndEvent into a CustomEvent with result metadata.
func (c *Converter) convertHandoffEnd(ev agentcore.HandoffEndEvent) []any {
	return []any{
		CustomEvent{
			BaseEvent: baseEvent(EventCustom, ev.EventTime()),
			Name:      "handoff_end",
			Value: map[string]any{
				"target_agent": ev.TargetAgent,
				"output":       ev.Output,
				"duration_ms":  ev.Duration.Milliseconds(),
			},
		},
	}
}

// convertCompactionStart converts a CompactionStartEvent into a CustomEvent.
func (c *Converter) convertCompactionStart(ev agentcore.CompactionStartEvent) []any {
	return []any{
		CustomEvent{
			BaseEvent: baseEvent(EventCustom, ev.EventTime()),
			Name:      "compaction_start",
			Value: map[string]any{
				"tokens_before":  ev.TokensBefore,
				"context_window": ev.ContextWindow,
			},
		},
	}
}

// convertCompactionEnd converts a CompactionEndEvent into a CustomEvent.
func (c *Converter) convertCompactionEnd(ev agentcore.CompactionEndEvent) []any {
	return []any{
		CustomEvent{
			BaseEvent: baseEvent(EventCustom, ev.EventTime()),
			Name:      "compaction_end",
			Value: map[string]any{
				"tokens_before": ev.TokensBefore,
				"tokens_after":  ev.TokensAfter,
				"messages_cut":  ev.MessagesCut,
				"duration_ms":   ev.Duration.Milliseconds(),
			},
		},
	}
}

// convertAutoRetry converts an AutoRetryEvent into a CustomEvent.
func (c *Converter) convertAutoRetry(ev agentcore.AutoRetryEvent) []any {
	return []any{
		CustomEvent{
			BaseEvent: baseEvent(EventCustom, ev.EventTime()),
			Name:      "auto_retry",
			Value: map[string]any{
				"attempt":     ev.Attempt,
				"max_retries": ev.MaxRetries,
				"delay_ms":    ev.Delay.Milliseconds(),
			},
		},
	}
}

// MessagesFromAgent converts agentcore messages to AGUI messages.
func MessagesFromAgent(msgs []agentcore.Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		agMsg := Message{
			ID:      m.ID,
			Role:    convertRole(m.Role),
			Content: m.Content,
			Name:    m.Name,
		}
		if m.ToolCallID != "" {
			agMsg.ToolCallID = m.ToolCallID
		}
		for _, tc := range m.ToolCalls {
			agMsg.ToolCalls = append(agMsg.ToolCalls, ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: ToolCallFunc{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		out = append(out, agMsg)
	}
	return out
}

func convertRole(r agentcore.Role) MessageRole {
	switch r {
	case agentcore.RoleUser:
		return MessageRoleUser
	case agentcore.RoleAssistant:
		return MessageRoleAssistant
	case agentcore.RoleSystem:
		return MessageRoleSystem
	case agentcore.RoleTool:
		return MessageRoleTool
	default:
		return MessageRoleDeveloper
	}
}

// CapabilitiesFromConfig builds AgentCapabilities from an agentcore config.
func CapabilitiesFromConfig(cfg agentcore.Config) AgentCapabilities {
	caps := AgentCapabilities{
		Identity: &IdentityCapabilities{
			Name:        cfg.Name,
			Type:        "mady",
			Description: cfg.Name,
		},
		Transport: &TransportCapabilities{
			Streaming: true,
		},
		Tools: &ToolsCapabilities{
			Supported:      len(cfg.Tools) > 0,
			ParallelCalls:  cfg.Concurrency > 1,
			ClientProvided: false,
		},
		State: &StateCapabilities{
			Snapshots:       true,
			Deltas:          false, // 当前仅支持全量快照，不支持 JSON Patch 增量
			PersistentState: cfg.Store != nil,
		},
		MultiAgent: &MultiAgentCapabilities{
			Supported:  len(cfg.Handoffs) > 0,
			Delegation: len(cfg.Handoffs) > 0,
			Handoffs:   len(cfg.Handoffs) > 0,
		},
		Execution: &ExecutionCapabilities{
			MaxIterations: cfg.MaxTurns,
		},
		HumanInTheLoop: &HumanInTheLoopCapabilities{
			Supported:  true,
			Approvals:  true,
			Interrupts: true,
		},
	}

	if len(cfg.Tools) > 0 {
		items := make([]ToolDef, 0, len(cfg.Tools))
		for _, t := range cfg.Tools {
			def := t.Definition()
			items = append(items, ToolDef{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			})
		}
		caps.Tools.Items = items
	}

	if len(cfg.Handoffs) > 0 {
		subs := make([]SubAgentDescriptor, 0, len(cfg.Handoffs))
		for _, h := range cfg.Handoffs {
			subs = append(subs, SubAgentDescriptor{
				Name:        h.Name,
				Description: h.Description,
			})
		}
		caps.MultiAgent.SubAgents = subs
	}

	if cfg.Thinking != nil {
		caps.Reasoning = &ReasoningCapabilities{
			Supported: true,
			Streaming: cfg.Streaming,
		}
	}

	return caps
}
