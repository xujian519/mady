package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/xujian519/mady/agentcore"
)

func (s *AgentStore) openOrCreate(ctx context.Context, key string) (*Manager, error) {
	mgr, err := s.sessions.Open(ctx, key)
	if err == nil {
		return mgr, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open session %q: %w", key, err)
	}
	mgr, err = s.sessions.Create(ctx, CreateOptions{ID: key, Cwd: s.cwd})
	if err != nil {
		return nil, fmt.Errorf("create session %q: %w", key, err)
	}
	return mgr, nil
}

func (s *AgentStore) syncMessages(ctx context.Context, mgr *Manager, want []agentcore.Message) (*Manager, error) {
	have := mgr.MessagesOnPath()
	if len(have) > len(want) || !messagesHavePrefix(want, have) {
		return s.rewriteSession(ctx, mgr, want)
	}
	for _, msg := range want[len(have):] {
		if err := mgr.AppendMessage(ctx, msg); err != nil {
			return nil, fmt.Errorf("append session message: %w", err)
		}
	}
	return mgr, nil
}

func (s *AgentStore) rewriteSession(ctx context.Context, prev *Manager, msgs []agentcore.Message) (*Manager, error) {
	header := prev.Header()
	threadCfg, threadCfgSet := latestThreadConfig(prev)

	// Create the new session first (overwrites the old file in place),
	// then purge the stale lock — this avoids the data-loss window that
	// would exist if we deleted before creating.
	mgr, err := s.sessions.Create(ctx, CreateOptions{
		ID:            header.ID,
		Cwd:           header.Cwd,
		ParentSession: header.ParentSession,
	})
	if err != nil {
		return nil, fmt.Errorf("recreate diverged session: %w", err)
	}

	// File already created — safe to purge the stale lock without a delete.
	s.sessions.lockCleanup(header.ID)

	for _, msg := range msgs {
		if err := mgr.AppendMessage(ctx, msg); err != nil {
			return nil, fmt.Errorf("rewrite session message: %w", err)
		}
	}
	if threadCfgSet {
		if err := appendThreadConfig(ctx, mgr, threadCfg); err != nil {
			return nil, err
		}
	}
	return mgr, nil
}

func latestAgentState(mgr *Manager) (agentStateData, bool) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	for i := len(mgr.entries) - 1; i >= 0; i-- {
		entry := mgr.entries[i]
		if entry.Type != EntryCustom {
			continue
		}
		var data agentStateData
		if json.Unmarshal(entry.Data, &data) == nil && data.Kind == agentStateEntryKind {
			return data, true
		}
	}
	return agentStateData{}, false
}

func buildThreadSnapshot(mgr *Manager) *ThreadSnapshot {
	snap := agentStateFromManager(mgr)
	cfg, _ := latestThreadConfig(mgr)
	return &ThreadSnapshot{
		Info:       mgr.Info(),
		Messages:   snap.Messages,
		Transcript: threadMessagesOnPath(mgr),
		Status:     snap.Status,
		Turn:       snap.Turn,
		TotalUsage: snap.TotalUsage,
		Config:     agentcore.CloneCallConfig(cfg),
		Thinking:   agentcore.CloneThinkingConfig(configThinking(cfg)),
	}
}

func agentStateFromManager(mgr *Manager) agentcore.StateSnapshot {
	snap := agentcore.StateSnapshot{
		Messages: mgr.MessagesOnPath(),
	}
	if meta, ok := latestAgentState(mgr); ok {
		snap.Status = meta.Status
		snap.Turn = meta.Turn
		snap.TotalUsage = meta.TotalUsage
	} else if len(snap.Messages) > 0 {
		snap.Status = agentcore.StatusFinished
	} else {
		snap.Status = agentcore.StatusIdle
	}
	return snap
}

