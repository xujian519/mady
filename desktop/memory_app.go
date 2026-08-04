//go:build darwin

package main

// memory_app.go — 记忆面板绑定（阶段 4：仿照 Reasonix MemoryPanel）。
//
// 直接暴露 memory 三层系统的只读/维护操作：
//   - ListMemories：合并列出 user/session/long_term 三层
//   - RememberMemory：手动写入一条记忆
//   - ForgetMemory：按 ID 删除
//   - RecallMemories：语义检索（搜索框）
//
// 依赖 bootstrap.Context.MemoryManager（deferred 初始化完成后非 nil；
// 未就绪时返回错误，前端静默降级为空列表）。

import (
	"context"
	"fmt"
	"sort"

	"github.com/xujian519/mady/memory"
)

// memoryManagerOrErr 返回内存记忆管理器；未初始化时返回错误。
func (a *App) memoryManagerOrErr() (*memory.Manager, error) {
	if a.fc == nil || a.fc.MemoryManager == nil {
		return nil, fmt.Errorf("memory system not ready")
	}
	return a.fc.MemoryManager, nil
}

// memoryCtx 返回 Wails 运行时上下文；极端情况下（startup 前绑定被调用）
// 退化为 context.Background()，避免 nil ctx 传入存储层。
func (a *App) memoryCtx() context.Context {
	if ctx := a.ctxOrNil(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// ListMemories 合并列出全部三层记忆（user/session/long_term），
// 按更新时间倒序；limit<=0 时返回全部（默认上限 200 防滥用）。
func (a *App) ListMemories(limit int) ([]memory.MemoryEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	mgr, err := a.memoryManagerOrErr()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	ctx := a.memoryCtx()
	layers := []memory.MemoryLayer{memory.LayerUser, memory.LayerSession, memory.LayerLongTerm}
	var all []memory.MemoryEntry
	for _, layer := range layers {
		entries, err := mgr.Store().List(ctx, layer, memory.ListOptions{Limit: limit})
		if err != nil {
			continue // 单层失败不阻断整体
		}
		all = append(all, entries...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// RememberMemory 手动写入一条长期记忆（用户主动记录，不参与自动提取）。
func (a *App) RememberMemory(content string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if content == "" {
		return "", fmt.Errorf("memory content is required")
	}
	mgr, err := a.memoryManagerOrErr()
	if err != nil {
		return "", err
	}
	return mgr.Store().Remember(a.memoryCtx(), content, memory.MemoryScope{}, memory.LayerLongTerm, nil)
}

// ForgetMemory 按 ID 删除一条记忆。
func (a *App) ForgetMemory(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("memory id is required")
	}
	mgr, err := a.memoryManagerOrErr()
	if err != nil {
		return err
	}
	return mgr.Store().Forget(a.memoryCtx(), id)
}

// RecallMemories 按语义相似度检索记忆（记忆面板搜索框）。
func (a *App) RecallMemories(query string, limit int) ([]memory.ScoredMemory, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if query == "" {
		return nil, nil
	}
	mgr, err := a.memoryManagerOrErr()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	results, err := mgr.Store().Recall(a.memoryCtx(), query, memory.MemoryFilter{})
	if err != nil {
		return nil, err
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
