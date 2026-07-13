package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/layout"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

type AppHost interface {
	Start() error
	Stop() error
	Done() <-chan struct{}

	AddChild(c core.Component)
	Focus(c core.Component)
	RequestRender()

	PushOverlay(ov OverlayRef)
	RemoveOverlay(ov OverlayRef) bool

	TerminalSize() (cols, rows int64)

	// EnableMouse / DisableMouse toggle SGR mouse reporting at runtime.
	// Used to disable mouse when the editor is focused so the terminal's
	// native right-click menu can appear.
	EnableMouse(mode string)
	DisableMouse()
}

type OverlayRef interface {
	OverlayContent() core.Component
	SetOverlayFocus(bool)
	SetOverlayDimBackground(bool)
	OverlayWantsFocus() bool
	OverlayDimBackground() bool
	OverlayAnchor() int
	OverlayPercentX() int
	OverlayPercentY() int
	OverlayWidthPct() int
	OverlayHeightPct() int
}

type ChatAppConfig struct {
	Title string

	KittyKeyboardMode     string
	KittyKeyboardFlags    int64
	DisableBracketedPaste bool
	AltScreen             bool
	MouseMode             string

	EditorMinRows int64
	EditorMaxRows int64
	EditorPrompt  string

	ShowTimings bool
	ShowTurns   bool

	// ReasoningRenderer controls how thinking segments are displayed in the
	// chat history. Pass nil (default) to hide reasoning; pass a
	// *DefaultReasoningRenderer to restore the legacy Show/Mode policy; pass
	// a custom implementation for full control (sidebar, overlay, etc.).
	ReasoningRenderer ReasoningRenderer

	Theme *ChatHistoryTheme

	Providers []core.AutocompleteProvider

	// Context is the cancellation root passed to OnSubmit. When nil,
	// context.Background() is used. Callers should wire the TUI's lifecycle
	// context here so in-flight submissions are canceled on Stop.
	Context context.Context

	OnSubmit     func(ctx context.Context, input string)
	OnQuit       func()
	OnInterrupt  func()
	OnImagePaste func() // called when an image paste is detected (clipboard image, empty text)

	Host AppHost

	// SuppressHandoffToolDisplay when true suppresses transfer_to_* tool calls
	// from appearing in the chat history. Used in integrated mode where handoffs
	// are invisible to the user. In Router mode this should be false so users
	// can see the routing process.
	SuppressHandoffToolDisplay bool
}

type chatModel struct {
	StreamID    string
	ActiveTools map[string]time.Time
	Running     bool
}

type ChatApp struct {
	cfg ChatAppConfig

	host      AppHost
	history   *ChatHistory
	editor    *component.Editor
	loader    *component.Loader
	layout    *chatLayout
	header    *component.TruncatedText
	statusBar *component.StatusBar
	ac        *component.Autocomplete
	km        *terminal.KeybindingsManager

	mu    sync.Mutex
	model chatModel

	helpOverlay OverlayRef

	// SuppressAutoRetry suppresses auto-retry messages from being printed to
	// the chat history. When true, retry events are silently dropped instead of
	// showing "⚠ retry N/M in D". Used by mady to buffer retry messages
	// and only flush them on final failure (Hermes-style buffered retry).
	SuppressAutoRetry bool

	skipRefresh bool // suppress autocomplete re-activation after applying a suggestion
}

func NewChatApp(cfg ChatAppConfig) *ChatApp {
	return newChatApp(cfg)
}

func NewChatAppWithHost(cfg ChatAppConfig, host AppHost) *ChatApp {
	cfg.Host = host
	return newChatApp(cfg)
}

