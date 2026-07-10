package domains

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/workflow"
)

func TestRouterConfig(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := RouterConfig(base)

	if cfg.Name != "mady-router" {
		t.Errorf("name = %q, want %q", cfg.Name, "mady-router")
	}
	if len(cfg.Handoffs) != 3 {
		t.Fatalf("handoffs = %d, want 3", len(cfg.Handoffs))
	}

	expectedDomains := map[string]bool{
		DomainChat:   false,
		DomainPatent: false,
		DomainLegal:  false,
	}
	for _, h := range cfg.Handoffs {
		expectedDomains[h.Name] = true
		if h.Mode != agentcore.HandoffDelegate {
			t.Errorf("handoff %q mode = %v, want delegate", h.Name, h.Mode)
		}
	}
	for domain, found := range expectedDomains {
		if !found {
			t.Errorf("missing handoff for domain %q", domain)
		}
	}
}

func TestRouterConfigWithClassifier_NilUsesKeyword(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}
	cfg := RouterConfigWithClassifier(base, nil)

	// Should produce the same result as RouterConfig.
	if cfg.Name != "mady-router" {
		t.Errorf("name = %q, want %q", cfg.Name, "mady-router")
	}
	if len(cfg.Handoffs) != 3 {
		t.Fatalf("handoffs = %d, want 3", len(cfg.Handoffs))
	}
}

func TestRouterStep(t *testing.T) {
	chatStep := &stubStep{name: "chat"}
	patentStep := &stubStep{name: "patent"}
	legalStep := &stubStep{name: "legal"}

	step := RouterStep(chatStep, patentStep, legalStep)

	if step == nil {
		t.Fatal("step is nil")
	}

	// The returned step should be a *workflow.Router.
	router, ok := step.(*workflow.Router)
	if !ok {
		t.Fatalf("expected *workflow.Router, got %T", step)
	}
	if len(router.Steps) != 3 {
		t.Fatalf("router steps = %d, want 3", len(router.Steps))
	}
}

func TestRouterStepWithClassifier_NilUsesKeyword(t *testing.T) {
	chatStep := &stubStep{name: "chat"}
	patentStep := &stubStep{name: "patent"}
	legalStep := &stubStep{name: "legal"}

	step := RouterStepWithClassifier(chatStep, patentStep, legalStep, nil)
	_, ok := step.(*workflow.Router)
	if !ok {
		t.Fatalf("expected *workflow.Router, got %T", step)
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

// stubStep implements workflow.Step for testing.
type stubStep struct {
	name string
}

func (s *stubStep) Run(_ context.Context, _ string) (string, error) {
	return s.name, nil
}
