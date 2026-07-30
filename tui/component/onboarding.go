package component

// onboarding.go — FirstRunWizard component.
//
// Shows a brief interactive guide when the chat history is empty.
// Displays 3-5 quick-start tips to help new users discover key features.
// Auto-dismisses when the user sends their first message or presses Esc.
//
// 输出示例（宽 50 列）：
//
//	╭────────────────────────────────────╮
//	│  👋 欢迎使用 Mady                    │
//	│  ├ 输入 `/` 搜索对话历史              │
//	│  ├ 按 `Ctrl+P` 打开命令中心           │
//	│  ├ 按 `?` 查看全部快捷键              │
//	│  └ 按 `Enter` 开始输入                │
//	│                                      │
//	│  按 Esc 关闭此引导                    │
//	╰────────────────────────────────────╯

import (
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// OnboardingTip is a single tip displayed in the welcome guide.
type OnboardingTip struct {
	Key  string // e.g. "/"
	Desc string // e.g. "搜索对话历史"
}

// FirstRunWizard shows a welcome guide for first-time users.
type FirstRunWizard struct {
	mu        sync.RWMutex
	visible   bool
	dismissed bool
	onDismiss func()
	onRender  func()
}

// NewFirstRunWizard creates a welcome guide. It starts visible.
func NewFirstRunWizard() *FirstRunWizard {
	return &FirstRunWizard{
		visible: true,
	}
}

// Dismiss hides the wizard permanently for this session.
func (w *FirstRunWizard) Dismiss() {
	w.mu.Lock()
	w.visible = false
	w.dismissed = true
	w.mu.Unlock()
	w.requestRender()
	if fn := w.onDismiss; fn != nil {
		fn()
	}
}

// IsVisible reports whether the wizard should be shown.
func (w *FirstRunWizard) IsVisible() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.visible && !w.dismissed
}

// SetOnDismiss registers a callback fired when the wizard is dismissed.
func (w *FirstRunWizard) SetOnDismiss(fn func()) {
	w.mu.Lock()
	w.onDismiss = fn
	w.mu.Unlock()
}

// SetOnRequestRender wires a render request callback.
func (w *FirstRunWizard) SetOnRequestRender(fn func()) {
	w.mu.Lock()
	w.onRender = fn
	w.mu.Unlock()
}

func (w *FirstRunWizard) requestRender() {
	w.mu.RLock()
	fn := w.onRender
	w.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (w *FirstRunWizard) Invalidate() {}

func (w *FirstRunWizard) Render(width int64) []string {
	w.mu.RLock()
	visible := w.visible && !w.dismissed
	w.mu.RUnlock()

	if !visible {
		return nil
	}

	if width < 30 {
		width = 30
	}

	pal := theme.CurrentPalette()
	innerW := width - 4 // 2-char padding on each side
	if innerW < 1 {
		innerW = 1
	}

	var lines []string
	pad := func(s string) string {
		return core.PadToWidth("  "+s, width)
	}

	// Top border
	lines = append(lines, pal.Accent.Render("╭"+strings.Repeat("─", int(width)-2)+"╮"))

	// Welcome
	lines = append(lines, pad(pal.Accent.Render("👋 欢迎使用 Mady — 证据驱动专利案件工作台")))

	// Tips
	tips := []struct{ Key, Desc string }{
		{"/", "搜索对话历史"},
		{"Ctrl+P", "打开命令中心"},
		{"?", "查看全部快捷键"},
		{"Enter", "开始输入"},
	}
	for _, tip := range tips {
		tipLine := "  ├ " + pal.Accent.Render(tip.Key) + " " + pal.Dim.Render(tip.Desc)
		lines = append(lines, core.PadToWidth(tipLine, width))
	}

	// Dismiss hint
	lines = append(lines, "")
	lines = append(lines, pad(pal.Dim.Render("按 Esc 关闭此引导")))

	// Bottom border
	lines = append(lines, pal.Accent.Render("╰"+strings.Repeat("─", int(width)-2)+"╯"))

	return lines
}

func (w *FirstRunWizard) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		for _, k := range terminal.ParseKeys(m.Data, m.KittyFlags) {
			if strings.ToLower(k.Name) == "escape" {
				w.Dismiss()
				return nil
			}
		}
	}
	return nil
}

// Ensure FirstRunWizard implements core.Component.
var _ core.Component = (*FirstRunWizard)(nil)
