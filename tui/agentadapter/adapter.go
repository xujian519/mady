// Package agentadapter bridges agentcore event streams to the TUI chat layer,
// converting 20 agent event types into ChatEvent messages.
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

// eventToChat 按 agentcore 事件类型转换（映射表驱动，取代大型 switch）。
// 转换函数各自断言具体事件类型，断言失败返回 nil（不会发生：映射表
// 已按类型一一对应）。
func (s *subscriberAdapter) eventToChat(e agentcore.Event) chat.ChatEvent {
	switch ev := e.(type) {
	case *agentcore.AgentStartEvent:
		return chat.AgentStartChatEvent{AgentName: ev.AgentName, Input: ev.Input}
	case *agentcore.AgentEndEvent:
		return chat.AgentEndChatEvent{AgentName: ev.AgentName, Output: ev.Output, FinishReason: ev.FinishReason}
	case *agentcore.AgentErrorEvent:
		return chat.AgentErrorChatEvent{Err: ev.Err}
	case *agentcore.AgentInterruptEvent:
		var reason string
		var data map[string]any
		if ev.Reason != nil {
			reason = ev.Reason.Reason
			data = ev.Reason.Data
		}
		return chat.AgentInterruptChatEvent{Reason: reason, Data: data}
	case *agentcore.TurnStartEvent:
		return chat.TurnStartChatEvent{Turn: ev.Turn}
	case *agentcore.TurnEndEvent:
		return chat.TurnEndChatEvent{Turn: ev.Turn, Usage: convertUsage(ev.Usage)}
	case *agentcore.MessageDeltaEvent:
		return chat.MessageDeltaChatEvent{Delta: ev.Delta, Kind: string(ev.Kind)}
	case *agentcore.ToolCallStartEvent:
		return chat.ToolCallStartChatEvent{ToolCall: chat.ToolCallInfo{
			ID: ev.ToolCall.ID, Name: ev.ToolCall.Name, Arguments: ev.ToolCall.Arguments,
		}}
	case *agentcore.ToolCallEndEvent:
		return chat.ToolCallEndChatEvent{
			ToolCallID: ev.ToolCallID, ToolName: ev.ToolName,
			Result: ev.Result, Err: ev.Err, Duration: ev.Duration,
		}
	case *agentcore.HandoffStartEvent:
		return chat.HandoffStartChatEvent{
			SourceAgent: ev.SourceAgent, TargetAgent: ev.TargetAgent,
			Mode: ev.Mode, Context: ev.Context,
			Invisible: ev.Invisible,
		}
	case *agentcore.HandoffEndEvent:
		return chat.HandoffEndChatEvent{
			TargetAgent: ev.TargetAgent, Output: ev.Output,
			Duration: ev.Duration, Err: ev.Err,
			Invisible: ev.Invisible,
		}
	case *agentcore.CompactionStartEvent:
		return chat.CompactionStartChatEvent{
			TokensBefore: ev.TokensBefore, ContextWindow: ev.ContextWindow,
		}
	case *agentcore.CompactionEndEvent:
		return chat.CompactionEndChatEvent{
			TokensBefore: ev.TokensBefore, TokensAfter: ev.TokensAfter,
			MessagesCut: ev.MessagesCut, Duration: ev.Duration,
		}
	case *agentcore.AutoRetryEvent:
		return chat.AutoRetryChatEvent{
			Attempt: ev.Attempt, MaxRetries: ev.MaxRetries,
			Delay: ev.Delay, Err: ev.Err,
		}
	case *agentcore.ApprovalPromptEvent:
		return chat.ApprovalPromptChatEvent{
			Content: ev.Content,
			Data:    parseReviewGateData(ev.Data),
		}
	case *agentcore.TaskCreatedEvent:
		return chat.TaskCreatedChatEvent{Task: agentTaskToInfo(ev.Task)}
	case *agentcore.TaskUpdatedEvent:
		return chat.TaskUpdatedChatEvent{Task: agentTaskToInfo(ev.Task)}
	case *agentcore.PlanTaskStatusChangedEvent:
		return chat.PlanTaskStatusChangedChatEvent{
			SessionID:  ev.SessionID,
			CaseID:     ev.CaseID,
			FromStatus: ev.FromStatus,
			ToStatus:   ev.ToStatus,
		}
	case *agentcore.PlanTaskFeedbackAddedEvent:
		return chat.PlanTaskFeedbackAddedChatEvent{
			SessionID: ev.SessionID,
			Text:      ev.Text,
			StepID:    ev.StepID,
		}
	case *agentcore.PlanTaskInterruptedEvent:
		return chat.PlanTaskInterruptedChatEvent{
			SessionID: ev.SessionID,
			StepID:    ev.StepID,
			Reason:    ev.Reason,
		}
	}
	return nil
}

