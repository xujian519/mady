package acp

import (
	"github.com/xujian519/mady/agentcore"
)

// RegisterEventListeners attaches per-prompt event handlers to the agent and
// returns an unregister function that MUST be called when the prompt finishes.
// The agent is reused across prompts within a session; without unregister,
// handlers accumulate and each new prompt re-notifies through stale closures
// bound to the previous prompt's notify channel.
func RegisterEventListeners(sessionID string, core *agentcore.Agent, notify func(method string, params any)) (unregister func()) {
	unsubs := make([]func(), 0, 3)
	unsubs = append(unsubs,
		core.On(agentcore.EventToolCallStart, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.ToolCallStartEvent)
			if !ok {
				return
			}
			kind := ToolKind(ev.ToolCall.Name)
			args := parseToolArgs(ev.ToolCall.Arguments)
			title := BuildToolTitle(ev.ToolCall.Name, args)
			notify("session/update", SessionNotification{
				SessionID: sessionID,
				Update: SessionUpdate{
					SessionUpdate: "tool_call",
					ToolCallID:    ev.ToolCall.ID,
					Title:         title,
					Kind:          kind,
					Status:        "in_progress",
					RawInput:      args,
				},
			})
		}),
		core.On(agentcore.EventToolCallEnd, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.ToolCallEndEvent)
			if !ok {
				return
			}
			kind := ToolKind(ev.ToolName)
			status := "completed"
			if ev.Err != nil {
				status = "error"
			}
			notify("session/update", SessionNotification{
				SessionID: sessionID,
				Update: SessionUpdate{
					SessionUpdate: "tool_call_update",
					ToolCallID:    ev.ToolCallID,
					Title:         ev.ToolName,
					Kind:          kind,
					Status:        status,
					RawOutput:     ev.Result,
				},
			})
		}),
		core.On(agentcore.EventMessageDelta, func(e agentcore.Event) {
			ev, ok := e.(*agentcore.MessageDeltaEvent)
			if !ok {
				return
			}
			if ev.Kind == "thinking" {
				notify("session/update", SessionNotification{
					SessionID: sessionID,
					Update: SessionUpdate{
						SessionUpdate: "agent_thought_chunk",
						Content:       TextContentBlock{Type: "text", Text: ev.Delta},
					},
				})
			} else {
				notify("session/update", SessionNotification{
					SessionID: sessionID,
					Update: SessionUpdate{
						SessionUpdate: "agent_message_chunk",
						Content:       TextContentBlock{Type: "text", Text: ev.Delta},
					},
				})
			}
		}),
	)

	return func() {
		for _, u := range unsubs {
			u()
		}
	}
}