func threadMessagesOnPath(mgr *Manager) []ThreadMessage {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	path := mgr.pathToLeaf()
	var transcript []ThreadMessage
	var lastCompaction *CompactionData
	var lastCompactionEntryID string

	for _, entry := range path {
		if entry.Type == EntryCompaction {
			var cd CompactionData
			if json.Unmarshal(entry.Data, &cd) == nil {
				lastCompaction = &cd
				lastCompactionEntryID = entry.ID
			}
		}
	}

	skipUntil := ""
	if lastCompaction != nil {
		skipUntil = lastCompaction.FirstKeptEntryID
		transcript = append(transcript, ThreadMessage{
			EntryID: lastCompactionEntryID,
			Message: agentcore.Message{
				Role:    agentcore.RoleSystem,
				Content: lastCompaction.Summary,
				Type:    agentcore.MessageTypeCompactionSummary,
			},
		})
	}

	skipping := skipUntil != ""
	for _, entry := range path {
		if skipping {
			if entry.ID == skipUntil {
				skipping = false
			} else {
				continue
			}
		}

		switch entry.Type {
		case EntryMessage:
			var msg agentcore.Message
			if json.Unmarshal(entry.Data, &msg) == nil {
				transcript = append(transcript, ThreadMessage{
					EntryID: entry.ID,
					Message: msg,
				})
			}
		case EntryBranchSummary:
			var bs BranchSummaryData
			if json.Unmarshal(entry.Data, &bs) == nil {
				transcript = append(transcript, ThreadMessage{
					EntryID: entry.ID,
					Message: agentcore.Message{
						Role:    agentcore.RoleSystem,
						Content: bs.Summary,
						Type:    agentcore.MessageTypeBranchSummary,
					},
				})
			}
		}
	}

	return transcript
}

func messagesHavePrefix(full, prefix []agentcore.Message) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i := range prefix {
		if !messagesEqual(full[i], prefix[i]) {
			return false
		}
	}
	return true
}

// messagesEqual compares two agentcore.Message values field by field.
// reflect.DeepEqual is used only for the complex Metadata/Blocks fields.
func messagesEqual(a, b agentcore.Message) bool {
	if a.ID != b.ID ||
		a.Role != b.Role ||
		a.Content != b.Content ||
		a.ToolCallID != b.ToolCallID ||
		a.Name != b.Name ||
		a.Type != b.Type ||
		a.InvocationID != b.InvocationID {
		return false
	}
	if len(a.ToolCalls) != len(b.ToolCalls) {
		return false
	}
	for i := range a.ToolCalls {
		at, bt := a.ToolCalls[i], b.ToolCalls[i]
		if at.ID != bt.ID || at.Name != bt.Name || at.Arguments != bt.Arguments {
			return false
		}
	}
	if !cacheControlEqual(a.CacheControl, b.CacheControl) {
		return false
	}
	if !reflect.DeepEqual(a.Metadata, b.Metadata) {
		return false
	}
	if !reflect.DeepEqual(a.Blocks, b.Blocks) {
		return false
	}
	return true
}

func cacheControlEqual(a, b *agentcore.CacheControlMarker) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Type == b.Type
}

func latestThreadConfig(mgr *Manager) (*agentcore.CallConfig, bool) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	for i := len(mgr.entries) - 1; i >= 0; i-- {
		entry := mgr.entries[i]
		if entry.Type != EntryCustom {
			continue
		}
		var cfgData threadConfigData
		if json.Unmarshal(entry.Data, &cfgData) == nil && cfgData.Kind == threadConfigEntryKind {
			return cfgData.Config, true
		}
		var data threadThinkingData
		if json.Unmarshal(entry.Data, &data) == nil && data.Kind == threadThinkingEntryKind {
			return &agentcore.CallConfig{Thinking: agentcore.CloneThinkingConfig(data.Thinking)}, true
		}
	}
	return nil, false
}

func appendThreadConfig(ctx context.Context, mgr *Manager, cfg *agentcore.CallConfig) error {
	meta, err := json.Marshal(threadConfigData{
		Kind:   threadConfigEntryKind,
		Config: agentcore.CloneCallConfig(cfg),
	})
	if err != nil {
		return fmt.Errorf("marshal thread config: %w", err)
	}
	if err := mgr.Append(ctx, Entry{Type: EntryCustom, Data: meta}); err != nil {
		return fmt.Errorf("append thread config: %w", err)
	}
	return nil
}

func configThinking(cfg *agentcore.CallConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	return cfg.Thinking
}

var _ agentcore.Store = (*AgentStore)(nil)