func newChatApp(cfg ChatAppConfig) *ChatApp {
	if cfg.EditorMinRows <= 0 {
		cfg.EditorMinRows = 1
	}
	if cfg.EditorMaxRows <= 0 {
		cfg.EditorMaxRows = 8
	}
	if cfg.EditorPrompt == "" {
		cfg.EditorPrompt = "> "
	}

	km := terminal.NewKeybindingsManager(terminal.DefaultKeybindings())

	history := NewChatHistory()
	if cfg.Theme != nil {
		history.SetTheme(*cfg.Theme)
	}
	if cfg.ReasoningRenderer != nil {
		history.SetReasoningRenderer(cfg.ReasoningRenderer)
	}

	editor := component.NewEditor(km)
	editor.SetMinRows(cfg.EditorMinRows)
	editor.SetMaxRows(cfg.EditorMaxRows)
	editor.SetPrompt(cfg.EditorPrompt, strings.Repeat(" ", len(cfg.EditorPrompt)))
	editor.SetFocusIndicator("")
	editor.SetPlaceholder("Type a message...")
	editor.SetPlaceholderFn(func(s string) string { return theme.CurrentPalette().Dim.Render(s) })

	loader := component.NewLoader(func() {}, theme.CurrentPalette().Dim.Render("thinking..."))

	statusBar := component.NewStatusBar()
	if cfg.Title != "" {
		statusBar.SetMode(cfg.Title)
	}

	chatApp := &ChatApp{
		cfg:       cfg,
		host:      cfg.Host,
		history:   history,
		editor:    editor,
		loader:    loader,
		statusBar: statusBar,
		km:        km,
		model:     chatModel{ActiveTools: make(map[string]time.Time)},
	}

	var header *component.TruncatedText
	if cfg.Title != "" {
		header = component.NewTruncatedText(theme.CurrentPalette().User.Render(cfg.Title))
	}
	chatApp.header = header

	if len(cfg.Providers) > 0 {
		chatApp.ac = component.NewAutocomplete(cfg.Providers...)
		chatApp.ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) {
			chatApp.skipRefresh = true
			chatApp.editor.SetValue(newValue)
			// Force a refresh only when switching to a file/folder browser so
			// that cascading providers activate immediately.
			if strings.HasPrefix(newValue, "@file:") || strings.HasPrefix(newValue, "@folder:") {
				chatApp.ac.Refresh(newValue, int64(len(newValue)))
			}
			chatApp.host.Focus(chatApp.editor)
			chatApp.host.RequestRender()
		})
		chatApp.ac.OnDismiss(func() {
			// File browser: ESC navigates up one directory level instead of
			// dismissing outright. At the root, dismiss normally.
			value := chatApp.editor.GetValue()
			if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
				len(value) > len("@file:") {
				// Remove the last path segment (file or directory).
				newValue := popLastPathSegment(value)
				chatApp.editor.SetValue(newValue)
				chatApp.ac.Refresh(newValue, int64(len(newValue)))
			}
			chatApp.host.RequestRender()
		})
	}

	layout := &chatLayout{
		host:      chatApp,
		app:       chatApp,
		history:   history,
		editor:    editor,
		loader:    loader,
		statusBar: statusBar,
		ac:        chatApp.ac,
	}
	if header != nil {
		layout.header = header
	}
	chatApp.layout = layout

	editor.OnChange(func(value string) {
		if chatApp.ac != nil {
			if chatApp.skipRefresh {
				chatApp.skipRefresh = false
			} else {
				chatApp.ac.Refresh(value, int64(len(value)))
			}
			chatApp.host.RequestRender()
		}
	})
	editor.OnSubmit(func(value string) {
		chatApp.onEditorSubmit(value)
	})
	editor.OnCancel(func() {
		if cfg.OnQuit != nil {
			cfg.OnQuit()
		}
		chatApp.Stop()
	})

	history.SetOnCopy(func(text string) {
		go func() {
			if err := CopyToClipboard(text); err != nil {
				chatApp.PrintError(fmt.Errorf("clipboard: %w", err))
			}
		}()
	})

	return chatApp
}

func (a *ChatApp) SetHost(host AppHost) {
	a.host = host
	a.loader = component.NewLoader(host.RequestRender, theme.CurrentPalette().Dim.Render("thinking..."))
	a.layout.loader = a.loader
	a.history.SetOnInvalidate(host.RequestRender)
}

func (a *ChatApp) Host() AppHost { return a.host }

func (a *ChatApp) History() *ChatHistory { return a.history }

func (a *ChatApp) Editor() *component.Editor { return a.editor }

func (a *ChatApp) Loader() *component.Loader { return a.loader }

func (a *ChatApp) Keybindings() *terminal.KeybindingsManager { return a.km }

func (a *ChatApp) StatusBar() *component.StatusBar { return a.statusBar }

// UpdateStatusBar updates the status bar title after provider/model/mode switches.
func (a *ChatApp) UpdateStatusBar(provider, model, mode string) {
	if a.statusBar == nil {
		return
	}
	a.statusBar.SetMode(fmt.Sprintf("mady · %s/%s · %s", provider, model, mode))
}

func (a *ChatApp) SetFooter(f core.Component) {
	a.layout.footer = f
	if a.host != nil {
		a.host.RequestRender()
	}
}

func (a *ChatApp) Footer() core.Component {
	return a.layout.footer
}

func (a *ChatApp) Start() error {
	a.host.AddChild(a.layout)
	if err := a.host.Start(); err != nil {
		return err
	}
	a.host.Focus(a.editor)
	return nil
}

func (a *ChatApp) Stop() error { return a.host.Stop() }

func (a *ChatApp) Done() <-chan struct{} { return a.host.Done() }

func (a *ChatApp) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model.Running
}

