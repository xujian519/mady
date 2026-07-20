package agentadapter

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

func BindAgent(sub chat.Subscriber, agent *agentcore.Agent) {
	adapter := &subscriberAdapter{agent: agent}
	sub.Subscribe(adapter)
}

type subscriberAdapter struct {
	agent *agentcore.Agent
}

func (s *subscriberAdapter) On(eventType chat.ChatEventType, handler func(chat.ChatEvent)) {
	switch eventType {
	case chat.ChatEventAgentStart:
		s.agent.On(agentcore.EventAgentStart, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.AgentStartEvent)
			if !ok {
				return
			}
			handler(chat.AgentStartChatEvent{AgentName: ev.AgentName, Input: ev.Input})
		})
	case chat.ChatEventAgentEnd:
		s.agent.On(agentcore.EventAgentEnd, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.AgentEndEvent)
			if !ok {
				return
			}
			handler(chat.AgentEndChatEvent{AgentName: ev.AgentName, Output: ev.Output})
		})
	case chat.ChatEventAgentError:
		s.agent.On(agentcore.EventAgentError, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.AgentErrorEvent)
			if !ok {
				return
			}
			handler(chat.AgentErrorChatEvent{Err: ev.Err})
		})
	case chat.ChatEventAgentInterrupt:
		s.agent.On(agentcore.EventAgentInterrupt, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.AgentInterruptEvent)
			if !ok {
				return
			}
			var reason string
			var data map[string]any
			if ev.Reason != nil {
				reason = ev.Reason.Reason
				data = ev.Reason.Data
			}
			handler(chat.AgentInterruptChatEvent{Reason: reason, Data: data})
		})
	case chat.ChatEventTurnStart:
		s.agent.On(agentcore.EventTurnStart, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.TurnStartEvent)
			if !ok {
				return
			}
			handler(chat.TurnStartChatEvent{Turn: ev.Turn})
		})
	case chat.ChatEventTurnEnd:
		s.agent.On(agentcore.EventTurnEnd, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.TurnEndEvent)
			if !ok {
				return
			}
			handler(chat.TurnEndChatEvent{Turn: ev.Turn, Usage: convertUsage(ev.Usage)})
		})
	case chat.ChatEventMessageDelta:
		s.agent.On(agentcore.EventMessageDelta, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.MessageDeltaEvent)
			if !ok {
				return
			}
			handler(chat.MessageDeltaChatEvent{Delta: ev.Delta, Kind: string(ev.Kind)})
		})
	case chat.ChatEventToolCallStart:
		s.agent.On(agentcore.EventToolCallStart, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.ToolCallStartEvent)
			if !ok {
				return
			}
			handler(chat.ToolCallStartChatEvent{ToolCall: chat.ToolCallInfo{
				ID: ev.ToolCall.ID, Name: ev.ToolCall.Name, Arguments: ev.ToolCall.Arguments,
			}})
		})
	case chat.ChatEventToolCallEnd:
		s.agent.On(agentcore.EventToolCallEnd, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.ToolCallEndEvent)
			if !ok {
				return
			}
			handler(chat.ToolCallEndChatEvent{
				ToolCallID: ev.ToolCallID, ToolName: ev.ToolName,
				Result: ev.Result, Err: ev.Err, Duration: ev.Duration,
			})
		})
	case chat.ChatEventHandoffStart:
		s.agent.On(agentcore.EventHandoffStart, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.HandoffStartEvent)
			if !ok {
				return
			}
			handler(chat.HandoffStartChatEvent{
				SourceAgent: ev.SourceAgent, TargetAgent: ev.TargetAgent,
				Mode: ev.Mode, Context: ev.Context,
				Invisible: ev.Invisible,
			})
		})
	case chat.ChatEventHandoffEnd:
		s.agent.On(agentcore.EventHandoffEnd, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.HandoffEndEvent)
			if !ok {
				return
			}
			handler(chat.HandoffEndChatEvent{
				TargetAgent: ev.TargetAgent, Output: ev.Output,
				Duration: ev.Duration, Err: ev.Err,
				Invisible: ev.Invisible,
			})
		})
	case chat.ChatEventCompactionStart:
		s.agent.On(agentcore.EventCompactionStart, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.CompactionStartEvent)
			if !ok {
				return
			}
			handler(chat.CompactionStartChatEvent{
				TokensBefore: ev.TokensBefore, ContextWindow: ev.ContextWindow,
			})
		})
	case chat.ChatEventCompactionEnd:
		s.agent.On(agentcore.EventCompactionEnd, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.CompactionEndEvent)
			if !ok {
				return
			}
			handler(chat.CompactionEndChatEvent{
				TokensBefore: ev.TokensBefore, TokensAfter: ev.TokensAfter,
				MessagesCut: ev.MessagesCut, Duration: ev.Duration,
			})
		})
	case chat.ChatEventAutoRetry:
		s.agent.On(agentcore.EventAutoRetry, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.AutoRetryEvent)
			if !ok {
				return
			}
			handler(chat.AutoRetryChatEvent{
				Attempt: ev.Attempt, MaxRetries: ev.MaxRetries,
				Delay: ev.Delay, Err: ev.Err,
			})
		})
	case chat.ChatEventApprovalPrompt:
		s.agent.On(agentcore.EventApprovalPrompt, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.ApprovalPromptEvent)
			if !ok {
				return
			}
			handler(chat.ApprovalPromptChatEvent{
				Content: ev.Content,
				Data:    parseReviewGateData(ev.Content, ev.Data),
			})
		})
	}
}

func convertUsage(u agentcore.TokenUsage) chat.TokenUsage {
	return chat.TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

// parseReviewGateData converts the unstructured map from agentcore into a
// typed ReviewGatePayload. This is the single parsing boundary — consumers
// in the chat package receive typed data and never parse map[string]any.
func parseReviewGateData(content string, data map[string]any) *chat.ReviewGatePayload {
	if data == nil {
		return nil
	}
	judgment, _ := data["judgment"].(string)
	if judgment == "" {
		return nil
	}
	title, _ := data["title"].(string)
	conf, _ := data["confidence"].(float64)

	payload := &chat.ReviewGatePayload{
		Title:      title,
		Judgment:   judgment,
		Confidence: conf,
	}

	// Parse evidences
	if evRaw, ok := data["evidences"].([]any); ok {
		for _, r := range evRaw {
			if m, ok := r.(map[string]any); ok {
				ev := component.ReviewEvidence{}
				if id, ok := m["id"].(string); ok {
					ev.ID = id
				}
				if t, ok := m["title"].(string); ok {
					ev.Title = t
				}
				if role, ok := m["role"].(string); ok {
					ev.Role = role
				}
				if s, ok := m["summary"].(string); ok {
					ev.Summary = s
				}
				if st, ok := m["status"].(float64); ok {
					ev.Status = component.EvidenceStatus(int(st))
				}
				payload.Evidences = append(payload.Evidences, ev)
			}
		}
	}

	// Parse checklist
	if clRaw, ok := data["checklist"].([]any); ok {
		for _, r := range clRaw {
			if m, ok := r.(map[string]any); ok {
				ci := component.ReviewCheckItem{}
				if l, ok := m["label"].(string); ok {
					ci.Label = l
				}
				if c, ok := m["checked"].(bool); ok {
					ci.Checked = c
				}
				payload.Checklist = append(payload.Checklist, ci)
			}
		}
	}

	// Parse risks
	if rRaw, ok := data["risks"].([]any); ok {
		for _, r := range rRaw {
			if s, ok := r.(string); ok {
				payload.Risks = append(payload.Risks, s)
			}
		}
	}

	return payload
}
