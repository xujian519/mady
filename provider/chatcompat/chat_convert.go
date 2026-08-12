package chatcompat

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// --- type conversion helpers ---

// ToMessages converts agentcore messages to Chat Completions wire format.
//
//nolint:revive // unexported-return: chatMessage is internal
func ToMessages(msgs []agentcore.Message) []chatMessage {
	out := make([]chatMessage, len(msgs))
	for i, m := range msgs {
		cm := chatMessage{
			Role:       string(m.Role),
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		cm.Content = MessageContent(m)
		if m.Role == agentcore.RoleAssistant {
			var reasoningParts []string
			for _, bl := range m.Blocks {
				if bl.Kind == agentcore.BlockKindThinking && bl.Text != "" {
					reasoningParts = append(reasoningParts, bl.Text)
				}
			}
			if len(reasoningParts) > 0 {
				cm.ReasoningContent = strings.Join(reasoningParts, "\n")
			}
		}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: chatFunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		out[i] = cm
	}
	return out
}

// MessageContent converts a message's content into the Chat Completions wire
// format, handling multi-block content with text and image parts.
func MessageContent(m agentcore.Message) any {
	if len(m.Blocks) == 0 {
		return m.Content
	}
	parts := make([]chatContentPart, 0, len(m.Blocks)+1)
	if m.Content != "" {
		parts = append(parts, chatContentPart{Type: "text", Text: m.Content})
	}
	for _, bl := range m.Blocks {
		switch bl.Kind {
		case agentcore.BlockKindText:
			if bl.Text != "" {
				parts = append(parts, chatContentPart{Type: "text", Text: bl.Text})
			}
		case agentcore.BlockKindImage:
			if bl.URL == "" {
				continue
			}
			parts = append(parts, chatContentPart{
				Type: "image_url",
				ImageURL: &chatImageURL{
					URL:    bl.URL,
					Detail: bl.Detail,
				},
			})
		}
	}
	if len(parts) == 0 {
		return m.Content
	}
	return parts
}

// ToTools converts agentcore tool definitions to Chat Completions wire format.
//
//nolint:revive // unexported-return: chatTool is internal
func ToTools(defs []agentcore.ToolDefinition) []chatTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]chatTool, len(defs))
	for i, d := range defs {
		out[i] = chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  d.Parameters,
			},
		}
	}
	return out
}