func (a *ChatApp) Subscribe(sub EventSubscriber) {
	sub.On(ChatEventAgentStart, a.onAgentStart)
	sub.On(ChatEventMessageDelta, a.onMessageDelta)
	sub.On(ChatEventToolCallStart, a.onToolStart)
	sub.On(ChatEventToolCallEnd, a.onToolEnd)
	sub.On(ChatEventHandoffStart, a.onHandoffStart)
	sub.On(ChatEventHandoffEnd, a.onHandoffEnd)
	sub.On(ChatEventTurnStart, a.onTurnStart)
	sub.On(ChatEventTurnEnd, a.onTurnEnd)
	sub.On(ChatEventCompactionStart, a.onCompactionStart)
	sub.On(ChatEventCompactionEnd, a.onCompactionEnd)
	sub.On(ChatEventAutoRetry, a.onAutoRetry)
	sub.On(ChatEventAgentError, a.onAgentError)
	sub.On(ChatEventAgentEnd, a.onAgentEnd)
}

func (a *ChatApp) PrintSystem(msg string) {
	a.history.Append(ChatMessage{Role: RoleSystem, Text: msg})
}

func (a *ChatApp) PrintError(err error) {
	if err == nil {
		return
	}
	a.history.Append(ChatMessage{Role: RoleError, Text: err.Error()})
}

func (a *ChatApp) PrintUser(input string) {
	a.history.Append(ChatMessage{Role: RoleUser, Text: input})
}

func (a *ChatApp) Busy(message string) {
	if message != "" {
		a.loader.SetMessage(theme.CurrentPalette().Dim.Render(message))
	}
	a.loader.Start()
	a.mu.Lock()
	a.model.Running = true
	a.mu.Unlock()
	if a.statusBar != nil {
		a.statusBar.Busy()
	}
	a.editor.SetPlaceholder("Ctrl+C to interrupt")
	a.host.RequestRender()
}

func (a *ChatApp) Idle() {
	a.loader.Stop()
	a.mu.Lock()
	a.model.Running = false
	a.mu.Unlock()
	if a.statusBar != nil {
		a.statusBar.Idle()
	}
	a.editor.SetPlaceholder("")
	a.host.RequestRender()
}

func (a *ChatApp) PrintStatus(message string) {
	a.loader.SetMessage(theme.CurrentPalette().Dim.Render(message))
	a.host.RequestRender()
}

func (a *ChatApp) ToggleKeyHelp() OverlayRef {
	a.mu.Lock()
	if a.helpOverlay != nil {
		// Close path: capture the overlay under the lock, then call host
		// methods outside the lock. Calling host.RemoveOverlay/PushOverlay
		// under a.mu would acquire host.mu / TUI.mu; if any other path took
		// those locks first and then ChatApp.mu, we'd deadlock.
		ov := a.helpOverlay
		a.helpOverlay = nil
		editor := a.editor
		a.mu.Unlock()
		a.host.RemoveOverlay(ov)
		a.host.Focus(editor)
		return nil
	}
	help := component.NewKeyHelp(a.km)
	help.SetTitle("Keybindings — Esc to close")
	ov := &overlayHandle{
		content:       help,
		focus:         true,
		dimBackground: true,
		widthPct:      70,
		heightPct:     70,
	}
	a.helpOverlay = ov
	a.mu.Unlock()
	a.host.PushOverlay(ov)
	return ov
}

func (a *ChatApp) CloseKeyHelp() {
	a.mu.Lock()
	ov := a.helpOverlay
	a.helpOverlay = nil
	a.mu.Unlock()
	if ov != nil {
		a.host.RemoveOverlay(ov)
		a.host.Focus(a.editor)
	}
}

type overlayHandle struct {
	content       core.Component
	focus         bool
	dimBackground bool
	anchor        int
	percentX      int
	percentY      int
	widthPct      int
	heightPct     int
}

func (o *overlayHandle) OverlayContent() core.Component { return o.content }
func (o *overlayHandle) SetOverlayFocus(v bool)         { o.focus = v }
func (o *overlayHandle) SetOverlayDimBackground(v bool) { o.dimBackground = v }
func (o *overlayHandle) OverlayWantsFocus() bool        { return o.focus }
func (o *overlayHandle) OverlayDimBackground() bool     { return o.dimBackground }
func (o *overlayHandle) OverlayAnchor() int             { return o.anchor }
func (o *overlayHandle) OverlayPercentX() int           { return o.percentX }
func (o *overlayHandle) OverlayPercentY() int           { return o.percentY }
func (o *overlayHandle) OverlayWidthPct() int           { return o.widthPct }
func (o *overlayHandle) OverlayHeightPct() int          { return o.heightPct }

func (a *ChatApp) onEditorSubmit(value string) {
	a.mu.Lock()
	// 当 autocomplete 激活时，先隐藏它然后正常提交。
	// 之前直接 return 会阻止 Enter 提交，而 chat_layout.Update 把 Enter
	// 转发给 autocomplete 的 SelectList，后者会 apply 当前选中项（带上 trigger
	// 前缀），导致用户输入被篡改（例如 /help → //help），斜杠命令失效。
	if a.ac != nil {
		a.ac.Hide()
	}
	onSubmit := a.cfg.OnSubmit
	ctx := a.cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Unlock()

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	// PrintUser / PushInputHistory / SetValue operate on history and editor
	// components, which own their own internal locks — they do not touch
	// ChatApp.model, so holding ChatApp.mu here is unnecessary and would
	// serialize with the event loop for no benefit.
	a.PrintUser(trimmed)
	a.editor.PushInputHistory(trimmed)
	a.editor.SetValue("")
	if onSubmit != nil {
		onSubmit(ctx, trimmed)
	}
}

