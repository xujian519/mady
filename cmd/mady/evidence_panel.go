package main

// evidence_panel.go wires the EvidenceOverlay component into the TUI as a
// centered overlay for browsing evidence (tool-call ledger receipts).
// Follows the same pattern as settings_panel.go and skill_panel.go.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

// openEvidenceOverlay builds an EvidenceOverlay from the session's available
// evidence sources and opens it as a dimmed overlay.
//
// When query is non-empty, shows a hint directing the user to /knowledge.
// When empty, it shows the current tool-call ledger receipts.
func (s *tuiSession) openEvidenceOverlay(query string) {
	overlay := component.NewEvidenceOverlay()

	if query != "" {
		s.app.PrintSystem("📚 知识检索结果暂不支持可视化浏览。使用 /knowledge <关键词> 查看文本结果。")
		return
	}

	title, items := buildLedgerEvidence(s.fc.EvidenceExt)
	overlay.SetTitle(title)
	overlay.SetItems(items)

	var ov chat.OverlayRef
	overlay.SetOnClose(func() {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("引用证据 — ↑↓ 浏览 · Esc 关闭")
	box.SetPadding(1, 1)
	box.AddChild(overlay)

	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 60, HeightPct: 50, Dim: true, Category: chat.OverlayCatReview})
}

// buildLedgerEvidence converts an EvidenceExtension into overlay content.
// Returns default empty values when the extension or its ledger is nil/empty.
func buildLedgerEvidence(ext *evidence.EvidenceExtension) (string, []component.EvidenceItem) {
	if ext == nil {
		return "引用证据详情", nil
	}
	ledger := ext.Ledger()
	if ledger == nil || ledger.Len() == 0 {
		return "引用证据详情", nil
	}
	receipts := ledger.Snapshot()
	items := make([]component.EvidenceItem, 0, len(receipts))
	for _, r := range receipts {
		label := r.ToolName
		if r.Success {
			label += " ✓"
		} else {
			label += " ✗"
		}
		excerpt := r.Command
		if excerpt == "" && len(r.Paths) > 0 {
			excerpt = strings.Join(r.Paths, ", ")
		}
		items = append(items, component.EvidenceItem{
			Title:   label,
			Source:  fmt.Sprintf("耗时 %dms", r.DurationMs),
			Score:   -1,
			Excerpt: excerpt,
		})
	}
	return fmt.Sprintf("本轮工具调用证据（%d 条）", len(receipts)), items
}
