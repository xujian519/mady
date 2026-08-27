package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/xujian519/mady/agentcore"
)

const agentStateEntryKind = "agent_state"
const threadThinkingEntryKind = "thread_thinking"
const threadConfigEntryKind = "thread_config"

type agentStateData struct {
	Kind       string               `json:"kind"`
	Status     agentcore.Status     `json:"status"`
	Turn       int64                `json:"turn"`
	TotalUsage agentcore.TokenUsage `json:"total_usage"`
}

type threadThinkingData struct {
	Kind     string                    `json:"kind"`
	Thinking *agentcore.ThinkingConfig `json:"thinking,omitempty"`
}

type threadConfigData struct {
	Kind   string                `json:"kind"`
	Config *agentcore.CallConfig `json:"config,omitempty"`
}

// AgentStore adapts session.FileStore to agentcore.Store so thread state can be
// persisted as append-only JSONL sessions while preserving message history.
type AgentStore struct {
	sessions *FileStore
	cwd      string
}

// ThreadMessage is one transcript item with a stable entry id for branching.
type ThreadMessage struct {
	EntryID string            `json:"entry_id,omitempty"`
	Message agentcore.Message `json:"message"`
}

// ThreadSnapshot is a thread-oriented view over a persisted agent session.
type ThreadSnapshot struct {
	Info       Info                      `json:"info"`
	Messages   []agentcore.Message       `json:"messages"`
	Transcript []ThreadMessage           `json:"transcript,omitempty"`
	Status     agentcore.Status          `json:"status"`
	Turn       int64                     `json:"turn"`
	TotalUsage agentcore.TokenUsage      `json:"total_usage"`
	Config     *agentcore.CallConfig     `json:"config,omitempty"`
	Thinking   *agentcore.ThinkingConfig `json:"thinking,omitempty"`
}

// NewAgentStore wraps a session FileStore for agent state persistence.
func NewAgentStore(sessions *FileStore, cwd string) *AgentStore {
	return &AgentStore{sessions: sessions, cwd: cwd}
}

// Save persists an agent state snapshot into the session store.
func (s *AgentStore) Save(ctx context.Context, key string, snap agentcore.StateSnapshot) error {
	mgr, err := s.openOrCreate(ctx, key)
	if err != nil {
		// 打开或创建会话失败，向上层传播错误
		return err
	}
	threadCfg, threadCfgSet := latestThreadConfig(mgr)

	mgr, err = s.syncMessages(ctx, mgr, snap.Messages)
	if err != nil {
		// 同步消息失败，向上层传播错误
		return err
	}
	if threadCfgSet {
		currentCfg, currentSet := latestThreadConfig(mgr)
		if !currentSet || !currentCfg.Equal(threadCfg) {
			if err := appendThreadConfig(ctx, mgr, threadCfg); err != nil {
				return err
			}
		}
	}

	meta, err := json.Marshal(agentStateData{
		Kind:       agentStateEntryKind,
		Status:     snap.Status,
		Turn:       snap.Turn,
		TotalUsage: snap.TotalUsage,
	})
	if err != nil {
		return fmt.Errorf("marshal agent state: %w", err)
	}

	if err := mgr.Append(ctx, Entry{Type: EntryCustom, Data: meta}); err != nil {
		return fmt.Errorf("append agent state: %w", err)
	}
	return nil
}

// Load retrieves an agent state snapshot from the session store.
func (s *AgentStore) Load(ctx context.Context, key string) (agentcore.StateSnapshot, error) {
	mgr, err := s.sessions.Open(ctx, key)
	if err != nil {
		return agentcore.StateSnapshot{}, fmt.Errorf("open agent session: %w", err)
	}

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

	return snap, nil
}

// Delete removes an agent session by key.
func (s *AgentStore) Delete(ctx context.Context, key string) error {
	return s.sessions.Delete(ctx, key)
}

// RenameThread 重命名会话（写入 session_info 元数据 name，readInfo/Info 自动可见）。
func (s *AgentStore) RenameThread(ctx context.Context, key string, name string) error {
	if key == "" {
		return fmt.Errorf("agent store: key is required")
	}
	if name == "" {
		return fmt.Errorf("agent store: name is required")
	}
	mgr, err := s.sessions.Open(ctx, key)
	if err != nil {
		return fmt.Errorf("open agent session for rename: %w", err)
	}
	if err := mgr.SetSessionName(ctx, name); err != nil {
		return fmt.Errorf("set session name: %w", err)
	}
	if err := mgr.flushAll(); err != nil {
		return fmt.Errorf("persist session name: %w", err)
	}
	return nil
}

// TrashThread 将会话移入回收站（软删除；阶段 1.4 回收站能力）。
func (s *AgentStore) TrashThread(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("agent store: key is required")
	}
	return s.sessions.MoveToTrash(ctx, key)
}

// RestoreThread 将回收站中的会话恢复回主目录。
func (s *AgentStore) RestoreThread(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("agent store: key is required")
	}
	return s.sessions.RestoreFromTrash(ctx, key)
}

// ListTrashedThreads 列出回收站中的会话（按更新时间倒序）。
func (s *AgentStore) ListTrashedThreads(ctx context.Context) ([]Info, error) {
	return s.sessions.ListTrashed(ctx)
}

