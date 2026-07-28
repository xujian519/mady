package main

// skill_panel.go wires the SkillCenter component into the TUI as a
// centered overlay, following the same pattern as settings_panel.go.

import (
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/skill"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
)

// openSkillCenter builds a skill management panel from the current session
// state and opens it as a focused, dimmed overlay.
func (s *tuiSession) openSkillCenter() {
	items := buildSkillItems(s.fc.BaseConfig.AvailableSkills, s.fc.BaseConfig.SelectedSkills)
	sc := component.NewSkillCenter()
	sc.SetItems(items)

	var ov chat.OverlayRef
	sc.SetOnSelect(func(item component.SkillItem) {
		if ov != nil {
			s.app.CloseOverlay(ov)
		}
		// Submit /skill:name to the agent for the skill extension to expand
		s.submitInput("/skill:" + item.Name)
	})

	box := component.NewBox()
	box.SetBorder(component.BorderRounded)
	box.SetTitle("技能中心 — ↑↓ 浏览 · Enter 启用  ·  Esc 关闭")
	box.SetPadding(1, 1)
	box.AddChild(sc)

	ov = s.app.OpenOverlay(box, chat.OverlayOpts{WidthPct: 60, HeightPct: 50, Dim: true, Category: chat.OverlayCatReview})
}

// buildSkillItems converts []skill.Skill and selected names to SkillItem slice.
func buildSkillItems(skills []skill.Skill, selected []string) []component.SkillItem {
	selectedMap := make(map[string]bool, len(selected))
	for _, name := range selected {
		selectedMap[name] = true
	}

	items := make([]component.SkillItem, 0, len(skills))
	for _, sk := range skills {
		state := "available"
		if selectedMap[sk.Name] {
			state = "active"
		}

		provenance := skillProvenance(sk.FilePath)

		items = append(items, component.SkillItem{
			Name:       sk.Name,
			State:      state,
			Provenance: provenance,
			Pinned:     false,
			UseCount:   0,
		})
	}
	return items
}

// skillProvenance derives a human-readable origin label from the skill's
// file path (e.g. "project", "global", "user").
func skillProvenance(filePath string) string {
	if filePath == "" {
		return "unknown"
	}
	// Normalize to forward slashes for consistent matching
	path := filepath.ToSlash(filePath)

	switch {
	case strings.Contains(path, "/.agent/"):
		return "agent"
	case strings.Contains(path, "/plugins/"):
		return "plugin"
	case strings.Contains(path, "/skills/"):
		return "skills"
	default:
		return "other"
	}
}
