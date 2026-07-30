package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// CompactionConfig configures automatic session compaction.
// NOTE: Compaction is currently triggered by message count only, not by
// token usage. Sessions with few but very long messages may overflow the
// model context window without triggering compaction. A token-based
// threshold (MaxTokens) should be added in a future iteration.
type CompactionConfig struct {
	// MaxMessages triggers compaction when the session exceeds this many messages.
	// Default: 50.
	MaxMessages int

	// KeepRecent is the number of most recent messages to keep uncompacted.
	// Default: 10.
	KeepRecent int

	// Enabled controls whether auto-compaction is active. Default: false.
	Enabled bool
}

// DefaultCompactionConfig returns sensible defaults for auto-compaction.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		MaxMessages: 50,
		KeepRecent:  10,
		Enabled:     false,
	}
}

// AutoCompactor monitors a session and triggers compaction when the message
// count exceeds the configured threshold. Compaction replaces older messages
// with a summary entry, keeping recent messages intact.
type AutoCompactor struct {
	config CompactionConfig
	mgr    *Manager
}

// NewAutoCompactor creates an AutoCompactor for the given session manager.
func NewAutoCompactor(mgr *Manager, config CompactionConfig) *AutoCompactor {
	if config.MaxMessages <= 0 {
		config.MaxMessages = 50
	}
	if config.KeepRecent <= 0 {
		config.KeepRecent = 10
	}
	return &AutoCompactor{
		config: config,
		mgr:    mgr,
	}
}

// CheckAndCompact examines the current session and triggers compaction
// if the message count exceeds MaxMessages. Returns true if compaction
// was performed.
func (a *AutoCompactor) CheckAndCompact(ctx context.Context) (bool, error) {
	if !a.config.Enabled {
		return false, nil
	}

	entries := a.mgr.Entries()
	msgCount := countMessages(entries)

	if msgCount <= a.config.MaxMessages {
		return false, nil
	}

	slog.Info("auto-compaction triggered",
		"message_count", msgCount,
		"threshold", a.config.MaxMessages,
		"keep_recent", a.config.KeepRecent,
	)

	// Build a summary from the older messages.
	summary := a.buildSummary(entries, msgCount)

	cd := CompactionData{
		Summary:          summary,
		FirstKeptEntryID: "", // kept entries start after the compaction
		KeptCount:        int64(a.config.KeepRecent),
	}

	if err := a.mgr.AppendCompaction(ctx, cd); err != nil {
		return false, fmt.Errorf("auto compaction: %w", err)
	}

	slog.Info("auto-compaction completed", "summary_length", len(summary))
	return true, nil
}

// countMessages counts user and assistant messages (excludes system/compaction entries).
func countMessages(entries []Entry) int {
	count := 0
	for _, e := range entries {
		switch e.Type {
		case EntryMessage:
			count++
		}
	}
	return count
}

// buildSummary creates a simple summary from the messages that will be compacted.
// Messages beyond KeepRecent are summarized into a single line per message.
func (a *AutoCompactor) buildSummary(entries []Entry, total int) string {
	if total <= a.config.KeepRecent {
		return ""
	}

	var lines []string
	compacted := 0
	for _, e := range entries {
		if e.Type != EntryMessage {
			continue
		}
		compacted++
		if compacted > total-a.config.KeepRecent {
			break
		}

		var msg agentcore.Message
		if err := json.Unmarshal(e.Data, &msg); err != nil {
			continue
		}

		role := "用户"
		if msg.Role == agentcore.RoleAssistant {
			role = "助手"
		}

		// Truncate long messages for the summary
		content := msg.Content
		if len([]rune(content)) > 100 {
			content = string([]rune(content)[:100]) + "..."
		}

		lines = append(lines, fmt.Sprintf("%s: %s", role, strings.TrimSpace(content)))
	}

	if len(lines) == 0 {
		return ""
	}

	return "对话摘要（早期消息已压缩）：\n" + strings.Join(lines, "\n")
}
