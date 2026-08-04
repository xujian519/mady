package chat

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// streamingWidthFixture is the streaming counterpart of the component-level
// fixture: CJK heading, table with bold header, and a long CJK paragraph.
const streamingWidthFixture = `### "原说明书和权利要求书记载的范围"如何认定？

| 允许修改的理由 | 限制修改的理由 |
|---|---|
| ① 申请人的表达和认知能力存在局限 | ① 促使申请人在申请阶段**充分公开**发明 |
| ② 提高申请文件质量，便于公众理解和运用 | ② 防止申请人**不正当取得先申请利益**（把申请日后的新内容补进去） |
| ③ 保障社会公众对专利信息的**信赖** |  |

①人的表达认知能力存在局限促使人在充分公开②提高质量便于公众理解和运用`

// TestStreamingMarkdownWidthClamp streams a CJK Markdown document through
// ChatHistory in small chunks and asserts that EVERY intermediate render
// frame stays within the viewport width — not just the final one. The
// streaming intermediate states (partial table separators, partial UTF-8
// sequences) are where the historical overflow bugs lived.
func TestStreamingMarkdownWidthClamp(t *testing.T) {
	h := NewChatHistory()
	h.SetMaxRows(60)
	h.Append(ChatMessage{Role: RoleUser, Text: "请总结专利法第33条"})

	const width = 80
	const chunkSize = 40
	aid := ""
	for i := 0; i < len(streamingWidthFixture); i += chunkSize {
		end := i + chunkSize
		if end > len(streamingWidthFixture) {
			end = len(streamingWidthFixture)
		}
		aid = h.AppendDelta(aid, streamingWidthFixture[i:end])
		for j, ln := range h.Render(width) {
			if vw := core.VisibleWidth(ln); vw > width {
				t.Errorf("intermediate frame after %d bytes: line %d exceeds width %d (visible=%d) %q",
					end, j, width, vw, core.StripAnsi(ln))
			}
		}
	}
	h.PatchMessage(aid, func(m *ChatMessage) { m.Pending = false })
	assertStreamingWidths(t, h.Render(width), width)
}

// TestStreamingTableWidthClamp streams a pipe table one rune at a time — the
// harshest case: the separator line "|---|---|" arrives as single runes and
// the delta-dedup must not swallow the repeated '-', or the table structure
// collapses (a historical bug, see applyDeltaLocked).
func TestStreamingTableWidthClamp(t *testing.T) {
	h := NewChatHistory()
	h.SetMaxRows(40)
	h.Append(ChatMessage{Role: RoleUser, Text: "表格"})

	src := `| 允许修改的理由 | 限制修改的理由 |
|---|---|
| ① 申请人的表达和认知能力存在局限 | ① 促使申请人在申请阶段**充分公开**发明 |
| ② 提高申请文件质量，便于公众理解和运用 | ② 防止申请人**不正当取得先申请利益**（把申请日后的新内容补进去） |
| ③ 保障社会公众对专利信息的**信赖** |  |
`
	const width = 80
	aid := ""
	for i := 0; i < len(src); i++ {
		// src[i:i+1] keeps raw bytes. (string(src[i]) would re-encode the
		// byte as Latin-1 → UTF-8, corrupting every multi-byte rune — a test
		// artifact that once masqueraded as a streaming bug.)
		aid = h.AppendDelta(aid, src[i:i+1])
		assertStreamingWidths(t, h.Render(width), width)
	}
	h.PatchMessage(aid, func(m *ChatMessage) { m.Pending = false })
	lines := h.Render(width)
	assertStreamingWidths(t, lines, width)

	// The content must survive rune-by-rune streaming intact: the delta-dedup
	// must not swallow repeated '-' in the separator (a historical bug), and
	// every cell's text must still be present. Note the table may fall back
	// to vertical layout at this width, so assert on cell text, not on the
	// horizontal separator.
	joined := ""
	for _, ln := range lines {
		joined += core.StripAnsi(ln) + "\n"
	}
	for _, want := range []string{"允许修改的理由", "限制修改的理由", "充分公开", "不正当取得先申请利益", "信赖"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rune streaming corrupted table content: missing %q in:\n%s", want, joined)
		}
	}
}

func assertStreamingWidths(t *testing.T, lines []string, width int64) {
	t.Helper()
	for i, ln := range lines {
		if vw := core.VisibleWidth(ln); vw > width {
			t.Errorf("line %d exceeds width %d (visible=%d) %q", i, width, vw, core.StripAnsi(ln))
		}
	}
}
