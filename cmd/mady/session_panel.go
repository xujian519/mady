package main

// session_panel.go wires the SessionSelector component into the TUI as a
// centered overlay for browsing, selecting, renaming, and deleting sessions.
//
// Previously /sessions only printed a text list; now it opens an interactive
// SessionSelector overlay that supports:
//   - fuzzy filtering
//   - enter to select/switch to a session
//   - Ctrl+X to delete a session
//   - Ctrl+R to rename a session
//
// Follows the same pattern as settings_panel.go, skill_panel.go.

import (
	"context"
	"fmt"
	"time"

	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

// openSessionSelector opens the SessionSelector overlay for interactive session
// management. It replaces the old text-only /sessions output.
func (s *tuiSession) openSessionSelector() {
	if s.agentStore == nil {
		s.app.PrintSystem("⚠ 会话持久化未启用")
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	threads, err := s.agentStore.ListThreads(ctx)
	if err != nil {
		s.app.PrintSystem(fmt.Sprintf("⚠ 读取会话列表失败: %v", err))
		return
	}

	items := make([]component.SessionItem, 0, len(threads))
	for _, t := range threads {
		name := t.Name
		if name == "" {
			name = t.ID
			if len(name) > 20 {
				name = name[:20] + "…"
			}
		}
		isCurrent := t.ID == s.currentThreadID
		items = append(items, component.SessionItem{
			ID:        t.ID,
			Name:      name,
			UpdatedAt: t.UpdatedAt.Format("01-02 15:04"),
			MsgCount:  t.MessageCount,
			IsCurrent: isCurrent,
		})
	}

	sel := component.NewSessionSelector()
	sel.SetTitle(fmt.Sprintf("会话列表（共 %d 个）", len(items)))
	sel.SetItems(items)

	var ov chat.OverlayRef

	// OnSelect: switch to the selected session.
	sel.SetOnSelect(func(item component.SessionItem) {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
		if item.ID == s.currentThreadID {
			s.app.PrintSystem(fmt.Sprintf("已在当前会话: %s", item.Name))
			return
		}
		if s.agentStore == nil {
			s.app.PrintSystem("⚠ 会话持久化未启用，无法切换")
			return
		}
		// Save current, switch to new.
		s.persistActiveSession()
		if agent := s.getCurrentAgent(); agent != nil {
			if err := agent.SaveState(context.Background(), s.currentThreadID); err != nil {
				s.app.PrintSystem(fmt.Sprintf("⚠ 保存当前会话失败: %v", err))
			}
		}
		s.currentThreadID = item.ID
		s.persistActiveSession()
		s.rebuildAgent()
		s.app.History().Clear()
		s.app.PrintSystem(fmt.Sprintf("📂 已切换到会话: %s", item.Name))
	})

	// OnCancel: close overlay.
	sel.SetOnCancel(func() {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
	})

	// OnDelete: delete the selected session.
	sel.SetOnDelete(func(item component.SessionItem) {
		if s.agentStore == nil {
			return
		}
		delCtx, delCancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer delCancel()
		if err := s.agentStore.Delete(delCtx, item.ID); err != nil {
			s.app.PrintSystem(fmt.Sprintf("⚠ 删除会话失败: %v", err))
			return
		}
		s.app.PrintSystem(fmt.Sprintf("🗑 已删除会话: %s", item.Name))
		// Remove from list.
		var updated []component.SessionItem
		for _, it := range items {
			if it.ID != item.ID {
				updated = append(updated, it)
			}
		}
		items = updated
		sel.SetItems(items)
	})

	// OnRename: rename the selected session.
	sel.SetOnRename(func(item component.SessionItem, newName string) {
		if s.agentStore == nil {
			return
		}
		renameCtx, renameCancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer renameCancel()
		if err := s.agentStore.SetThreadName(renameCtx, item.ID, newName); err != nil {
			s.app.PrintSystem(fmt.Sprintf("⚠ 重命名失败: %v", err))
			return
		}
		s.app.PrintSystem(fmt.Sprintf("✏️ 已重命名: %s → %s", item.Name, newName))
		// Update in place.
		for i := range items {
			if items[i].ID == item.ID {
				items[i].Name = newName
				break
			}
		}
		sel.SetItems(items)
	})

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("会话管理 — ↑↓ 选择 · Enter 切换 · Ctrl+X 删除 · Ctrl+R 重命名 · Esc 关闭")
	box.SetPadding(1, 1)
	box.AddChild(sel)

	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 65, HeightPct: 55, Dim: true, Category: chat.OverlayCatSelection})
}
