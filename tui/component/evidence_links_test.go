package component

// evidence_links_test.go — RenderEvidenceCardWithLinks 的链接元数据验证。

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestRenderEvidenceCardWithLinks(t *testing.T) {
	msg := &DomainMessage{
		Type:  DomainMsgTypeEvidenceCard,
		Title: "证据核对",
		Spans: []EvidenceRef{
			{SourceURI: "file://d1.pdf", PageRange: "p3", Direction: DirectionSupporting, URL: "https://docs.example.com/d1#p3"},
			{SourceURI: "file://d2.pdf", PageRange: "p5", Direction: DirectionContradicting}, // 无 URL
		},
	}
	lines, links := RenderEvidenceCardWithLinks(msg, false, DefaultEvidenceCardTheme(), 80)

	if len(lines) != len(links) {
		t.Fatalf("lines(%d) and links(%d) must be aligned", len(lines), len(links))
	}
	if len(lines) < 3 {
		t.Fatalf("expected header + 2 evidence rows, got %d lines", len(lines))
	}

	// 第 1 条证据行（index 1）应有链接，覆盖 loc 文本（"file://d1.pdf · p3"）。
	row1 := links[1]
	if len(row1) != 1 {
		t.Fatalf("evidence row 1 should have 1 link, got %d", len(row1))
	}
	ls := row1[0]
	if ls.URL != "https://docs.example.com/d1#p3" {
		t.Errorf("link URL = %q", ls.URL)
	}
	// 列区间应覆盖来源文本，且 StartCol 在方向指示之后（非 0）。
	if ls.StartCol <= 0 || ls.EndCol <= ls.StartCol {
		t.Errorf("link span [%d,%d) invalid", ls.StartCol, ls.EndCol)
	}
	// 序列化后 OSC 8 包裹的来源文本应与行内容一致。
	serialized := core.SerializeRow(core.ParseLine(lines[1]))
	if got := core.StripAnsi(serialized); got == "" {
		t.Error("serialized row should contain visible text")
	}

	// 第 2 条证据行（index 2）无 URL → 无链接。
	if len(links[2]) != 0 {
		t.Errorf("evidence row 2 (no URL) should have 0 links, got %d", len(links[2]))
	}
}

func TestRenderEvidenceCardWithLinksCollapsed(t *testing.T) {
	msg := &DomainMessage{
		Type:  DomainMsgTypeEvidenceCard,
		Title: "证据核对",
		Spans: []EvidenceRef{{SourceURI: "file://d1.pdf", Direction: DirectionSupporting, URL: "https://x.com"}},
	}
	lines, links := RenderEvidenceCardWithLinks(msg, true, DefaultEvidenceCardTheme(), 80)
	if len(links) != 0 {
		t.Errorf("collapsed card should have no links, got %d", len(links))
	}
	if len(lines) == 0 {
		t.Error("collapsed card should still render a summary line")
	}
}

func TestRenderEvidenceCardWithLinksWidePrefix(t *testing.T) {
	// 方向指示含宽字符（⊕/⊖）时，链接 StartCol 仍按可见列计算。
	msg := &DomainMessage{
		Type:  DomainMsgTypeEvidenceCard,
		Title: "证据核对",
		Spans: []EvidenceRef{
			{SourceURI: "file://d1.pdf", Direction: DirectionSupporting, URL: "https://x.com/1"},
		},
	}
	_, links := RenderEvidenceCardWithLinks(msg, false, DefaultEvidenceCardTheme(), 80)
	ls := links[1][0]
	// prefix = "  " + "⊕ supporting" + "  "；⊕ 在 CJK 模式外为 1 列。
	// 不依赖具体宽度，只验证链接起始列在来源文本处（> 方向文本宽度）。
	if ls.StartCol < 8 {
		t.Errorf("link should start after direction text, got StartCol=%d", ls.StartCol)
	}
}
