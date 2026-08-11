package chat

// chat_history_user_prefix_test.go — 方案 A 视觉区分回归测试：
// 用户消息渲染时必须应用 UserPrefix 标记（UserStyle 着色）+ 续行缩进，
// 与无前缀的智能体输出形成视觉区分。

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestUserMessageRendersWithPrefix 验证用户消息首行带配置的前缀。
func TestUserMessageRendersWithPrefix(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "请分析新颖性"})

	lines := h.Render(60)
	joined := core.StripAnsi(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "> 请分析新颖性") {
		t.Fatalf("user message should render with prefix, got:\n%q", joined)
	}
}

// TestUserMessageContinuationIndent 验证多行用户消息的续行缩进到前缀宽度，
// 使整条消息读作一个视觉块。
func TestUserMessageContinuationIndent(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "第一段内容\n\n第二段内容"})

	lines := h.Render(60)
	var sawIndent bool
	for _, ln := range lines {
		clean := core.StripAnsi(ln)
		if strings.HasPrefix(clean, "  第二段内容") {
			sawIndent = true
		}
	}
	if !sawIndent {
		joined := core.StripAnsi(strings.Join(lines, "\n"))
		t.Fatalf("continuation line should be indented to prefix width, got:\n%q", joined)
	}
}

// TestUserMessagePrefixDoesNotOverflow 验证长用户消息加前缀后仍不超出渲染宽度
// （前缀占用 innerWidth，markdown 必须按缩减后的宽度换行）。
func TestUserMessagePrefixDoesNotOverflow(t *testing.T) {
	h := NewChatHistory()
	long := strings.Repeat("很长的一段用户输入内容", 20) // 240 汉字
	h.Append(ChatMessage{Role: RoleUser, Text: long})

	const width = 60
	for i, ln := range h.Render(width) {
		if vw := core.VisibleWidth(ln); vw > width {
			t.Errorf("line %d exceeds width %d (visible=%d) %q", i, width, vw, core.StripAnsi(ln))
		}
	}
}

// TestUserMessageEmptyPrefixFallsBackToPlain 验证配置空前缀时退化为纯 markdown
// （向后兼容：自定义主题可关闭前缀）。
func TestUserMessageEmptyPrefixFallsBackToPlain(t *testing.T) {
	th := DefaultChatHistoryTheme()
	th.UserPrefix = ""
	h := NewChatHistory()
	h.SetTheme(th)
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})

	lines := h.Render(60)
	joined := core.StripAnsi(strings.Join(lines, "\n"))
	if !strings.HasPrefix(joined, "hello") {
		t.Fatalf("empty prefix should fall back to plain text, got %q", joined)
	}
}

// TestUserMessageCustomPrefix 验证自定义前缀（如 "❯ "）生效。
func TestUserMessageCustomPrefix(t *testing.T) {
	th := DefaultChatHistoryTheme()
	th.UserPrefix = "❯ "
	h := NewChatHistory()
	h.SetTheme(th)
	h.Append(ChatMessage{Role: RoleUser, Text: "你好"})

	joined := core.StripAnsi(strings.Join(h.Render(60), "\n"))
	if !strings.Contains(joined, "❯ 你好") {
		t.Fatalf("custom prefix should apply, got %q", joined)
	}
}

// TestAssistantMessageStaysUnprefixed 验证智能体消息保持无前缀（回归保护：
// 方案 A 只动用户侧，智能体输出视觉不变）。
func TestAssistantMessageStaysUnprefixed(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})
	h.Append(ChatMessage{Role: RoleAssistant, Text: "world"})

	lines := h.Render(60)
	joined := core.StripAnsi(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "> hello") {
		t.Fatalf("user message missing prefix: %q", joined)
	}
	if strings.Contains(joined, "> world") || strings.Contains(joined, "❯ world") {
		t.Fatalf("assistant message must not carry a user prefix: %q", joined)
	}
}