func (s *subscriberAdapter) On(eventType chat.ChatEventType, handler func(chat.ChatEvent)) {
	// 事件映射表驱动：chat 事件类型 → agentcore 事件类型。
	evType, ok := chatToAgentEvent[eventType]
	if !ok {
		return
	}
	s.agent.On(evType, func(e agentcore.Event) {
		if conv := s.eventToChat(e); conv != nil {
			handler(conv)
		}
	})
}

// chatToAgentEvent 建立 chat 事件类型到 agentcore 事件类型的双向映射。
// eventToChat 按具体类型断言转换，此处只需确定订阅的事件类型。
var chatToAgentEvent = map[chat.ChatEventType]agentcore.EventType{
	chat.ChatEventAgentStart:            agentcore.EventAgentStart,
	chat.ChatEventAgentEnd:              agentcore.EventAgentEnd,
	chat.ChatEventAgentError:            agentcore.EventAgentError,
	chat.ChatEventAgentInterrupt:        agentcore.EventAgentInterrupt,
	chat.ChatEventTurnStart:             agentcore.EventTurnStart,
	chat.ChatEventTurnEnd:               agentcore.EventTurnEnd,
	chat.ChatEventMessageDelta:          agentcore.EventMessageDelta,
	chat.ChatEventToolCallStart:         agentcore.EventToolCallStart,
	chat.ChatEventToolCallEnd:           agentcore.EventToolCallEnd,
	chat.ChatEventHandoffStart:          agentcore.EventHandoffStart,
	chat.ChatEventHandoffEnd:            agentcore.EventHandoffEnd,
	chat.ChatEventCompactionStart:       agentcore.EventCompactionStart,
	chat.ChatEventCompactionEnd:         agentcore.EventCompactionEnd,
	chat.ChatEventAutoRetry:             agentcore.EventAutoRetry,
	chat.ChatEventApprovalPrompt:        agentcore.EventApprovalPrompt,
	chat.ChatEventTaskCreated:           agentcore.EventTaskCreated,
	chat.ChatEventTaskUpdated:           agentcore.EventTaskUpdated,
	chat.ChatEventPlanTaskStatusChanged: agentcore.EventPlanTaskStatusChanged,
	chat.ChatEventPlanTaskFeedbackAdded: agentcore.EventPlanTaskFeedbackAdded,
	chat.ChatEventPlanTaskInterrupted:   agentcore.EventPlanTaskInterrupted,
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
func parseReviewGateData(data map[string]any) *chat.ReviewGatePayload { //nolint:gocognit // 渲染/分发/状态机复杂分支，拆分列入 P3
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

// agentTaskToInfo converts an agentcore.Task to a chat.TaskInfo,
// maintaining the architectural boundary that tui/chat must not import agentcore.
func agentTaskToInfo(t *agentcore.Task) *chat.TaskInfo {
	if t == nil {
		return nil
	}
	return &chat.TaskInfo{
		ID:       t.ID,
		Subject:  t.Subject,
		Status:   string(t.Status),
		Priority: string(t.Priority),
	}
}
