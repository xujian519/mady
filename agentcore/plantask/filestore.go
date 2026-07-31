package plantask

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore 是基于文件系统的 Store 实现。
// 每个会话存为 baseDir/<id>.json；写入采用"写临时文件 + rename"的原子模式，
// 防止崩溃导致数据损坏。baseDir 在构造时确保存在。
type FileStore struct {
	mu      sync.Mutex
	baseDir string
}

// NewFileStore 在 baseDir 下创建文件存储。baseDir 不存在时自动创建。
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("plantask: baseDir is empty")
	}
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, fmt.Errorf("plantask: create base dir %s: %w", baseDir, err)
	}
	return &FileStore{baseDir: baseDir}, nil
}

func (f *FileStore) path(id string) string {
	return filepath.Join(f.baseDir, id+".json")
}

// Save 原子写入会话（临时文件 + rename）。
func (f *FileStore) Save(_ context.Context, s *PlanTaskSession) error {
	if s.ID == "" {
		return fmt.Errorf("plantask: session ID is empty")
	}
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("plantask: marshal session %q: %w", s.ID, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	tmp := f.path(s.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("plantask: write temp for %q: %w", s.ID, err)
	}
	if err := os.Rename(tmp, f.path(s.ID)); err != nil {
		return fmt.Errorf("plantask: rename for %q: %w", s.ID, err)
	}
	return nil
}

// Load 按 ID 读取会话（返回 Clone）。
func (f *FileStore) Load(_ context.Context, id string) (*PlanTaskSession, error) {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return nil, fmt.Errorf("plantask: invalid session ID %q", id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := os.ReadFile(f.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plantask: session %q not found", id)
		}
		return nil, fmt.Errorf("plantask: read session %q: %w", id, err)
	}
	var s PlanTaskSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("plantask: unmarshal session %q: %w", id, err)
	}
	return s.Clone(), nil
}

// ListPending 返回所有未终态会话，按创建时间升序。
func (f *FileStore) ListPending(ctx context.Context) ([]*PlanTaskSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listFilteredLocked(ctx, func(s *PlanTaskSession) bool { return !isTerminal(s.Status) })
}

// ListByCase 返回某案件的会话，按创建时间升序。
func (f *FileStore) ListByCase(ctx context.Context, caseID string) ([]*PlanTaskSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listFilteredLocked(ctx, func(s *PlanTaskSession) bool { return s.CaseID == caseID })
}

// listFilteredLocked 扫描目录并按过滤器收集会话（调用方持锁）。
func (f *FileStore) listFilteredLocked(_ context.Context, keep func(*PlanTaskSession) bool) ([]*PlanTaskSession, error) {
	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		return nil, fmt.Errorf("plantask: list dir %s: %w", f.baseDir, err)
	}
	var out []*PlanTaskSession
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		data, err := os.ReadFile(f.path(id))
		if err != nil {
			continue // 跳过不可读文件（可能是并发清理）
		}
		var s PlanTaskSession
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		if keep(&s) {
			out = append(out, s.Clone())
		}
	}
	sortSessions(out)
	return out, nil
}

// Delete 删除会话文件。
func (f *FileStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := os.Remove(f.path(id))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("plantask: delete session %q: %w", id, err)
	}
	return nil
}
