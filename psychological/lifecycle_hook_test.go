package psychological

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// Extension.TransformContext — 替代旧 NewLifecycleHook 路径
// ---------------------------------------------------------------------------

func TestExtension_TransformContext_InjectsSystemMessage(t *testing.T) {
	ext := NewExtension(Config{SkipDistortionDetection: true})

	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "我今天心情不太好，这个案子总是被驳回"},
	}

	result := ext.TransformContext(context.Background(), msgs)

	// TransformContext 在最后一条用户消息之前插入心理分析系统消息
	if len(result) < 2 {
		t.Fatalf("expected at least 2 messages after TransformContext, got %d", len(result))
	}

	// 系统消息应在用户消息之前
	if result[0].Role != agentcore.RoleSystem {
		t.Fatalf("expected first message role=system, got %q", result[0].Role)
	}
	if result[0].Content == "" {
		t.Fatal("expected non-empty system message content")
	}
}

func TestExtension_TransformContext_EmptyInput(t *testing.T) {
	ext := NewExtension(Config{})

	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: ""},
	}

	result := ext.TransformContext(context.Background(), msgs)

	// 空输入应不注入，返回原消息
	if len(result) != 1 {
		t.Fatalf("expected 1 message for empty input, got %d", len(result))
	}
}

func TestExtension_TransformContext_EmptyMessages(t *testing.T) {
	ext := NewExtension(Config{})

	result := ext.TransformContext(context.Background(), nil)

	if len(result) != 0 {
		t.Fatal("expected no messages for empty input")
	}
}

func TestExtension_TransformContext_NegativeInput_ProducesEmpatheticStrategy(t *testing.T) {
	ext := NewExtension(Config{SkipDistortionDetection: true})

	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "这太让人失望了，老是驳回我的意见，我真的很担心很害怕！"},
	}

	result := ext.TransformContext(context.Background(), msgs)

	if len(result) < 2 {
		t.Fatal("expected messages after TransformContext")
	}
	content := result[0].Content
	if content == "" {
		t.Fatal("expected non-empty system message content")
	}
	if !strings.Contains(content, "empathetic") && !strings.Contains(content, "共情") {
		t.Errorf("expected empathetic strategy for negative input, got:\n%s", content)
	}
}

func TestExtension_TransformContext_PreservesExistingMessages(t *testing.T) {
	ext := NewExtension(Config{SkipDistortionDetection: true})

	existingMsg := agentcore.Message{Role: agentcore.RoleSystem, Content: "existing system prompt"}
	msgs := []agentcore.Message{
		existingMsg,
		{Role: agentcore.RoleUser, Content: "帮我分析一下这篇专利"},
	}

	result := ext.TransformContext(context.Background(), msgs)

	// 心理上下文在最后一条用户消息前注入，所以总消息数 = 原有 + 1
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (original system + injected + user), got %d", len(result))
	}

	// 检查原有系统消息和用户消息是否保留
	foundExisting := false
	foundUser := false
	for _, msg := range result {
		if msg.Content == "existing system prompt" {
			foundExisting = true
		}
		if msg.Content == "帮我分析一下这篇专利" {
			foundUser = true
		}
	}
	if !foundExisting {
		t.Fatal("existing system message was lost")
	}
	if !foundUser {
		t.Fatal("user message was lost")
	}
}

func TestExtension_TransformContext_DuplicateInput_NotDoubleInjected(t *testing.T) {
	ext := NewExtension(Config{SkipDistortionDetection: true})

	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "重复输入测试"},
	}

	// 第一次调用：应注入
	result1 := ext.TransformContext(context.Background(), msgs)
	if len(result1) < 2 {
		t.Fatal("expected injection on first call")
	}

	// 第二次相同输入：不应重复注入
	result2 := ext.TransformContext(context.Background(), msgs)
	if len(result2) != 1 {
		t.Fatalf("expected no duplicate injection, got %d messages", len(result2))
	}
}
