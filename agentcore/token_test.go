package agentcore

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("empty got %d", got)
	}
	if got := EstimateTokens("hello"); got != 2 {
		t.Fatalf("'hello' got %d", got)
	}
	// 1000 chars / 4 = 250 tokens
	s := string(make([]byte, 1000))
	if got := EstimateTokens(s); got != 250 {
		t.Fatalf("1000 chars got %d", got)
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	msg := Message{
		Role:    RoleUser,
		Content: "hello world",
	}
	tokens := EstimateMessageTokens(msg)
	if tokens <= 0 {
		t.Fatalf("expected positive token count, got %d", tokens)
	}
	// 4 (overhead) + 3 ('hello world' = 11/4 = 2.75 -> 3) = 7
	if tokens != 7 {
		t.Fatalf("expected 7 tokens, got %d", tokens)
	}
}

func TestEstimateMessageTokensWithToolCalls(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{
			{Name: "get_weather", Arguments: `{"city":"london"}`},
		},
	}
	tokens := EstimateMessageTokens(msg)
	if tokens <= 0 {
		t.Fatalf("expected positive token count, got %d", tokens)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: "hello there"},
	}
	tokens := EstimateMessagesTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token count, got %d", tokens)
	}
}

func TestEstimateToolDefinitionsTokens(t *testing.T) {
	tokens := EstimateToolDefinitionsTokens(nil)
	if tokens != 0 {
		t.Fatalf("expected 0 for nil, got %d", tokens)
	}

	tokens = EstimateToolDefinitionsTokens([]ToolDefinition{})
	if tokens != 0 {
		t.Fatalf("expected 0 for empty, got %d", tokens)
	}

	defs := []ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			Parameters:  map[string]any{"type": "object"},
		},
	}
	tokens = EstimateToolDefinitionsTokens(defs)
	if tokens <= 0 {
		t.Fatalf("expected positive token count, got %d", tokens)
	}
}

func TestCountCJKRunes(t *testing.T) {
	// Pure ASCII → 0
	if got := countCJKRunes("hello world"); got != 0 {
		t.Fatalf("ASCII: expected 0, got %d", got)
	}
	// Pure Chinese → 5
	if got := countCJKRunes("你好世界吗"); got != 5 {
		t.Fatalf("Chinese: expected 5, got %d", got)
	}
	// Mixed → 3
	if got := countCJKRunes("hello 世界 abc"); got != 2 {
		t.Fatalf("Mixed: expected 2, got %d", got)
	}
	// Japanese Hiragana
	if got := countCJKRunes("こんにちは"); got != 5 {
		t.Fatalf("Hiragana: expected 5, got %d", got)
	}
	// Korean Hangul
	if got := countCJKRunes("안녕하세요"); got != 5 {
		t.Fatalf("Hangul: expected 5, got %d", got)
	}
}

func TestEstimateTokens_CJK(t *testing.T) {
	// Chinese text should get a HIGHER estimate than pure ASCII of the same
	// byte length, because CJK characters cost more tokens in real tokenizers.
	chineseText := strings.Repeat("你", 100) // 100 CJK chars, 300 bytes UTF-8
	asciiText := strings.Repeat("a", 300)   // 300 bytes

	cjkTokens := EstimateTokens(chineseText)
	asciiTokens := EstimateTokens(asciiText)

	if cjkTokens <= asciiTokens {
		t.Fatalf("CJK text should estimate higher than ASCII of same byte length: cjk=%d ascii=%d",
			cjkTokens, asciiTokens)
	}
	// Base for 300 bytes = 75 tokens. CJK correction = 100 * 0.75 = 75.
	// Total should be ~150.
	if cjkTokens < 100 {
		t.Fatalf("CJK estimate too low: expected >= 100, got %d", cjkTokens)
	}
}