func (a *ChatApp) onAgentStart(e ChatEvent) {
	if _, ok := e.(AgentStartChatEvent); !ok {
		return
	}
	a.mu.Lock()
	a.model.StreamID = ""
	a.mu.Unlock()
	a.Busy("thinking...")
}

func (a *ChatApp) onMessageDelta(e ChatEvent) {
	d, ok := e.(MessageDeltaChatEvent)
	if !ok {
		return
	}
	// Read-modify-write StreamID under a single critical section. The
	// previous code released the lock between reading StreamID and writing
	// the new one, so two concurrent deltas could both read the same old id
	// and both append to the same baseline — corrupting the stream.
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.model.StreamID
	newID := a.history.AppendDelta(id, d.Delta)
	if newID != id {
		a.model.StreamID = newID
	}
}

func (a *ChatApp) onToolStart(e ChatEvent) {
	tc, ok := e.(ToolCallStartChatEvent)
	if !ok {
		return
	}
	// 集成模式下跳过 transfer_to_* 工具调用，不在 UI 中显示
	if a.cfg.SuppressHandoffToolDisplay && strings.HasPrefix(tc.ToolCall.Name, "transfer_to_") {
		return
	}
	a.mu.Lock()
	a.model.ActiveTools[tc.ToolCall.ID] = time.Now()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.history.Append(ChatMessage{
		ID:   "tool-" + tc.ToolCall.ID,
		Role: RoleTool,
		Meta: tc.ToolCall.Name,
		Text: theme.CurrentPalette().Dim.Render("..."),
	})
}

func (a *ChatApp) onToolEnd(e ChatEvent) {
	tc, ok := e.(ToolCallEndChatEvent)
	if !ok {
		return
	}
	// 集成模式下跳过 transfer_to_* 工具调用
	if a.cfg.SuppressHandoffToolDisplay && strings.HasPrefix(tc.ToolName, "transfer_to_") {
		return
	}
	a.mu.Lock()
	delete(a.model.ActiveTools, tc.ToolCallID)
	a.mu.Unlock()

	status := theme.CurrentPalette().Success.Render(theme.SymbolCheck + " done")
	dur := time.Duration(0)
	if a.cfg.ShowTimings {
		dur = tc.Duration
	}
	if tc.Err != nil {
		errMsg := tc.Err.Error()
		if core.VisibleWidth(errMsg) > 180 {
			errMsg = core.TruncateToWidth(errMsg, 177, "...")
		}
		status = theme.CurrentPalette().Error.Render(theme.SymbolCross + " failed: " + errMsg)
	}
	toolID := "tool-" + tc.ToolCallID
	collapsed := len(status) > 120
	if !a.history.PatchMessage(toolID, func(m *ChatMessage) {
		m.Text = status
		m.Duration = dur
		m.Collapsed = collapsed
	}) {
		a.history.Append(ChatMessage{
			Role:      RoleTool,
			Meta:      tc.ToolName,
			Text:      status,
			Duration:  dur,
			Collapsed: collapsed,
		})
	}

	// Automatically show diff blocks for edit/file-writing tools (inline, collapsed).
	if tc.Err == nil && tc.Result != "" && isEditorTool(tc.ToolName) {
		filePath, diffText, added, removed, fileContent := extractToolDiff(tc.ToolName, tc.Result)
		if diffText != "" || filePath != "" {
			var parts []string
			if filePath != "" {
				switch {
				case (tc.ToolName == "write_file" || tc.ToolName == "write") && fileContent != "":
					// Write tool: show content preview.
					parts = append(parts, fmt.Sprintf("⌨  Wrote %d lines to %s", added, filePath), formatContentPreview(fileContent, added))
				default:
					summary := fmt.Sprintf("✏️ %s", filePath)
					if added > 0 || removed > 0 {
						if added > 0 {
							summary += " " + theme.CurrentPalette().Success.Render(fmt.Sprintf("+%d", added))
						}
						if removed > 0 {
							summary += " " + theme.CurrentPalette().Error.Render(fmt.Sprintf("-%d", removed))
						}
					}
					parts = append(parts, summary)
				}
			}
			if diffText != "" {
				parts = append(parts, "```diff", diffText, "```")
			}
			if len(parts) > 0 {
				a.history.Append(ChatMessage{
					Role:      RoleAssistant,
					Meta:      "diff",
					Text:      strings.Join(parts, "\n"),
					Collapsed: false,
				})
			}
		}
	}
}

