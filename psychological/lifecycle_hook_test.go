package psychological

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// NewLifecycleHook
// ---------------------------------------------------------------------------

func TestNewLifecycleHook_ReturnsNonNil(t *testing.T) {
	hook := NewLifecycleHook(Config{})
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}
}

func TestNewLifecycleHook_ReturnsPsychologicalHook(t *testing.T) {
	hook := NewLifecycleHook(Config{})
	if _, ok := hook.(*psychologicalHook); !ok {
		t.Fatalf("expected *psychologicalHook, got %T", hook)
	}
}

// ---------------------------------------------------------------------------
// psychologicalHook.BeforeAgentRun
// ---------------------------------------------------------------------------

func TestPsychologicalHook_BeforeAgentRun_InjectsSystemMessage(t *testing.T) {
	hook := NewLifecycleHook(Config{SkipDistortionDetection: true})

	arc := &agentcore.AgentRunContext{
		Input:    "我今天心情不太好，这个案子总是被驳回",
		Messages: []agentcore.Message{},
	}

	err := hook.BeforeAgentRun(context.Background(), arc)
	if err != nil {
		t.Fatalf("BeforeAgentRun returned error: %v", err)
	}

	// Should have wrapped the input with psychological system messages.
	if len(arc.Messages) == 0 {
		t.Fatal("expected at least one message after BeforeAgentRun")
	}

	// The first message should be a system message with psychological context.
	if arc.Messages[0].Role != agentcore.RoleSystem {
		t.Fatalf("expected first message role=system, got %q", arc.Messages[0].Role)
	}
	if arc.Messages[0].Content == "" {
		t.Fatal("expected non-empty system message content")
	}
}

func TestPsychologicalHook_BeforeAgentRun_EmptyInput(t *testing.T) {
	hook := NewLifecycleHook(Config{})

	arc := &agentcore.AgentRunContext{
		Input:    "",
		Messages: []agentcore.Message{},
	}

	// Empty input should not cause an error or panic.
	err := hook.BeforeAgentRun(context.Background(), arc)
	if err != nil {
		t.Fatalf("BeforeAgentRun with empty input: %v", err)
	}
	if len(arc.Messages) != 0 {
		t.Fatal("expected no messages injected for empty input")
	}
}

func TestPsychologicalHook_BeforeAgentRun_NilARC(t *testing.T) {
	hook := NewLifecycleHook(Config{})

	// Must not panic.
	err := hook.BeforeAgentRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeforeAgentRun with nil ARC: %v", err)
	}
}

func TestPsychologicalHook_BeforeAgentRun_NegativeInput_ProducesEmpatheticStrategy(t *testing.T) {
	hook := NewLifecycleHook(Config{SkipDistortionDetection: true})

	arc := &agentcore.AgentRunContext{
		Input:    "这太让人失望了，老是驳回我的意见，我真的很担心很害怕！",
		Messages: []agentcore.Message{},
	}

	err := hook.BeforeAgentRun(context.Background(), arc)
	if err != nil {
		t.Fatalf("BeforeAgentRun returned error: %v", err)
	}

	// Verify the system message contains empathetic-related content.
	if len(arc.Messages) == 0 {
		t.Fatal("expected messages after BeforeAgentRun")
	}
	content := arc.Messages[0].Content
	if content == "" {
		t.Fatal("expected non-empty system message content")
	}
	if !strings.Contains(content, "empathetic") && !strings.Contains(content, "共情") {
		t.Errorf("expected empathetic strategy for negative input, got:\n%s", content)
	}
}

func TestPsychologicalHook_BeforeAgentRun_PreservesExistingMessages(t *testing.T) {
	hook := NewLifecycleHook(Config{SkipDistortionDetection: true})

	existingMsg := agentcore.Message{Role: agentcore.RoleSystem, Content: "existing system prompt"}
	arc := &agentcore.AgentRunContext{
		Input:    "帮我分析一下这篇专利",
		Messages: []agentcore.Message{existingMsg},
	}

	err := hook.BeforeAgentRun(context.Background(), arc)
	if err != nil {
		t.Fatalf("BeforeAgentRun returned error: %v", err)
	}

	// The psychological context should be prepended before existing messages,
	// so total message count should be original + 1.
	if len(arc.Messages) != 2 {
		t.Fatalf("expected 2 messages (injected + original), got %d", len(arc.Messages))
	}
	// Original message should still be present.
	if arc.Messages[1].Content != "existing system prompt" {
		t.Fatalf("original message content preserved, got %q", arc.Messages[1].Content)
	}
}
