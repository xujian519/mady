package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/tui/chat"
)

func parsePathListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(filepath.ListSeparator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func parseListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func defaultModel() string {
	switch util.EnvOrDefault("PROVIDER", "deepseek") {
	case "deepseek":
		return "deepseek-chat"
	case "zhipu":
		return "glm-5.2"
	case "kimi":
		return "kimi-k2-0905-preview"
	default:
		return os.Getenv("MODEL")
	}
}

// --- tool definitions ---

func storeHasKey(ctx context.Context, store agentcore.Store, key string) (bool, error) {
	return store.Has(ctx, key)
}

func chatMessageFromAgentMessage(msg agentcore.Message) chat.ChatMessage {
	role := chat.RoleSystem
	switch msg.Role {
	case agentcore.RoleUser:
		role = chat.RoleUser
	case agentcore.RoleAssistant:
		role = chat.RoleAssistant
	case agentcore.RoleTool:
		role = chat.RoleTool
	case agentcore.RoleSystem:
		role = chat.RoleSystem
	}
	text := msg.Content
	if text == "" && len(msg.ToolCalls) > 0 {
		text = fmt.Sprintf("tool calls: %d", len(msg.ToolCalls))
	}
	return chat.ChatMessage{
		Role: role,
		Text: text,
	}
}