var editorTools = map[string]bool{
	"edit_block":  true,
	"edit":        true,
	"write_file":  true,
	"write":       true,
	"apply_patch": true,
}

func isEditorTool(name string) bool {
	return editorTools[name]
}

func extractToolDiff(toolName, resultJSON string) (path, diff string, added, removed int64, content string) {
	var raw struct {
		Path     string `json:"path"`
		Diff     string `json:"diff"`
		Patch    string `json:"patch"`
		OldLines int64  `json:"old_lines"`
		NewLines int64  `json:"new_lines"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &raw); err != nil {
		return "", "", 0, 0, ""
	}

	path = raw.Path
	added = raw.NewLines
	removed = raw.OldLines
	content = raw.Content

	switch toolName {
	case "edit_block", "edit":
		// Prefer unified patch, fall back to full diff.
		diff = raw.Patch
		if diff == "" {
			diff = raw.Diff
		}
	case "apply_patch":
		diff = raw.Patch
		if diff == "" {
			diff = raw.Diff
		}
		if diff != "" {
			if added == 0 && removed == 0 {
				for _, line := range strings.Split(diff, "\n") {
					if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
						added++
					} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
						removed++
					}
				}
			}
		}
	case "write_file", "write":
		if content != "" {
			added = int64(strings.Count(content, "\n")) + 1
		}
		return path, "", added, removed, content
	}
	return path, diff, added, removed, content
}

// displayFileSearchResult parses and displays search_project_files results.
func (a *ChatApp) displayFileSearchResult(resultJSON string) {
	var raw struct {
		Files []struct {
			Path      string  `json:"path"`
			Category  string  `json:"category"`
			Relevance float64 `json:"relevance"`
			Preview   string  `json:"preview"`
		} `json:"files"`
		Total   int    `json:"total"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &raw); err != nil {
		return
	}
	if len(raw.Files) == 0 {
		if raw.Message != "" {
			a.history.Append(ChatMessage{Role: RoleSystem, Text: raw.Message})
		}
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📁 案件文件搜索结果（共 %d 项）\n\n", len(raw.Files))
	pal := theme.CurrentPalette()
	for i, f := range raw.Files {
		relPath := f.Path
		cat := f.Category
		if cat == "" {
			cat = "?"
		}
		score := f.Relevance
		scoreStr := fmt.Sprintf("%.0f%%", score*100)
		preview := f.Preview
		if core.VisibleWidth(preview) > 80 {
			preview = core.TruncateToWidth(preview, 77, "...")
		}
		fmt.Fprintf(&sb, "%d. %s  [%s] (%s)\n", i+1, pal.Bold.Render(relPath), cat, pal.Dim.Render(scoreStr))
		if preview != "" {
			fmt.Fprintf(&sb, "   %s\n", pal.Dim.Render(preview))
		}
		sb.WriteString("\n")
	}

	collapsed := !a.isRunning()
	a.history.Append(ChatMessage{
		Role:      RoleAssistant,
		Meta:      "file_search",
		Text:      strings.TrimSpace(sb.String()),
		Collapsed: collapsed,
	})
}

// displayFileReadResult parses and displays read_project_file results.
func (a *ChatApp) displayFileReadResult(resultJSON string) {
	var raw struct {
		Path       string            `json:"path"`
		Category   string            `json:"category"`
		Content    string            `json:"content"`
		Confidence float64           `json:"confidence"`
		CostNotice string            `json:"cost_notice"`
		Error      string            `json:"error"`
		Metadata   map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &raw); err != nil {
		return
	}

	if raw.Error != "" {
		a.history.Append(ChatMessage{Role: RoleSystem, Text: "⚠ " + raw.Error})
		return
	}
	pathLabel := raw.Path
	if pathLabel == "" {
		pathLabel = "（未知文件）"
	}
	if raw.Content == "" && raw.CostNotice != "" {
		a.history.Append(ChatMessage{
			Role: RoleSystem,
			Text: "📄 " + pathLabel + " — " + raw.CostNotice,
		})
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📄 %s", raw.Path)

	if raw.Confidence < 1.0 {
		fmt.Fprintf(&sb, "  [置信度: %.0f%%]", raw.Confidence*100)
	}
	sb.WriteString("\n\n")

	// Show a preview of the content (first 2000 chars).
	content := raw.Content
	if core.VisibleWidth(content) > 2000 {
		content = core.TruncateToWidth(content, 1997, "...") + "\n\n[内容较长，已截断]"
	}
	sb.WriteString(content)

	collapsed := !a.isRunning()
	a.history.Append(ChatMessage{
		Role:      RoleAssistant,
		Meta:      "file_read",
		Text:      strings.TrimSpace(sb.String()),
		Collapsed: collapsed,
	})
}

func formatContentPreview(content string, totalLines int64) string {
	const previewLines = 6
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i >= previewLines {
			break
		}
		b.WriteString("  ")
		b.WriteString(line)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	if int64(len(lines)) > previewLines {
		fmt.Fprintf(&b, "\n  ... +%d lines", int64(len(lines))-previewLines)
	}
	return b.String()
}

func (a *ChatApp) onHandoffStart(e ChatEvent) {
	h, ok := e.(HandoffStartChatEvent)
	if !ok {
		return
	}
	if h.Invisible {
		return // 不可见交接：不在 UI 中显示
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.history.Append(ChatMessage{
		Role: RoleSystem,
		Text: fmt.Sprintf("%s 已切换至 %s", theme.SymbolArrow, h.TargetAgent),
	})
}

func (a *ChatApp) onHandoffEnd(e ChatEvent) {
	h, ok := e.(HandoffEndChatEvent)
	if !ok {
		return
	}
	if h.Invisible {
		// 不可见交接：只清理流状态，不显示结束公告
		a.mu.Lock()
		a.finalizeStreamLocked()
		a.mu.Unlock()
		return
	}
	if h.Err != nil {
		a.history.Append(ChatMessage{
			Role: RoleError,
			Text: fmt.Sprintf("%s 交接失败: %s", h.TargetAgent, h.Err.Error()),
		})
		return
	}
	meta := h.TargetAgent + " 已完成"
	if a.cfg.ShowTimings {
		meta += fmt.Sprintf(" (%s)", h.Duration.Round(time.Millisecond))
	}
	a.history.Append(ChatMessage{Role: RoleSystem, Text: theme.SymbolCheck + " " + meta})
}

func (a *ChatApp) onTurnStart(e ChatEvent) {
	t, ok := e.(TurnStartChatEvent)
	if !ok {
		return
	}
	if a.cfg.ShowTurns && t.Turn > 1 {
		a.history.Append(ChatMessage{
			Role: RoleDivider,
			Text: fmt.Sprintf("turn %d", t.Turn),
		})
	}
}

func (a *ChatApp) onTurnEnd(e ChatEvent) {
	// Collapse consecutive tool messages now that the turn is complete.
	// (Token-usage display, if enabled, is owned by the StatusBar via its
	// own event subscription; this hook no longer forwards raw numbers to
	// a config callback. Consumers needing the numbers should subscribe to
	// ChatEventTurnEnd directly.)
	a.history.CollapseConsecutiveTools()
}

func (a *ChatApp) onCompactionStart(e ChatEvent) {
	ev, ok := e.(CompactionStartChatEvent)
	if !ok {
		return
	}
	a.Busy(fmt.Sprintf("compacting context (%d tokens)...", ev.TokensBefore))
}

func (a *ChatApp) onCompactionEnd(e ChatEvent) {
	ev, ok := e.(CompactionEndChatEvent)
	if !ok {
		return
	}
	a.history.Append(ChatMessage{
		Role: RoleSystem,
		Text: fmt.Sprintf("%s compacted %d %s %d tokens (-%d msgs, %s)",
			theme.SymbolCheck, ev.TokensBefore, theme.SymbolArrow, ev.TokensAfter,
			ev.MessagesCut, ev.Duration.Round(time.Millisecond)),
	})
}

func (a *ChatApp) onAutoRetry(e ChatEvent) {
	r, ok := e.(AutoRetryChatEvent)
	if !ok {
		return
	}
	if a.SuppressAutoRetry {
		return
	}
	a.history.Append(ChatMessage{
		Role: RoleSystem,
		Text: fmt.Sprintf("%s retry %d/%d in %s",
			theme.SymbolWarning, r.Attempt, r.MaxRetries, r.Delay.Round(time.Millisecond)),
	})
}

func (a *ChatApp) onAgentError(e ChatEvent) {
	ev, ok := e.(AgentErrorChatEvent)
	if !ok {
		return
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.Idle()
	a.PrintError(ev.Err)
}

func (a *ChatApp) onAgentEnd(e ChatEvent) {
	if _, ok := e.(AgentEndChatEvent); !ok {
		return
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.Idle()
}

func (a *ChatApp) finalizeStreamLocked() {
	id := a.model.StreamID
	a.model.StreamID = ""
	if id != "" {
		a.history.Finalize(id)
	}
}

func (a *ChatApp) TerminalSize() (cols, rows int64) {
	if a.host != nil {
		return a.host.TerminalSize()
	}
	return 80, 24
}

type layoutHost interface {
	TerminalSize() (cols, rows int64)
}

// editorFrame wraps the editor with a horizontal top/bottom border. It exists
// so the editor border can participate in the declarative Flex layout.
type editorFrame struct {
	editor core.Component
}

func (f *editorFrame) Render(width int64) []string {
	lines := f.editor.Render(width)
	border := theme.CurrentPalette().Border.Render(strings.Repeat("─", int(width)))
	out := make([]string, 0, len(lines)+2)
	out = append(out, border)
	out = append(out, lines...)
	out = append(out, border)
	return out
}

func (f *editorFrame) Invalidate() {}

type chatLayout struct {
	host         layoutHost
	app          *ChatApp
	header       core.Component
	history      *ChatHistory
	loader       *component.Loader
	editor       core.Component
	statusBar    *component.StatusBar
	footer       core.Component
	ac           *component.Autocomplete
	lastRows     int64
	headerHeight int
	// editorTop is the absolute screen row of the editor's top border, as
	// computed by the most recent Render call. Used to translate MouseMsg
	// screen coordinates into the editor's own row space (see Update).
	editorTop int64
}

type textSelectionComponent interface {
	GetSelectedText() string
	ClearSelection()
}

func (l *chatLayout) Render(width int64) []string {
	var rows int64
	if l.host != nil {
		_, rows = l.host.TerminalSize()
	}
	if rows <= 0 {
		rows = l.lastRows
	}
	if rows <= 0 {
		rows = 24
	}
	l.lastRows = rows

	// Use a fixed terminal-size override for this render so Flex can compute
	// the Fill allocation correctly even when the host reports a different size
	// due to timing. The real terminal size is read from l.host in the first
	// branch above; this wrapper just makes sure Flex sees a consistent height.
	bounds := &fixedBounds{width: width, height: rows}

	flex := layout.NewFlex(layout.DirectionVertical)
	flex.Bounds = bounds

	headerIndex := -1
	editorFrameIndex := -1

	if l.header != nil {
		headerIndex = len(flex.Children)
		flex.AddChild(layout.Natural(l.header))
	}
	if l.history != nil {
		flex.AddChild(layout.FillWeight(l.history, 1).WithAllocate(func(h int64) {
			l.history.SetMaxRowsDirect(h)
		}))
	}
	if l.ac != nil && l.ac.Active() {
		flex.AddChild(layout.Natural(l.ac))
	}
	if l.loader != nil && l.loader.IsRunning() {
		flex.AddChild(layout.Natural(l.loader))
	}
	editorFrame := &editorFrame{editor: l.editor}
	editorFrameIndex = len(flex.Children)
	flex.AddChild(layout.Natural(editorFrame))
	if l.footer != nil {
		flex.AddChild(layout.Natural(l.footer))
	}
	if l.statusBar != nil {
		flex.AddChild(layout.Natural(l.statusBar))
	}

	out := flex.Render(width)

	if headerIndex >= 0 {
		l.headerHeight = int(flex.ChildRect(headerIndex).Height)
	}
	if editorFrameIndex >= 0 {
		l.editorTop = flex.ChildRect(editorFrameIndex).Row
	}
	return out
}

type fixedBounds struct {
	width, height int64
}

func (b *fixedBounds) TerminalSize() (cols, rows int64) {
	return b.width, b.height
}

func (l *chatLayout) Invalidate() {}

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

func doCopyToClipboard(l *chatLayout, text string) {
	go func() {
		if err := CopyToClipboard(text); err != nil {
			l.app.PrintError(fmt.Errorf("clipboard: %w", err))
		} else {
			truncated := text
			if core.VisibleWidth(truncated) > 60 {
				truncated = core.TruncateToWidth(truncated, 57, "...")
			}
			l.app.PrintSystem("📋 已复制: " + truncated)
		}
	}()
}

// hasSelection reports whether the editor or chat history currently has a
// non-empty text selection, without clearing it. Used to decide whether
// Ctrl/Cmd+C should copy instead of interrupting the running agent.
func hasSelection(l *chatLayout) bool {
	if sel, ok := l.editor.(textSelectionComponent); ok && sel.GetSelectedText() != "" {
		return true
	}
	return l.history.GetSelectedText() != ""
}

func isPrimaryShortcutMod(mods terminal.Modifier) bool {
	return mods&terminal.ModCtrl != 0 || mods&terminal.ModSuper != 0 || mods&terminal.ModMeta != 0
}

func isCopyShortcut(k terminal.Key) bool {
	name := strings.ToLower(k.Name)
	if name == "c" {
		return isPrimaryShortcutMod(k.Mods)
	}
	if name == "insert" {
		return k.Mods&terminal.ModCtrl != 0 || k.Mods&terminal.ModShift != 0
	}
	return false
}

func (l *chatLayout) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.WindowSizeMsg:
		l.lastRows = m.Height
		l.recalcMaxRows(m.Width, m.Height)
	case core.PasteMsg:
		// Image paste detection: when clipboard has an image, the terminal
		// sends empty/short text.  Let the caller hook pasteImageFn.
		if m.Text == "" || (len(m.Text) < 4 && m.Text == "\r") {
			if l.app.cfg.OnImagePaste != nil {
				l.app.cfg.OnImagePaste()
				return nil
			}
		}
	}
	if l.history != nil {
		switch m := msg.(type) {
		case core.MouseMsg:
			// Right-click (Button 2) → copy selected text.
			if m.Action == core.MouseRelease && m.Button == 2 {
				doCopy(l)
				return nil
			}
			adjusted := m
			adjusted.Row -= int64(l.headerHeight)
			if adjusted.Row >= 0 {
				l.history.Update(adjusted)
			}
			// Also forward to the editor (row-adjusted for its own screen
			// position, skipping its top border row) so click-drag text
			// selection works inside the input box. Mouse mode itself stays
			// globally enabled — the editor bounds-checks and ignores events
			// outside its own rendered rows.
			//
			// Forwarded unconditionally (not gated on the adjusted row being
			// in range): once a drag starts, the mouse can easily drift onto
			// the editor's border row or outside it entirely before the
			// button is released. If MouseRelease were dropped here because
			// the row briefly fell out of range, the editor would be stuck
			// with selDragging=true forever, and every later mouse move
			// (terminals report motion regardless of button state) would
			// keep extending a phantom selection. The editor's own
			// hitTestLocked/selDragging checks already validate press/motion
			// positions, so forwarding everything is safe.
			if upd, ok := l.editor.(core.Updatable); ok {
				editorAdjusted := m
				editorAdjusted.Row -= l.editorTop + 1
				upd.Update(editorAdjusted)
			}
		case core.KeyMsg:
			for _, k := range terminal.ParseKeys(m.Data) {
				name := strings.ToLower(k.Name)
				switch name {
				case "v":
					if isPrimaryShortcutMod(k.Mods) &&
						k.Mods&terminal.ModAlt != 0 {
						// Ctrl+Alt+V / Cmd+Alt+V → image paste
						if l.app.cfg.OnImagePaste != nil {
							l.app.cfg.OnImagePaste()
						}
						return nil
					}
				case "escape":
					if l.ac != nil && l.ac.Active() {
						l.ac.Hide()
						// File browser: ESC navigates up one level.
						value := l.app.editor.GetValue()
						if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
							len(value) > len("@file:") {
							newValue := popLastPathSegment(value)
							l.app.editor.SetValue(newValue)
							l.app.skipRefresh = false
							l.ac.Refresh(newValue, int64(len(newValue)))
						}
						return nil
					}
				case "pageUp":
					l.history.ScrollBy(-5)
				case "pageDown":
					l.history.ScrollBy(5)
				case "c", "insert":
					if isCopyShortcut(k) {
						// Prefer copying an active selection over interrupting.
						// This matters because many terminals don't report a
						// distinguishable Cmd modifier (no Kitty keyboard protocol
						// support) and send Cmd+C as the same byte sequence as
						// Ctrl+C. Without this check, Cmd+C while the agent is
						// running would interrupt it instead of copying the text
						// the user just selected.
						if hasSelection(l) {
							doCopy(l)
							return nil
						}
						// Ctrl+C while agent is running: interrupt
						if k.Mods&terminal.ModCtrl != 0 && l.app.cfg.OnInterrupt != nil && l.app.isRunning() {
							l.app.cfg.OnInterrupt()
							return nil
						}
						doCopy(l)
						return nil
					}
				}
			}
		}
	}
	if l.statusBar != nil {
		l.statusBar.Update(msg)
	}
	if l.ac != nil && l.ac.Active() {
		if _, ok := msg.(core.KeyMsg); ok {
			l.ac.Update(msg)
		}
	}
	return nil
}

func (l *chatLayout) recalcMaxRows(width, height int64) {
	var headerH, loaderH, editorH, footerH, statusH, acH int64
	if l.header != nil {
		headerH = int64(len(l.header.Render(width)))
	}
	if l.editor != nil {
		editorH = int64(len(l.editor.Render(width))) + 2
	}
	if l.loader != nil && l.loader.IsRunning() {
		loaderH = int64(len(l.loader.Render(width)))
	}
	if l.footer != nil {
		footerH = int64(len(l.footer.Render(width)))
	}
	if l.statusBar != nil {
		statusH = int64(len(l.statusBar.Render(width)))
	}
	if l.ac != nil && l.ac.Active() {
		acH = int64(len(l.ac.Render(width)))
	}
	reserved := headerH + editorH + loaderH + footerH + statusH + acH
	remaining := height - reserved
	if remaining < 1 {
		remaining = 1
	}
	if l.history != nil {
		l.history.SetMaxRows(remaining)
	}
}

// popLastPathSegment removes the trailing directory or file name from a value
// like "@file:cmd/mady/" → "@file:cmd/" or "@file:main.go" → "@file:".
func popLastPathSegment(value string) string {
	// Strip trailing slash if present, then remove the last segment.
	trimmed := strings.TrimSuffix(value, "/")
	idx := strings.LastIndexAny(trimmed, "/:")
	if idx < 0 {
		return value
	}
	return trimmed[:idx+1]
}
