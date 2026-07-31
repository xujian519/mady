package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func testSkillItems() []SkillItem {
	return []SkillItem{
		{Name: "chat", State: "active", Provenance: "builtin", Pinned: true, UseCount: 5},
		{Name: "patent", State: "stale", Provenance: "plugin", UseCount: 0},
		{Name: "legal", State: "archived", Provenance: "plugin"},
		{Name: "browser", State: "weird", Provenance: "custom"},
	}
}

func TestSkillCenterConstruction(t *testing.T) {
	s := NewSkillCenter()
	if s.theme.Title == "" {
		t.Fatal("expected default title")
	}
	if s.km == nil {
		t.Fatal("expected keybindings")
	}
	s.SetTitle("自定义")
	if s.theme.Title != "自定义" {
		t.Fatalf("expected title set, got %q", s.theme.Title)
	}
	s.Invalidate() // no-op
}

func TestSkillCenterSetItems(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	if len(s.items) != 4 || len(s.filtered) != 4 {
		t.Fatalf("expected 4 items, got %d/%d", len(s.items), len(s.filtered))
	}
	if s.selected != 0 {
		t.Fatalf("expected selected 0, got %d", s.selected)
	}
	// SetItems resets the filter.
	s.filter = "chat"
	s.SetItems(testSkillItems())
	if s.filter != "" {
		t.Fatalf("expected filter reset, got %q", s.filter)
	}
}

func TestSkillCenterFilter(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	s.filter = "pat"
	s.applyFilterLocked()
	if len(s.filtered) != 1 || s.filtered[0].Name != "patent" {
		t.Fatalf("expected patent only, got %v", s.filtered)
	}
	// Provenance matching.
	s.filter = "plugin"
	s.applyFilterLocked()
	if len(s.filtered) != 2 {
		t.Fatalf("expected 2 plugin skills, got %v", s.filtered)
	}
	// No match.
	s.filter = "zzz"
	s.applyFilterLocked()
	if len(s.filtered) != 0 {
		t.Fatalf("expected no matches, got %v", s.filtered)
	}
}

func TestSkillCenterNavigation(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	s.Update(core.KeyMsg{Data: "\x1b[A"}) // up wraps to last
	if s.selected != 3 {
		t.Fatalf("expected wrap to 3, got %d", s.selected)
	}
	s.Update(core.KeyMsg{Data: "\x1b[B"}) // down wraps to 0
	if s.selected != 0 {
		t.Fatalf("expected wrap to 0, got %d", s.selected)
	}
	// Move with no items — no-op.
	s2 := NewSkillCenter()
	s2.Update(core.KeyMsg{Data: "\x1b[B"})
	if s2.selected != 0 {
		t.Fatalf("expected selected 0, got %d", s2.selected)
	}
}

func TestSkillCenterConfirm(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	var selected []string
	s.SetOnSelect(func(it SkillItem) { selected = append(selected, it.Name) })
	s.Update(core.KeyMsg{Data: "\x1b[B"}) // -> patent
	s.Update(core.KeyMsg{Data: "\r"})     // enter
	if len(selected) != 1 || selected[0] != "patent" {
		t.Fatalf("expected patent selected, got %v", selected)
	}
	// No callback — no-op.
	s2 := NewSkillCenter()
	s2.SetItems(testSkillItems())
	s2.Update(core.KeyMsg{Data: "\r"})
	// Empty filtered — no-op.
	s3 := NewSkillCenter()
	s3.Update(core.KeyMsg{Data: "\r"})
}

func TestSkillCenterSetOnInvalidate(t *testing.T) {
	s := NewSkillCenter()
	called := false
	s.SetOnInvalidate(func() { called = true })
	// Invalidate() is a no-op; the callback is only for parent wiring.
	s.Invalidate()
	if called {
		t.Fatal("Invalidate must be a no-op")
	}
}

func TestSkillCenterRender(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	s.height = 20
	lines := s.Render(60)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Skills", "Active: 1 | Stale: 1 | Archived: 1 | Total: 4", "chat", "patent", "legal", "📌", "(5 uses)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in render, got %q", want, joined)
		}
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 60 {
			t.Fatalf("line width %d > 60 (line=%q)", w, ln)
		}
	}
}

func TestSkillCenterRenderSelected(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	s.selected = 1
	lines := s.Render(60)
	if !strings.Contains(strings.Join(lines, "\n"), "▸") {
		t.Fatal("expected selection cursor")
	}
}

func TestSkillCenterRenderFilterIndicator(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	s.filter = "chat"
	s.applyFilterLocked()
	lines := s.Render(60)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Filter: chat") || !strings.Contains(joined, "1 of 4") {
		t.Fatalf("expected filter indicator, got %q", joined)
	}
}

func TestSkillCenterRenderEmpty(t *testing.T) {
	s := NewSkillCenter()
	lines := s.Render(60)
	if !strings.Contains(strings.Join(lines, "\n"), "No skills found") {
		t.Fatalf("expected empty message, got %q", strings.Join(lines, "\n"))
	}
}

func TestSkillCenterStateIconAndStyle(t *testing.T) {
	s := NewSkillCenter()
	cases := map[string]string{
		"active":   "●",
		"stale":    "◐",
		"archived": "○",
		"unknown":  "○",
		"":         "○",
	}
	for state, want := range cases {
		if got := s.stateIcon(state); got != want {
			t.Fatalf("stateIcon(%q) = %q, want %q", state, got, want)
		}
	}
	for _, state := range []string{"active", "stale", "archived", "other"} {
		_ = s.stateStyle(state)
	}
}

func TestSkillCenterCountStates(t *testing.T) {
	s := NewSkillCenter()
	s.SetItems(testSkillItems())
	active, stale, archived := s.countStates()
	if active != 1 || stale != 1 || archived != 1 {
		t.Fatalf("unexpected counts %d,%d,%d", active, stale, archived)
	}
}

func TestSkillCenterFormatSkillItemTruncates(t *testing.T) {
	s := NewSkillCenter()
	long := strings.Repeat("very-long-skill-name", 10)
	s.SetItems([]SkillItem{{Name: long, State: "active"}})
	line := s.formatSkillItem(s.items[0], 30)
	if w := core.VisibleWidth(line); w > 30 {
		t.Fatalf("formatted line width %d > 30 (line=%q)", w, line)
	}
}
