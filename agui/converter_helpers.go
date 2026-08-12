package agui

import "github.com/xujian519/mady/agentcore"

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
