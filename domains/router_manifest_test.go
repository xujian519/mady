package domains

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestRouterConfigFromManifests_Empty(t *testing.T) {
	// Empty manifests should return the default RouterConfig
	base := agentcore.Config{}
	cfg := RouterConfigFromManifests(base, nil)

	if cfg.Name != "mady-router" {
		t.Errorf("expected name 'mady-router', got %q", cfg.Name)
	}
	if len(cfg.Handoffs) == 0 {
		t.Error("expected non-empty Handoffs from default RouterConfig")
	}
}

func TestRouterConfigFromManifests_AllDomains(t *testing.T) {
	manifests := []agentcore.AgentManifest{
		{
			Name:        "chat-agent",
			Domain:      DomainChat,
			Description: "日常聊天",
		},
		{
			Name:        "assistant-agent",
			Domain:      DomainAssistant,
			Description: "通用助理",
		},
		{
			Name:            "patent-agent",
			Domain:          DomainPatent,
			Description:     "专利分析",
			GuardrailLevel:  "strict",
			KnowledgeDomain: "patent",
		},
		{
			Name:            "legal-advisor",
			Domain:          DomainLegal,
			Description:     "法律咨询",
			GuardrailLevel:  "strict",
			KnowledgeDomain: "legal",
		},
	}

	base := agentcore.Config{}
	cfg := RouterConfigFromManifests(base, manifests)

	if cfg.Name != "mady-router" {
		t.Errorf("expected name 'mady-router', got %q", cfg.Name)
	}
	if len(cfg.Handoffs) != 4 {
		t.Fatalf("expected 4 handoffs, got %d", len(cfg.Handoffs))
	}

	// Verify each handoff target exists
	handoffNames := make(map[string]agentcore.HandoffConfig)
	for _, h := range cfg.Handoffs {
		handoffNames[h.Name] = h
	}

	expectedNames := []string{"chat-agent", "assistant-agent", "patent-agent", "legal-advisor"}
	for _, name := range expectedNames {
		if _, ok := handoffNames[name]; !ok {
			t.Errorf("missing handoff target: %s", name)
		}
	}

	// Verify descriptions propagate
	if h := handoffNames["chat-agent"]; h.Description != "日常聊天" {
		t.Errorf("expected description '日常聊天', got %q", h.Description)
	}
}

func TestRouterConfigFromManifests_UnknownDomainSkipped(t *testing.T) {
	manifests := []agentcore.AgentManifest{
		{
			Name:   "chat-agent",
			Domain: DomainChat,
		},
		{
			Name:   "unknown-agent",
			Domain: "unknown", // not in factoryMap
		},
	}

	base := agentcore.Config{}
	cfg := RouterConfigFromManifests(base, manifests)

	// Should only have the valid domain
	if len(cfg.Handoffs) != 1 {
		t.Fatalf("expected 1 handoff, got %d", len(cfg.Handoffs))
	}
	if cfg.Handoffs[0].Name != "chat-agent" {
		t.Errorf("expected handoff 'chat-agent', got %q", cfg.Handoffs[0].Name)
	}
}

func TestRouterConfigFromManifests_SystemPrompt(t *testing.T) {
	manifests := []agentcore.AgentManifest{
		{
			Name:        "chat-agent",
			Domain:      DomainChat,
			Description: "日常聊天",
		},
		{
			Name:        "patent-agent",
			Domain:      DomainPatent,
			Description: "专利分析",
		},
	}

	base := agentcore.Config{}
	cfg := RouterConfigFromManifests(base, manifests)

	// System prompt should mention both agents
	if !contains(cfg.SystemPrompt, "chat-agent") {
		t.Error("system prompt should mention chat-agent")
	}
	if !contains(cfg.SystemPrompt, "patent-agent") {
		t.Error("system prompt should mention patent-agent")
	}
	if !contains(cfg.SystemPrompt, "专利分析") {
		t.Error("system prompt should contain description '专利分析'")
	}
}

func TestRouterConfigFromManifests_HandoffConfig(t *testing.T) {
	manifests := []agentcore.AgentManifest{
		{Name: "chat-agent", Domain: DomainChat, Description: "聊天测试"},
	}

	base := agentcore.Config{}
	cfg := RouterConfigFromManifests(base, manifests)

	h := cfg.Handoffs[0]
	if h.Mode != agentcore.HandoffDelegate {
		t.Errorf("expected HandoffDelegate mode, got %v", h.Mode)
	}
	if len(h.AllowedSources) != 1 || h.AllowedSources[0] != "*" {
		t.Errorf("expected AllowedSources [\"*\"], got %v", h.AllowedSources)
	}
	if h.FallbackMsg == "" {
		t.Error("FallbackMsg should not be empty")
	}
	if h.AgentConfig.Name != "chat-agent" {
		t.Errorf("expected AgentConfig.Name 'chat-agent', got %q", h.AgentConfig.Name)
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
