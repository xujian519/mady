package domains

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestUnifiedAgentConfig(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	if cfg.Name != "mady-agent" {
		t.Errorf("name = %q, want %q", cfg.Name, "mady-agent")
	}
	// UnifiedAgentConfig 应注册 patent/legal 两个 Handoff（不含 assistant/chat 自身）
	if len(cfg.Handoffs) != 2 {
		t.Fatalf("handoffs = %d, want 2", len(cfg.Handoffs))
	}

	expectedDomains := map[string]bool{
		DomainPatent: false,
		DomainLegal:  false,
	}
	for _, h := range cfg.Handoffs {
		expectedDomains[h.Name] = true
		if h.Mode != agentcore.HandoffDelegate {
			t.Errorf("handoff %q mode = %v, want delegate", h.Name, h.Mode)
		}
		if !h.Invisible {
			t.Errorf("handoff %q should be invisible", h.Name)
		}
	}
	for domain, found := range expectedDomains {
		if !found {
			t.Errorf("missing handoff for domain %q", domain)
		}
	}
}

func TestUnifiedAgentConfig_AllowedSources(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := UnifiedAgentConfig(base)

	for _, h := range cfg.Handoffs {
		found := false
		for _, src := range h.AllowedSources {
			if src == "mady-agent" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("handoff %q AllowedSources missing 'mady-agent'", h.Name)
		}
		// 确保不包含通配符
		for _, src := range h.AllowedSources {
			if src == "*" {
				t.Errorf("handoff %q AllowedSources should not contain '*'", h.Name)
			}
		}
	}
}

func TestAppendLifecycle(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		result := appendLifecycle(nil, nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("first nil returns second", func(t *testing.T) {
		hook := agentcore.BaseLifecycleHook{}
		result := appendLifecycle(nil, &hook)
		if result != &hook {
			t.Errorf("expected second hook, got %v", result)
		}
	})

	t.Run("second nil returns first", func(t *testing.T) {
		hook := agentcore.BaseLifecycleHook{}
		result := appendLifecycle(&hook, nil)
		if result != &hook {
			t.Errorf("expected first hook, got %v", result)
		}
	})

	t.Run("both non-nil creates chain", func(t *testing.T) {
		a := agentcore.BaseLifecycleHook{}
		b := agentcore.BaseLifecycleHook{}
		result := appendLifecycle(&a, &b)

		chain, ok := result.(agentcore.LifecycleChain)
		if !ok {
			t.Fatalf("expected LifecycleChain, got %T", result)
		}
		if len(chain) != 2 {
			t.Fatalf("chain length = %d, want 2", len(chain))
		}
		if chain[0] != &a || chain[1] != &b {
			t.Errorf("unexpected chain order")
		}
	})

	t.Run("append to existing chain", func(t *testing.T) {
		a := agentcore.BaseLifecycleHook{}
		b := agentcore.BaseLifecycleHook{}
		c := agentcore.BaseLifecycleHook{}
		chain := agentcore.LifecycleChain{&a, &b}

		result := appendLifecycle(chain, &c)
		extended, ok := result.(agentcore.LifecycleChain)
		if !ok {
			t.Fatalf("expected LifecycleChain, got %T", result)
		}
		if len(extended) != 3 {
			t.Fatalf("chain length = %d, want 3", len(extended))
		}
		if extended[2] != &c {
			t.Errorf("expected c at index 2")
		}
	})
}
