package chat

// Copy and clipboard shortcut helpers — doCopy, doCopyToClipboard,
// hasSelection, and keyboard shortcut detection.

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xujian519/mady/tui/terminal"
)

func doCopyToClipboard(l *chatLayout, text string) {
	go func() {
		if err := CopyToClipboard(text); err != nil {
			l.app.PrintError(fmt.Errorf("clipboard: %w", err))
		} else {
			runeCount := utf8.RuneCountInString(text)
			l.app.PrintSystem(fmt.Sprintf("📋 已复制（%d 字符）", runeCount))
		}
	}()
}

func isCopyShortcut(k terminal.Key) bool {
	name := strings.ToLower(k.Name)
	if name == "c" {
		if isPrimaryShortcutMod(k.Mods) {
			return true
		}
		if k.Mods&terminal.ModCtrl != 0 && k.Mods&terminal.ModShift != 0 {
			return true
		}
	}
	if name == "insert" {
		// Ctrl+Insert is copy; Shift+Insert is paste (handled by isPasteShortcut).
		return k.Mods&terminal.ModCtrl != 0 && k.Mods&terminal.ModShift == 0
	}
	return false
}

func isPasteShortcut(k terminal.Key) bool {
	name := strings.ToLower(k.Name)
	if name == "v" {
		if isPrimaryShortcutMod(k.Mods) {
			return true
		}
		if k.Mods&terminal.ModCtrl != 0 && k.Mods&terminal.ModShift != 0 {
			return true
		}
	}
	if name == "insert" {
		return k.Mods&terminal.ModShift != 0
	}
	return false
}

func doCopy(l *chatLayout) {
	// Copy editor selection first.
	if sel, ok := l.editor.(textSelectionComponent); ok {
		if text := sel.GetSelectedText(); text != "" {
			doCopyToClipboard(l, text)
			return
		}
	}
	// Copy history selection
	text := l.history.GetSelectedText()
	if text != "" {
		doCopyToClipboard(l, text)
		return
	}
	// 无显式选区时，复制最后一条助手消息（最常用场景）。
	msgs := l.history.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleAssistant && msgs[i].Text != "" {
			doCopyToClipboard(l, msgs[i].Text)
			return
		}
	}
}

func isPrimaryShortcutMod(mods terminal.Modifier) bool {
	return mods&terminal.ModSuper != 0 || mods&terminal.ModMeta != 0
}

// hasSelection reports whether there is currently selected text in the editor
// or the history viewport.
func hasSelection(l *chatLayout) bool {
	if sel, ok := l.editor.(textSelectionComponent); ok {
		if sel.GetSelectedText() != "" {
			return true
		}
	}
	return l.history.GetSelectedText() != ""
}
