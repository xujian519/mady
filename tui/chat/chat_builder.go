package chat

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Builder helpers — sub-component creation and wiring
// ---------------------------------------------------------------------------

// applyChatDefaults fills in default values for a ChatAppConfig.
func applyChatDefaults(cfg ChatAppConfig) ChatAppConfig {
	if cfg.EditorMinRows <= 0 {
		cfg.EditorMinRows = 1
	}
	if cfg.EditorMaxRows <= 0 {
		cfg.EditorMaxRows = 8
	}
	if cfg.EditorPrompt == "" {
		cfg.EditorPrompt = "❯ "
	}
	return cfg
}

// newChatHistoryWithConfig creates a ChatHistory and applies theme/renderer.
func newChatHistoryWithConfig(cfg ChatAppConfig) *ChatHistory {
	history := NewChatHistory()
	if cfg.Theme != nil {
		history.SetTheme(*cfg.Theme)
	}
	if cfg.ReasoningRenderer != nil {
		history.SetReasoningRenderer(cfg.ReasoningRenderer)
	}
	return history
}

// newChatEditor creates the input editor with config-driven settings.
func newChatEditor(cfg ChatAppConfig, km *terminal.KeybindingsManager) *component.Editor {
	editor := component.NewEditor(km)
	editor.SetMinRows(cfg.EditorMinRows)
	editor.SetMaxRows(cfg.EditorMaxRows)
	editor.SetPrompt(cfg.EditorPrompt, strings.Repeat(" ", int(core.VisibleWidth(cfg.EditorPrompt))))
	editor.SetPromptFn(theme.CurrentPalette().Accent.Render)
	editor.SetFocusIndicator("")
	editor.SetPlaceholder("输入消息…（↑↓ 历史  / 查看命令）")
	editor.SetPlaceholderFn(func(s string) string { return theme.CurrentPalette().Dim.Render(s) })
	editor.SetSelectedBg(theme.CurrentPalette().SelectionBg.Render(""))
	return editor
}

// newChatHeader creates the title header component, or nil if no title is set.
func newChatHeader(cfg ChatAppConfig) *component.TruncatedText {
	if cfg.Title == "" {
		return nil
	}
	return component.NewTruncatedText(theme.CurrentPalette().User.Render(cfg.Title))
}

// newChatAutocomplete sets up autocomplete with file/folder navigation callbacks.
func newChatAutocomplete(cfg ChatAppConfig, a *ChatApp) *component.Autocomplete {
	if len(cfg.Providers) == 0 {
		return nil
	}
	ac := component.NewAutocomplete(cfg.Providers...)
	ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) {
		a.skipRefresh = true
		a.editor.SetValue(newValue)
		if strings.HasPrefix(newValue, "@file:") || strings.HasPrefix(newValue, "@folder:") {
			a.ac.Refresh(newValue, int64(len(newValue)))
		}
		a.host.Focus(a.editor)
		a.host.RequestRender()
	})
	ac.OnDismiss(func() {
		value := a.editor.GetValue()
		if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
			len(value) > len("@file:") {
			newValue := popLastPathSegment(value)
			a.editor.SetValue(newValue)
			a.ac.Refresh(newValue, int64(len(newValue)))
		}
		a.host.RequestRender()
	})
	return ac
}

// newChatLayout builds the layout tree for the chat app.
func newChatLayout(cfg ChatAppConfig, a *ChatApp, history *ChatHistory, editor *component.Editor, loader *component.Loader, statusBar *component.StatusBar) *chatLayout {
	layout := &chatLayout{
		host:          a,
		app:           a,
		history:       history,
		judgmentView:  a.judgmentView,
		editor:        editor,
		loader:        loader,
		statusBar:     statusBar,
		ac:            a.ac,
		todoBar:       &todoBar{app: a},
		editorMaxRows: cfg.EditorMaxRows,
	}
	if a.header != nil {
		layout.header = a.header
	}
	return layout
}

// bindChatEditorEvents wires all editor and history event callbacks.
func bindChatEditorEvents(a *ChatApp, editor *component.Editor, history *ChatHistory) {
	if a.ac != nil {
		editor.SetAutocompleteActiveCheck(a.ac.Active)
	}

	editor.OnChange(func(value string) {
		if a.ac != nil {
			if a.skipRefresh {
				a.skipRefresh = false
			} else {
				a.ac.Refresh(value, int64(len(value)))
			}
			a.host.RequestRender()
		}
	})
	editor.OnSubmit(func(value string) {
		a.onEditorSubmit(value)
	})
	editor.OnCopy(func(text string) {
		go func() {
			if err := CopyToClipboard(text); err != nil {
				a.PrintError(fmt.Errorf("clipboard: %w", err))
				return
			}
			runeCount := utf8.RuneCountInString(text)
			a.PrintSystem(fmt.Sprintf("📋 已复制（%d 字符）", runeCount))
		}()
	})
	editor.OnPaste(func() core.Cmd {
		return func() core.Msg {
			text, err := ReadFromClipboard()
			if err != nil {
				a.PrintError(fmt.Errorf("paste: %w", err))
				a.host.RequestRender()
				return nil
			}
			// Check for oversized paste: store as placeholder.
			if len(text) > pasteThresholdChars || strings.Count(text, "\n") > pasteThresholdLines {
				return a.handlePastePlaceholder(text)
			}
			return core.PasteMsg{Text: text}
		}
	})
	editor.OnCancel(func() {
		if a.cfg.OnQuit != nil {
			a.cfg.OnQuit()
		}
		_ = a.Stop()
	})

	history.SetOnCopy(func(text string) {
		go func() {
			if err := CopyToClipboard(text); err != nil {
				a.PrintError(fmt.Errorf("clipboard: %w", err))
			}
		}()
	})
}
