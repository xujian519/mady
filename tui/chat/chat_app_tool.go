package chat

// This file groups ChatApp event handlers for tool calls, handoffs, turns,
// auto-retry, and context compaction. It also holds the editor-tool diff
// extraction helpers (editorTools / isEditorTool / extractToolDiff /
// formatContentPreview) that turn a tool result JSON into an inline diff block.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

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
// displayFileReadResult parses and displays read_project_file results.
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
	te, ok := e.(TurnEndChatEvent)
	if !ok {
		a.history.CollapseConsecutiveTools()
		return
	}
	a.mu.Lock()
	a.model.usagePrompt += te.Usage.PromptTokens
	a.model.usageCompletion += te.Usage.CompletionTokens
	// tok/s for this turn: completion tokens / elapsed since AgentStart.
	// Guard against zero-elapsed (sub-millisecond turns) to avoid div-by-zero.
	var tokPerSec int64
	if !a.model.turnStarted.IsZero() {
		elapsed := time.Since(a.model.turnStarted).Seconds()
		if elapsed > 0 {
			tokPerSec = int64(float64(te.Usage.CompletionTokens) / elapsed)
		}
	}
	prompt := a.model.usagePrompt
	completion := a.model.usageCompletion
	a.mu.Unlock()

	if a.statusBar != nil {
		a.statusBar.SetUsage(prompt, completion, tokPerSec)
		a.host.RequestRender()
	}
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
