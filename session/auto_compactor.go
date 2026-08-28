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
type CompactionConfig struct {
	// MaxMessages triggers compaction when the session exceeds this many messages.
	// Default: 50.
	MaxMessages int

	// MaxTokens triggers compaction when the estimated token count of all
	// message entries exceeds this value. Sessions with few but very long
	// messages may otherwise overflow the model context window without ever
	// hitting MaxMessages. 0 disables the token trigger (message-count
	// trigger still applies). Default: 20000.
	MaxTokens int64

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
		MaxTokens:   20000,
		KeepRecent:  10,
		Enabled:     false,
	}
}

// AutoCompactor monitors a session and triggers compaction when the message
// count exceeds MaxMessages or the estimated message tokens exceed MaxTokens.
// Compaction replaces older messages with a summary entry, keeping recent
// messages intact.
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
	if config.MaxTokens < 0 {
		config.MaxTokens = 0
	}
	return &AutoCompactor{
		config: config,
		mgr:    mgr,
	}
}

// CheckAndCompact examines the current session and triggers compaction
// if the message count exceeds MaxMessages or the estimated token count
// exceeds MaxTokens. Returns true if compaction was performed.
func (a *AutoCompactor) CheckAndCompact(ctx context.Context) (bool, error) {
	if !a.config.Enabled {
		return false, nil
	}

	entries := a.mgr.Entries()
	msgCount := countMessages(entries)

	// 没有可压缩的旧消息时不动手：避免 KeepRecent 之外为空时的无效压缩
	// （token 触发下尤其重要，否则近几条长消息会反复触发空转压缩）。
	if msgCount <= a.config.KeepRecent {
		return false, nil
	}

	estTokens := estimateEntriesTokens(entries)
	tokenExceeded := a.config.MaxTokens > 0 && estTokens >= a.config.MaxTokens
	if msgCount <= a.config.MaxMessages && !tokenExceeded {
		return false, nil
	}

	slog.Info("auto-compaction triggered",
		"message_count", msgCount,
		"threshold", a.config.MaxMessages,
		"estimated_tokens", estTokens,
		"token_threshold", a.config.MaxTokens,
		"token_triggered", tokenExceeded,
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

// estimateEntriesTokens estimates the token count of all message entries via
// agentcore.EstimateMessageTokens（与 agentcore 压缩同一估算口径）。解析失败的
// 条目跳过——坏消息不应阻塞压缩判定。
func estimateEntriesTokens(entries []Entry) int64 {
	var total int64
	for _, e := range entries {
		if e.Type != EntryMessage {
			continue
		}
		var msg agentcore.Message
		if err := json.Unmarshal(e.Data, &msg); err != nil {
			continue
		}
		total += agentcore.EstimateMessageTokens(msg)
	}
	return total
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
