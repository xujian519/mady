package chat

// chat_history_links_test.go — ChatHistory 的链接元数据（core.LinkProvider）
// 端到端验证：渲染证据卡后 RenderLinks 返回与可见行对齐的链接。

import (
	"testing"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
)

func TestChatHistoryRenderLinksEvidenceCard(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "请核对证据"})
	h.Append(ChatMessage{
		Role: RoleAssistant,
		DomainMsg: &component.DomainMessage{
			Type:  component.DomainMsgTypeEvidenceCard,
			Title: "新颖性证据核对",
			Spans: []component.EvidenceRef{
				{SourceURI: "file://d1.pdf", PageRange: "p3", Direction: component.DirectionSupporting, URL: "https://docs.example.com/d1#p3"},
			},
			Body: "经比对，权利要求1具备新颖性。",
		},
	})

	lines := h.Render(80)
	links := h.RenderLinks(80)

	if len(lines) != len(links) {
		t.Fatalf("RenderLinks must align with Render rows: lines=%d links=%d", len(lines), len(links))
	}

	// 找到证据行并验证链接。
	found := false
	for i, ls := range links {
		if len(ls) == 1 && ls[0].URL == "https://docs.example.com/d1#p3" {
			found = true
			// 链接列区间必须在行宽内。
			if ls[0].EndCol > int64(len([]rune(core.StripAnsi(lines[i])))) {
				t.Errorf("link span [%d,%d) exceeds row width %d: %q",
					ls[0].StartCol, ls[0].EndCol, len([]rune(core.StripAnsi(lines[i]))), lines[i])
			}
		}
	}
	if !found {
		t.Error("evidence card link not found in RenderLinks output")
	}
}

func TestChatHistoryRenderLinksViewportSync(t *testing.T) {
	// 视口裁剪后，RenderLinks 必须与可见行对齐（裁剪同步）。
	h := NewChatHistory()
	for i := 0; i < 8; i++ {
		h.Append(ChatMessage{Role: RoleUser, Text: "填充消息"})
	}
	h.Append(ChatMessage{
		Role: RoleAssistant,
		DomainMsg: &component.DomainMessage{
			Type:  component.DomainMsgTypeEvidenceCard,
			Title: "证据",
			Spans: []component.EvidenceRef{
				{SourceURI: "file://d.pdf", Direction: component.DirectionSupporting, URL: "https://x.com/1"},
			},
		},
	})
	h.SetMaxRows(4)

	lines := h.Render(60)
	links := h.RenderLinks(60)

	if len(lines) != 4 {
		t.Fatalf("viewport should clip to 4 rows, got %d", len(lines))
	}
	if len(links) != len(lines) {
		t.Fatalf("clipped RenderLinks(%d) must align with clipped Render rows(%d)", len(links), len(lines))
	}
}

func TestChatHistoryRenderLinksNoLinks(t *testing.T) {
	// 无链接消息：RenderLinks 返回等长 nil。
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "plain"})
	h.Append(ChatMessage{Role: RoleAssistant, Text: "plain reply"})

	lines := h.Render(60)
	links := h.RenderLinks(60)
	if len(links) != len(lines) {
		t.Fatalf("links(%d) must align with rows(%d)", len(links), len(lines))
	}
	for i, ls := range links {
		if len(ls) != 0 {
			t.Errorf("row %d should have no links, got %d", i, len(ls))
		}
	}
}
