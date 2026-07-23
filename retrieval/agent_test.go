package retrieval

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// TriggerPolicy
// ---------------------------------------------------------------------------

func TestTriggerPolicyDefaults(t *testing.T) {
	cfg := DefaultRetrievalConfig()
	if cfg.TriggerPolicy != TriggerAlways {
		t.Fatalf("expected TriggerAlways, got %s", cfg.TriggerPolicy)
	}
	if cfg.FirstNTurns != 3 {
		t.Fatalf("expected 3 first_n_turns, got %d", cfg.FirstNTurns)
	}
}

func TestTriggerAlways(t *testing.T) {
	hook := &RetrievalHook{
		chunks: []Chunk{{Content: "test content"}},
		config: RetrievalConfig{TriggerPolicy: TriggerAlways},
	}
	arc := &agentcore.AgentRunContext{Messages: []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "test"},
	}}
	if !hook.shouldTrigger(arc, "test") {
		t.Fatal("TriggerAlways should always trigger")
	}
}

func TestTriggerFirstN(t *testing.T) {
	hook := &RetrievalHook{
		chunks: []Chunk{{Content: "test"}},
		config: RetrievalConfig{TriggerPolicy: TriggerFirstN, FirstNTurns: 3},
	}

	// First 3 turns should trigger
	arc := &agentcore.AgentRunContext{}
	for turn := 1; turn <= 3; turn++ {
		hook.turnCount = int64(turn - 1)
		if !hook.shouldTrigger(arc, "分析专利新颖性") {
			t.Fatalf("turn %d should trigger", turn)
		}
	}

	// Turn 4 should NOT trigger
	hook.turnCount = 3
	if hook.shouldTrigger(arc, "分析专利新颖性") {
		t.Fatal("turn 4 should not trigger")
	}
}

func TestTriggerOnDemand(t *testing.T) {
	hook := &RetrievalHook{
		chunks: []Chunk{{Content: "test"}},
		config: RetrievalConfig{TriggerPolicy: TriggerOnDemand},
	}
	arc := &agentcore.AgentRunContext{}
	if hook.shouldTrigger(arc, "分析专利新颖性") {
		t.Fatal("OnDemand should not auto-trigger")
	}
}

type mockComplexityClassifier struct{}

func (m mockComplexityClassifier) Classify(_ string, _ []agentcore.Message) agentcore.Complexity {
	return agentcore.ComplexityMedium
}

type mockLowComplexity struct{}

func (m mockLowComplexity) Classify(_ string, _ []agentcore.Message) agentcore.Complexity {
	return agentcore.ComplexityLow
}

func TestTriggerSmart(t *testing.T) {
	// Medium complexity should trigger
	hook := &RetrievalHook{
		chunks: []Chunk{{Content: "test"}},
		config: RetrievalConfig{
			TriggerPolicy:        TriggerSmart,
			ComplexityClassifier: mockComplexityClassifier{},
		},
	}
	arc := &agentcore.AgentRunContext{Messages: []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "分析专利新颖性"},
	}}
	if !hook.shouldTrigger(arc, "分析专利新颖性") {
		t.Fatal("Medium complexity should trigger")
	}
}

func TestTriggerSmart_LowComplexity(t *testing.T) {
	// Low complexity should NOT trigger
	hook := &RetrievalHook{
		chunks: []Chunk{{Content: "test"}},
		config: RetrievalConfig{
			TriggerPolicy:        TriggerSmart,
			ComplexityClassifier: mockLowComplexity{},
		},
	}
	arc := &agentcore.AgentRunContext{Messages: []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "你好"},
	}}
	if hook.shouldTrigger(arc, "分析专利新颖性") {
		t.Fatal("Low complexity should not trigger")
	}
}

func TestTriggerSmart_NilClassifier(t *testing.T) {
	// No classifier = fallback to trigger
	hook := &RetrievalHook{
		chunks: []Chunk{{Content: "test"}},
		config: RetrievalConfig{
			TriggerPolicy: TriggerSmart,
		},
	}
	arc := &agentcore.AgentRunContext{Messages: []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "test"},
	}}
	if !hook.shouldTrigger(arc, "分析专利新颖性") {
		t.Fatal("nil classifier should fallback to trigger")
	}
}

// ---------------------------------------------------------------------------
// RetrievalHook construction with TriggerPolicy
// ---------------------------------------------------------------------------

func TestNewRetrievalHookWithTriggerPolicy(t *testing.T) {
	// Verify TriggerFirstN passes through the constructor
	// (it's in RetrievalConfig, not a separate mechanism)
	chunks := []Chunk{{Content: "test"}}
	cfg := RetrievalConfig{
		TopK:          3,
		TriggerPolicy: TriggerFirstN,
		FirstNTurns:   5,
	}
	hook := NewRetrievalHook(chunks, cfg)
	if hook.config.TriggerPolicy != TriggerFirstN {
		t.Fatalf("expected TriggerFirstN, got %s", hook.config.TriggerPolicy)
	}
	if hook.config.FirstNTurns != 5 {
		t.Fatalf("expected 5, got %d", hook.config.FirstNTurns)
	}
}

// ---------------------------------------------------------------------------
// BeforeModelCall with chunk query
// ---------------------------------------------------------------------------

func TestRetrievalHook_BeforeModelCall_Nil(t *testing.T) {
	// Verify no crash with nil mcc
	hook := NewRetrievalHook(nil, DefaultRetrievalConfig())
	ctx := context.Background()
	arc := &agentcore.AgentRunContext{}
	err := hook.BeforeModelCall(ctx, arc, nil)
	if err != nil {
		t.Fatalf("should not error: %v", err)
	}
}

func TestRetrievalHook_EmptyChunks(t *testing.T) {
	hook := NewRetrievalHook(nil, DefaultRetrievalConfig())
	ctx := context.Background()
	arc := &agentcore.AgentRunContext{Messages: []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "test"},
	}}
	mcc := &agentcore.ModelCallContext{Request: &agentcore.ProviderRequest{}}

	err := hook.BeforeModelCall(ctx, arc, mcc)
	if err != nil {
		t.Fatalf("should not error on empty chunks: %v", err)
	}
}