// PurgeThread 从回收站彻底删除会话（不可恢复）。
func (s *AgentStore) PurgeThread(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("agent store: key is required")
	}
	return s.sessions.PurgeTrashed(ctx, key)
}

// Has checks whether an agent session exists by key.
func (s *AgentStore) Has(ctx context.Context, key string) (bool, error) {
	return s.sessions.Has(ctx, key)
}

// List returns all agent session keys.
func (s *AgentStore) List(ctx context.Context) ([]string, error) {
	info, err := s.sessions.List(ctx)
	if err != nil {
		// 列出会话失败，向上层传播错误
		return nil, err
	}
	keys := make([]string, 0, len(info))
	for _, item := range info {
		keys = append(keys, item.ID)
	}
	return keys, nil
}

// ListThreads returns thread metadata ordered by most recently updated first.
func (s *AgentStore) ListThreads(ctx context.Context) ([]Info, error) {
	return s.sessions.List(ctx)
}

// GetThread returns the persisted thread transcript and agent metadata.
func (s *AgentStore) GetThread(ctx context.Context, key string) (*ThreadSnapshot, error) {
	mgr, err := s.sessions.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open agent session: %w", err)
	}
	return buildThreadSnapshot(mgr), nil
}

// GetThreadConfig returns the persisted call config for a thread. The second
// return value reports whether an explicit thread-level config exists.
func (s *AgentStore) GetThreadConfig(ctx context.Context, key string) (*agentcore.CallConfig, bool, error) {
	mgr, err := s.sessions.Open(ctx, key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("open agent session: %w", err)
	}
	cfg, ok := latestThreadConfig(mgr)
	return agentcore.CloneCallConfig(cfg), ok, nil
}

// SetThreadConfig persists a thread-level config override. Passing nil clears
// the effective thread override while recording that reset in history.
func (s *AgentStore) SetThreadConfig(ctx context.Context, key string, cfg *agentcore.CallConfig) (*ThreadSnapshot, error) {
	mgr, err := s.openOrCreate(ctx, key)
	if err != nil {
		// 打开或创建会话失败，向上层传播错误
		return nil, err
	}
	if err := appendThreadConfig(ctx, mgr, cfg); err != nil {
		// 追加线程配置失败，向上层传播错误
		return nil, err
	}
	if err := mgr.flushAll(); err != nil {
		return nil, err
	}
	return buildThreadSnapshot(mgr), nil
}

// GetThreadThinking returns the persisted thinking config for a thread. The
// second return value reports whether an explicit thread-level config exists.
func (s *AgentStore) GetThreadThinking(ctx context.Context, key string) (*agentcore.ThinkingConfig, bool, error) {
	cfg, ok, err := s.GetThreadConfig(ctx, key)
	if err != nil {
		// 获取线程配置失败，向上层传播错误
		return nil, false, err
	}
	if !ok || cfg == nil {
		return nil, false, nil
	}
	return agentcore.CloneThinkingConfig(cfg.Thinking), cfg.Thinking != nil, nil
}

// SetThreadThinking persists a thread-level thinking override. Passing nil
// clears the effective thread override while recording that reset in history.
func (s *AgentStore) SetThreadThinking(ctx context.Context, key string, cfg *agentcore.ThinkingConfig) (*ThreadSnapshot, error) {
	return s.SetThreadConfig(ctx, key, &agentcore.CallConfig{
		Thinking: agentcore.CloneThinkingConfig(cfg),
	})
}

// CreateThread creates a new empty thread and returns its initial snapshot.
func (s *AgentStore) CreateThread(ctx context.Context) (*ThreadSnapshot, error) {
	mgr, err := s.sessions.Create(ctx, CreateOptions{Cwd: s.cwd})
	if err != nil {
		return nil, fmt.Errorf("create thread: %w", err)
	}
	return &ThreadSnapshot{
		Info:       mgr.Info(),
		Status:     agentcore.StatusIdle,
		Messages:   nil,
		Transcript: nil,
	}, nil
}

// SetThreadName sets the display name for a thread by appending a
// session_info entry. Returns an error if the thread does not exist.
func (s *AgentStore) SetThreadName(ctx context.Context, key, name string) error {
	mgr, err := s.sessions.Open(ctx, key)
	if err != nil {
		return fmt.Errorf("open thread %q: %w", key, err)
	}
	return mgr.SetSessionName(ctx, name)
}

// BranchThread creates a new thread from an existing thread. If entryID is empty,
// it branches from the current leaf; otherwise it branches from the given entry.
func (s *AgentStore) BranchThread(ctx context.Context, key, entryID string) (*ThreadSnapshot, error) {
	mgr, err := s.sessions.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open agent session: %w", err)
	}
	if entryID != "" {
		if err := mgr.Branch(entryID); err != nil {
			return nil, fmt.Errorf("branch thread at entry: %w", err)
		}
	}

	newID, err := mgr.CreateBranchedSession(ctx, s.sessions)
	if err != nil {
		return nil, fmt.Errorf("branch thread: %w", err)
	}
	return s.GetThread(ctx, newID)
}
