//go:build darwin

package main

// tabs.go — 会话标签（阶段 2.1：Go 侧 tabs 状态机第一步）。
//
// 仿照 Reasonix 桌面端（DeepSeek-Reasonix-main-v2/desktop/tabs.go）的设计：
// 标签是 Go 侧一等概念（内存为真相源 + JSON 持久化 + 启动恢复），
// 前端 TabBar 通过 ListTabs/CreateTab/CloseTab/ActivateTab 绑定驱动。
//
// 本文件实现状态模型与生命周期；「绑定方法按 tab 分派」（Chat/GetThread 等
// ForTab 化）与「前端 TabBar UI」在后续子步落地。

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Tab 是一个桌面会话标签。
// ThreadID 关联后端会话；空值表示该标签尚未开始任何对话。
type Tab struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"threadId,omitempty"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	ActiveAt  time.Time `json:"activeAt"`
}

// tabStore 管理标签列表。内存为真相源；每次变更后落盘
// ~/.mady/desktop-tabs.json（home 为空时仅内存，不持久化）。
type tabStore struct {
	mu     sync.Mutex
	path   string // 持久化路径；"" = 不持久化
	tabs   []*Tab
	active string // 当前激活标签 ID
}

// newTabStore 创建标签管理器并加载持久化状态。
func newTabStore(home string) *tabStore {
	ts := &tabStore{path: ""}
	if home != "" {
		ts.path = filepath.Join(home, "desktop-tabs.json")
		ts.load()
	}
	if len(ts.tabs) == 0 {
		ts.createLocked()
	}
	return ts
}

// load 从磁盘恢复标签列表与激活标签。
func (ts *tabStore) load() {
	if ts.path == "" {
		return
	}
	data, err := os.ReadFile(filepath.Clean(ts.path))
	if err != nil {
		return // 首次启动/文件缺失：使用默认单标签
	}
	var persisted struct {
		Tabs   []*Tab `json:"tabs"`
		Active string `json:"active"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		log.Printf("[mady-desktop] tabs: ignore corrupt %s: %v", ts.path, err)
		return
	}
	ts.tabs = persisted.Tabs
	ts.active = persisted.Active
	if !ts.hasLocked(ts.active) {
		ts.active = ""
	}
}

// save 将当前标签状态落盘。
func (ts *tabStore) save() {
	if ts.path == "" {
		return
	}
	data, err := json.Marshal(map[string]any{"tabs": ts.tabs, "active": ts.active})
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Clean(ts.path), data, 0o600); err != nil {
		log.Printf("[mady-desktop] tabs: save failed: %v", err)
	}
}

// List 返回标签快照列表（深拷贝副本，安全并发读取）。
func (ts *tabStore) List() []Tab {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]Tab, 0, len(ts.tabs))
	for _, t := range ts.tabs {
		out = append(out, *t)
	}
	return out
}

// ActiveID 返回当前激活标签 ID（"" = 无）。
func (ts *tabStore) ActiveID() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.active
}

// Create 新建一个标签并设为激活；返回新标签。
func (ts *tabStore) Create() Tab {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.createLocked()
}

// createLocked 内部实现：新建标签并激活（调用方须持有 ts.mu）。
func (ts *tabStore) createLocked() Tab {
	now := time.Now()
	t := &Tab{
		ID:        fmt.Sprintf("tab-%d", now.UnixNano()),
		Title:     "新会话",
		CreatedAt: now,
		ActiveAt:  now,
	}
	ts.tabs = append(ts.tabs, t)
	ts.active = t.ID
	ts.save()
	return *t
}

// Close 关闭指定标签；若关闭的是激活标签，则激活相邻标签（优先左侧）。
// 最后一个标签不允许关闭（返回错误）。
func (ts *tabStore) Close(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	idx := ts.indexLocked(id)
	if idx < 0 {
		return fmt.Errorf("tab %s: not found", id)
	}
	if len(ts.tabs) <= 1 {
		return fmt.Errorf("tab %s: cannot close the last tab", id)
	}
	ts.tabs = append(ts.tabs[:idx], ts.tabs[idx+1:]...)
	if ts.active == id {
		// 激活相邻标签：优先左侧（关闭的是最左标签时回退到右侧相邻）。
		next := idx - 1
		if next < 0 {
			next = 0
		}
		ts.active = ts.tabs[next].ID
	}
	ts.save()
	return nil
}

// Activate 激活指定标签。
func (ts *tabStore) Activate(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	idx := ts.indexLocked(id)
	if idx < 0 {
		return fmt.Errorf("tab %s: not found", id)
	}
	ts.active = id
	ts.tabs[idx].ActiveAt = time.Now()
	ts.save()
	return nil
}

// Get 返回指定标签的快照。
func (ts *tabStore) Get(id string) (*Tab, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	idx := ts.indexLocked(id)
	if idx < 0 {
		return nil, fmt.Errorf("tab %s: not found", id)
	}
	cp := *ts.tabs[idx]
	return &cp, nil
}

// SetThreadID 关联会话到标签（阶段 2.1b：ChatInTab 在首次发消息时写回）。
func (ts *tabStore) SetThreadID(id, threadID string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	idx := ts.indexLocked(id)
	if idx < 0 {
		return fmt.Errorf("tab %s: not found", id)
	}
	if threadID == "" {
		return fmt.Errorf("tab %s: threadID is required", id)
	}
	ts.tabs[idx].ThreadID = threadID
	ts.save()
	return nil
}

// indexLocked 返回标签索引；-1 = 不存在（调用方须持有 ts.mu）。
func (ts *tabStore) indexLocked(id string) int {
	for i, t := range ts.tabs {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// hasLocked 报告标签是否存在（调用方须持有 ts.mu）。
func (ts *tabStore) hasLocked(id string) bool {
	return ts.indexLocked(id) >= 0
}
