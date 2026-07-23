package domains

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestRouterConfigFromManifests_Empty(t *testing.T) {
	// Empty manifests should return the base config (no handoffs)
	base := agentcore.Config{}
	cfg := RouterConfigFromManifests(base, nil)

	// With no manifests, RouterConfigFromManifests returns base as-is
	// (no name override, no handoffs).
	if len(cfg.Handoffs) != 0 {
		t.Errorf("expected 0 handoffs for empty manifests, got %d", len(cfg.Handoffs))
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
	// AllowedSources 包含 mady-router, chat-agent, mady-agent
	if len(h.AllowedSources) != 3 {
		t.Errorf("expected 3 AllowedSources, got %d: %v", len(h.AllowedSources), h.AllowedSources)
	}
	for _, want := range []string{"mady-router", "chat-agent", "mady-agent"} {
		found := false
		for _, got := range h.AllowedSources {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllowedSources missing %q, got %v", want, h.AllowedSources)
		}
	}
	if h.FallbackMsg == "" {
		t.Error("FallbackMsg should not be empty")
	}
	// chat domain 现在映射到 UnifiedAgentConfig，所以 AgentConfig.Name 是 "mady-agent"
	if h.AgentConfig.Name != "mady-agent" {
		t.Errorf("expected AgentConfig.Name 'mady-agent', got %q", h.AgentConfig.Name)
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
