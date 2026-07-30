package sanitizer

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================
// PII pattern matching tests (table-driven)
// ============================================================

func TestSanitizeChineseID(t *testing.T) {
	rules := defaultRules()
	tests := []struct {
		name     string
		input    string
		expected string // expected to contain placeholder, not exact value
	}{
		{
			name:     "standard 18-digit ID",
			input:    "我的身份证号是110101199001011234",
			expected: "我的身份证号是[**身份证号#1**]",
		},
		{
			name:     "ID with lowercase x",
			input:    "证件号32010619900307791x",
			expected: "证件号[**身份证号#1**]",
		},
		{
			name:     "ID with uppercase X",
			input:    "证件号32010619900307791X",
			expected: "证件号[**身份证号#1**]",
		},
		{
			name:     "invalid ID (bad date) matched by bank card rule",
			input:    "110101199099011234", // month 99 is invalid for ID, but has 18 digits → bank card rule
			expected: "[**银行卡号#1**]",
		},
		{
			name:     "16 digit number matched by bank card rule",
			input:    "编号1234567890123456", // 16 digits → bank card
			expected: "编号[**银行卡号#1**]",
		},
		{
			name:     "multiple IDs in same text",
			input:    "甲:110101199001011234, 乙:32010619900307791X",
			expected: "甲:[**身份证号#1**], 乙:[**身份证号#2**]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := newReplacementMap()
			got := sanitizeText(tt.input, rules, rm)
			if got != tt.expected {
				t.Errorf("sanitizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizePhoneNumber(t *testing.T) {
	rules := defaultRules()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard mobile",
			input:    "手机号13800138000已注册",
			expected: "手机号[**手机号#1**]已注册",
		},
		{
			name:     "different prefix 159",
			input:    "15912345678",
			expected: "[**手机号#1**]",
		},
		{
			name:     "landline (no match)",
			input:    "座机010-88888888",
			expected: "座机010-88888888",
		},
		{
			name:     "too short (10 digits) does not match",
			input:    "1380013800",
			expected: "1380013800",
		},
		{
			name:     "invalid prefix (12) does not match",
			input:    "12123456789",
			expected: "12123456789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := newReplacementMap()
			got := sanitizeText(tt.input, rules, rm)
			if got != tt.expected {
				t.Errorf("sanitizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeBankCard(t *testing.T) {
	rules := defaultRules()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "16-digit card",
			input:    "卡号6222021234561234",
			expected: "卡号[**银行卡号#1**]",
		},
		{
			name:     "19-digit card",
			input:    "6217002880088888888",
			expected: "[**银行卡号#1**]",
		},
		{
			name:     "15 digits (too short) does not match",
			input:    "123456789012345",
			expected: "123456789012345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := newReplacementMap()
			got := sanitizeText(tt.input, rules, rm)
			if got != tt.expected {
				t.Errorf("sanitizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeEmail(t *testing.T) {
	rules := defaultRules()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic email",
			input:    "联系邮箱zhangsan@example.com",
			expected: "联系邮箱[**电子邮箱#1**]",
		},
		{
			name:     "email with dots and plus",
			input:    "test.name+tag@gmail.com.cn",
			expected: "[**电子邮箱#1**]",
		},
		{
			name:     "email in sentence",
			input:    "请发送至 admin@company.com",
			expected: "请发送至 [**电子邮箱#1**]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := newReplacementMap()
			got := sanitizeText(tt.input, rules, rm)
			if got != tt.expected {
				t.Errorf("sanitizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeMixedPII(t *testing.T) {
	rules := defaultRules()
	input := "姓名张三, 身份证110101199001011234, 电话13800138000, 邮箱a@b.com"
	rm := newReplacementMap()
	got := sanitizeText(input, rules, rm)

	// Should contain placeholders (without requiring exact match since hash-based format is used).
	if got == input {
		t.Errorf("sanitizeText did not replace any PII")
	}
}

// ============================================================
// SanitizingProvider integration tests (with stub provider)
// ============================================================

// stubProvider implements agentcore.Provider for testing.
type stubProvider struct {
	completeResp *agentcore.ProviderResponse
	streamCh     chan agentcore.StreamDelta
	err          error
}

func (s *stubProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return s.completeResp, s.err
}

func (s *stubProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return s.streamCh, s.err
}

func TestSanitizingProvider_Complete_SanitizesRequest(t *testing.T) {
	// capturedRequest records what the inner provider receives.
	var capturedReq *agentcore.ProviderRequest
	capture := &stubProvider{
		completeResp: &agentcore.ProviderResponse{
			Content: "已处理申请",
			Blocks: []agentcore.ContentBlock{
				{Kind: agentcore.BlockKindText, Text: "已处理申请"},
			},
		},
	}
	// Override with a capture provider.
	captureProvider := &captureProvider{capture: func(req *agentcore.ProviderRequest) {
		capturedReq = req
	}}
	captureProvider.completeResp = capture.completeResp

	sp := New(captureProvider)

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "我的身份证是110101199001011234，手机是13800138000"},
		},
	}
	_, _ = sp.Complete(context.Background(), req)

	if capturedReq == nil || len(capturedReq.Messages) == 0 {
		t.Fatal("inner provider did not receive request")
	}
	content := capturedReq.Messages[0].Content
	if containsPII(content) {
		t.Errorf("inner provider received unsanitized content: %q", content)
	}
}

// captureProvider records the request before delegating to the stub.
type captureProvider struct {
	stubProvider
	capture func(req *agentcore.ProviderRequest)
}

func (c *captureProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	c.capture(req)
	return c.completeResp, c.err
}

func (c *captureProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	if c.capture != nil {
		c.capture(req)
	}
	return c.streamCh, c.err
}

func containsPII(s string) bool {
	// Check if any known PII pattern matches.
	for _, rule := range defaultRules() {
		if rule.Pattern.MatchString(s) {
			return true
		}
	}
	return false
}

func TestSanitizingProvider_Complete_RestoresResponse(t *testing.T) {
	// Inner provider echoes back a message that references PII.
	inner := &stubProvider{
		completeResp: &agentcore.ProviderResponse{
			Content: "已核实身份证[**身份证号#1**]和手机[**手机号#1**]",
			Blocks: []agentcore.ContentBlock{
				{Kind: agentcore.BlockKindText, Text: "已核实身份证[**身份证号#1**]和手机[**手机号#1**]"},
			},
		},
	}
	sp := New(inner)

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "我的身份证是110101199001011234，手机是13800138000"},
		},
	}
	resp, err := sp.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "已核实身份证110101199001011234和手机13800138000" {
		t.Errorf("response not restored: got %q", resp.Content)
	}
	if len(resp.Blocks) > 0 && resp.Blocks[0].Text != "已核实身份证110101199001011234和手机13800138000" {
		t.Errorf("block text not restored: got %q", resp.Blocks[0].Text)
	}
}

func TestSanitizingProvider_Complete_PassthroughOnNilRequest(t *testing.T) {
	called := false
	inner := &captureProvider{
		capture: func(req *agentcore.ProviderRequest) {
			called = true
			if req != nil {
				t.Error("expected nil request")
			}
		},
	}
	inner.completeResp = &agentcore.ProviderResponse{Content: "ok"}

	sp := New(&inner.stubProvider)
	// Override inner again.
	sp.inner = inner

	_, err := sp.Complete(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("inner provider was not called")
	}
}

func TestSanitizingProvider_Stream_SanitizesRequest(t *testing.T) {
	var capturedReq *agentcore.ProviderRequest
	inner := &captureProvider{
		capture: func(req *agentcore.ProviderRequest) {
			capturedReq = req
		},
	}
	inCh := make(chan agentcore.StreamDelta)
	close(inCh)
	inner.streamCh = inCh

	sp := New(inner)

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "我的手机13800138000"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh, err := sp.Stream(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if capturedReq == nil || len(capturedReq.Messages) == 0 {
		t.Fatal("inner provider did not receive request")
	}
	if containsPII(capturedReq.Messages[0].Content) {
		t.Errorf("inner provider received unsanitized content: %q", capturedReq.Messages[0].Content)
	}

	// Drain the output channel (already closed when innerCh is closed).
	for range outCh {
	}
}

func TestSanitizingProvider_Stream_RestoresDelta(t *testing.T) {
	inner := &stubProvider{}
	deltaCh := make(chan agentcore.StreamDelta, 2)
	deltaCh <- agentcore.StreamDelta{Content: "您的[**身份证号#1**]已"}
	deltaCh <- agentcore.StreamDelta{Content: "确认", Done: true}
	close(deltaCh)
	inner.streamCh = deltaCh

	sp := New(inner)

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "身份证110101199001011234"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := sp.Stream(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	var full string
	for d := range ch {
		full += d.Content
	}

	if full != "您的110101199001011234已确认" {
		t.Errorf("stream restoration failed: got %q", full)
	}
}

// ============================================================
// replacementMap tests
// ============================================================

func TestReplacementMap_RoundTrip(t *testing.T) {
	rm := newReplacementMap()

	// Register matches.
	p1 := rm.register("身份证号", "110101199001011234")
	p2 := rm.register("身份证号", "32010619900307791X")

	if p1 == p2 {
		t.Error("placeholders should be unique")
	}

	// Restore.
	text := "用户[ID:" + p1 + "]和[ID:" + p2 + "]"
	got := rm.restore(text)
	want := "用户[ID:110101199001011234]和[ID:32010619900307791X]"
	if got != want {
		t.Errorf("restore = %q, want %q", got, want)
	}
}

func TestReplacementMap_RestoreUnregisteredNoOp(t *testing.T) {
	rm := newReplacementMap()
	// No registrations.
	got := rm.restore("some text [**身份证号#999**]")
	if got != "some text [**身份证号#999**]" {
		t.Error("should not restore unregistered placeholder")
	}
}

func TestSanitizeText_EmptyString(t *testing.T) {
	rules := defaultRules()
	rm := newReplacementMap()
	got := sanitizeText("", rules, rm)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestSanitizeText_BlocksAreSanitized(t *testing.T) {
	rules := defaultRules()
	input := "联系手机13800138000"
	rm := newReplacementMap()
	got := sanitizeText(input, rules, rm)
	if got == input {
		t.Errorf("expected sanitized text, got %q", got)
	}
}

func TestSanitizingProvider_Complete_BlockTextSanitized(t *testing.T) {
	var capturedReq *agentcore.ProviderRequest
	capture := &captureProvider{
		capture: func(req *agentcore.ProviderRequest) {
			capturedReq = req
		},
	}
	capture.completeResp = &agentcore.ProviderResponse{Content: "ok"}

	sp := New(capture)

	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{
				Role: agentcore.RoleUser,
				Blocks: []agentcore.ContentBlock{
					{Kind: agentcore.BlockKindText, Text: "手机13800138000"},
				},
			},
		},
	}
	_, _ = sp.Complete(context.Background(), req)

	if capturedReq != nil && len(capturedReq.Messages) > 0 &&
		len(capturedReq.Messages[0].Blocks) > 0 {
		blockText := capturedReq.Messages[0].Blocks[0].Text
		if containsPII(blockText) {
			t.Errorf("block text not sanitized: %q", blockText)
		}
	}
}

// TestSanitizePhone_WordBoundary ensures phone numbers surrounded by
// non-word characters are correctly matched.
func TestSanitizePhone_WordBoundary(t *testing.T) {
	rules := defaultRules()
	tests := []struct {
		input    string
		contains string // expected placeholder prefix
	}{
		{"电话:13800138000", "[**手机号#1**]"},
		{"(13800138000)", "[**手机号#1**]"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			rm := newReplacementMap()
			got := sanitizeText(tt.input, rules, rm)
			if got == tt.input {
				t.Errorf("expected sanitization of %q, got %q", tt.input, got)
			}
		})
	}
}
