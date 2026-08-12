package agui

import (
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

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
