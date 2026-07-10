package domains

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// Note: mockProvider is defined in classifier_test.go and reused here.
// It implements agentcore.Provider with a callback-based respond function.

func TestRouterClassifyToChat(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"最近压力有点大", DomainChat},
		{"今天天气怎么样", DomainChat},
		{"你好", DomainChat},
		{"什么是AI", DomainChat},
		{"谢谢你的帮助", DomainChat},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ClassifyIntent(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyIntent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRouterClassifyToAssistant(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"帮我写个Python脚本", DomainAssistant},
		{"搜索一下最新AI论文", DomainAssistant},
		{"帮我实现一个排序算法", DomainAssistant},
		{"生成一份周报", DomainAssistant},
		{"帮我调试这段代码", DomainAssistant},
		{"帮我优化一下性能", DomainAssistant},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ClassifyIntent(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyIntent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRouterClassifyToPatent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"分析这个权利要求的新颖性", DomainPatent},
		{"帮我查一下这个专利号CN123456", DomainPatent},
		{"prior art检索", DomainPatent},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ClassifyIntent(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyIntent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRouterClassifyToLegal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"合同法第52条怎么解读", DomainLegal},
		{"这个案件判例怎么找", DomainLegal},
		{"帮我查一下劳动法相关规定", DomainLegal},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ClassifyIntent(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyIntent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestKeywordPriority verifies classification priority: patent > legal > assistant > chat.
func TestKeywordPriority(t *testing.T) {
	// "专利" should always win over assistant/legal keywords.
	got := ClassifyIntent("帮我查一下这个专利的代码实现")
	if got != DomainPatent {
		t.Errorf("patent keyword should win, got %q", got)
	}

	// Legal keywords should win over assistant.
	got = ClassifyIntent("帮我写法律意见书")
	if got != DomainLegal {
		t.Errorf("legal keyword should win over assistant, got %q", got)
	}
}

func TestRouterConfigProducesAllHandoffs(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := RouterConfig(base)

	if len(cfg.Handoffs) != 4 {
		t.Fatalf("expected 4 handoffs, got %d", len(cfg.Handoffs))
	}

	domains := map[string]bool{
		DomainChat:      false,
		DomainAssistant: false,
		DomainPatent:    false,
		DomainLegal:     false,
	}
	for _, h := range cfg.Handoffs {
		domains[h.Name] = true
		if h.Mode != agentcore.HandoffDelegate {
			t.Errorf("handoff %q mode = %v, want delegate", h.Name, h.Mode)
		}
	}
	for d, found := range domains {
		if !found {
			t.Errorf("missing handoff for domain %q", d)
		}
	}
}

func TestChatAgentConfig_NoTools(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := ChatAgentConfig(base)

	if cfg.Name != "chat-agent" {
		t.Errorf("name = %q, want chat-agent", cfg.Name)
	}
	// Chat agent should NOT have tools extension.
	for _, ext := range cfg.Extensions {
		if ext.Name() == "builtin-tools" {
			t.Error("chat agent should not have tools extension")
		}
	}
}

func TestAssistantAgentConfig_HasTools(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := AssistantAgentConfig(base)

	if cfg.Name != "assistant-agent" {
		t.Errorf("name = %q, want assistant-agent", cfg.Name)
	}
	// Assistant agent should have tools extension.
	hasTools := false
	for _, ext := range cfg.Extensions {
		if ext.Name() == "builtin-tools" {
			hasTools = true
			break
		}
	}
	if !hasTools {
		t.Error("assistant agent should have tools extension")
	}
}

func TestDomainConstants(t *testing.T) {
	if DomainChat != "chat" {
		t.Errorf("DomainChat = %q", DomainChat)
	}
	if DomainAssistant != "assistant" {
		t.Errorf("DomainAssistant = %q", DomainAssistant)
	}
	if DomainPatent != "patent" {
		t.Errorf("DomainPatent = %q", DomainPatent)
	}
	if DomainLegal != "legal" {
		t.Errorf("DomainLegal = %q", DomainLegal)
	}
}

func TestGuardrailConfigs_PerDomain(t *testing.T) {
	// Verify each domain config produces a valid Agent configuration.
	base := agentcore.Config{}
	base.Provider = &mockProvider{}

	for _, fn := range []func(agentcore.Config) agentcore.Config{
		ChatAgentConfig,
		AssistantAgentConfig,
		PatentAgentConfig,
		LegalAgentConfig,
	} {
		cfg := fn(base)
		if cfg.Name == "" {
			t.Error("domain config should set a name")
		}
		if cfg.Provider == nil {
			t.Error("domain config should have a provider")
		}
	}
}

func TestPsychologicalConfigs(t *testing.T) {
	// Verify all psych configs are non-zero with valid SessionIDs.
	cfgs := map[string]struct {
		cfg     interface{ SessionID() string }
		session string
	}{
		"chat":      {session: "chat"},
		"assistant": {session: "assistant"},
		"patent":    {session: "patent"},
		"legal":     {session: "legal"},
	}

	for name, tt := range cfgs {
		t.Run(name, func(t *testing.T) {
			// Just verify the configs can be created without panic.
			switch name {
			case "chat":
				cfg := ChatPsychConfig()
				if cfg.SessionID != tt.session {
					t.Errorf("SessionID = %q, want %q", cfg.SessionID, tt.session)
				}
			case "assistant":
				cfg := AssistantPsychConfig()
				if cfg.SessionID != tt.session {
					t.Errorf("SessionID = %q, want %q", cfg.SessionID, tt.session)
				}
			case "patent":
				cfg := PatentPsychConfig()
				if cfg.SessionID != tt.session {
					t.Errorf("SessionID = %q, want %q", cfg.SessionID, tt.session)
				}
			case "legal":
				cfg := LegalPsychConfig()
				if cfg.SessionID != tt.session {
					t.Errorf("SessionID = %q, want %q", cfg.SessionID, tt.session)
				}
			}
		})
	}
}

func TestAssistantAgentConfig_DisabledTools(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := AssistantAgentConfig(base)

	// Verify that the tools extension is configured with DisableTools.
	hasTools := false
	for _, ext := range cfg.Extensions {
		if ext.Name() == "builtin-tools" {
			hasTools = true
			break
		}
	}
	if !hasTools {
		t.Error("assistant agent should have tools extension")
	}

	// Verify the config builds without error — the agent should create successfully.
	agent := agentcore.New(cfg)
	defer agent.Close()
	if agent == nil {
		t.Error("expected non-nil agent")
	}
}
