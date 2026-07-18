package server

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// poolEntry 包装池化 agent，用引用计数跟踪并发使用（C2 use-after-free 修复）。
// 所有字段仅由 poolMu 保护下访问：
//   - loadAgent 借用时 refs+1，releaseAgent 归还时 refs-1；
//   - 淘汰/关闭仅标记 evicted 并摘除出池；
//   - 只有 refs 归零且 evicted 时才真正 Close agent，
//     保证使用中的 agent 绝不会被提前关闭。
type poolEntry struct {
	agent   *agentcore.Agent
	refs    int  // 正在使用该 agent 的请求数
	evicted bool // 已从池中摘除；refs 归零后需要 Close
	pooled  bool // 是否已存入 agentPool
}

// loadAgent 借用 threadID 对应的 agent（优先复用池化实例）。
// 调用方必须在使用完成后调用 releaseAgent 归还恰好一次。
func (s *Server) loadAgent(ctx context.Context, threadID string, callCfg *agentcore.CallConfig) (*poolEntry, error) {
	if threadID != "" && callCfg == nil {
		// 池命中路径：在 poolMu 内原子完成查找与引用计数递增，
		// 防止与淘汰/关闭竞争（此前 Load 不持锁，release 的淘汰逻辑
		// 可能关闭正在使用的 agent）。
		s.poolMu.Lock()
		cached, ok := s.agentPool.Load(threadID)
		if ok {
			cached.(*poolEntry).refs++
		}
		s.poolMu.Unlock()
		if ok {
			entry := cached.(*poolEntry)
			if ts, has := s.threadStore(); has {
				if threadCfg, hasCfg, err := ts.GetThreadConfig(ctx, threadID); err == nil && hasCfg {
					entry.agent.ApplyCallConfig(threadCfg)
				}
			}
			if err := entry.agent.LoadState(ctx, threadID); err == nil {
				return entry, nil
			}
			// LoadState 失败：摘除并按引用计数释放（仍有其他请求使用时延迟关闭）。
			s.discardPoolEntry(threadID, entry)
		}
	}

	cfg := s.snapshotConfig()
	var provider agentcore.ThreadConfigProvider
	if ts, ok := s.threadStore(); ok {
		provider = ts
	}
	agent, err := agentcore.LoadAgent(ctx, cfg, agentcore.LoadAgentOptions{
		ThreadID:          threadID,
		CallCfg:           callCfg,
		ThreadCfgProvider: provider,
	})
	if err != nil {
		return nil, err
	}
	return &poolEntry{agent: agent, refs: 1}, nil
}

// discardPoolEntry 将 entry 从池中摘除并归还一次引用；若已无其他请求使用，
// 立即 Close，否则由最后一次 releaseAgent 负责 Close。
func (s *Server) discardPoolEntry(threadID string, entry *poolEntry) {
	s.poolMu.Lock()
	defer s.poolMu.Unlock()
	// 仅当池中仍是同一个 entry 时摘除，避免误删并发新建的同 key entry。
	if cur, ok := s.agentPool.Load(threadID); ok && cur == entry {
		s.agentPool.Delete(threadID)
		entry.evicted = true
	}
	entry.refs--
	if entry.refs == 0 && entry.evicted {
		entry.agent.Close()
	}
}

// releaseAgent 归还 loadAgent 借用的 entry。池化 entry 保留复用；
// 非池化 entry 尝试入池（池满或已有存活 entry 时直接关闭）。
func (s *Server) releaseAgent(entry *poolEntry, threadID string) {
	if threadID == "" {
		entry.agent.Close()
		return
	}
	s.poolMu.Lock()
	defer s.poolMu.Unlock()
	entry.refs--
	if entry.evicted {
		// 已被淘汰：最后一次归还时关闭。
		if entry.refs == 0 {
			entry.agent.Close()
		}
		return
	}
	if entry.pooled {
		// 正常归还池化 entry，保留在池中复用。
		return
	}
	// 非池化来源的 agent：尝试入池。
	if _, ok := s.agentPool.Load(threadID); ok {
		// 池中已有同 threadID 的存活 entry（并发请求已建立），关闭多余的 agent。
		entry.agent.Close()
		return
	}
	if s.poolCountLocked() >= s.poolLimit {
		s.evictIdleLocked()
	}
	if s.poolCountLocked() >= s.poolLimit {
		// 没有空闲 entry 可淘汰：不入池，直接关闭。
		entry.agent.Close()
		return
	}
	entry.pooled = true
	s.agentPool.Store(threadID, entry)
}

// poolCountLocked 返回当前池内 entry 数。调用方须持有 poolMu。
func (s *Server) poolCountLocked() int {
	count := 0
	s.agentPool.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// evictIdleLocked 淘汰一个空闲（refs==0）entry 并立即关闭其 agent。
// 使用中的 entry 不会被淘汰。调用方须持有 poolMu。
func (s *Server) evictIdleLocked() {
	s.agentPool.Range(func(key, value any) bool {
		entry := value.(*poolEntry)
		if entry.refs != 0 {
			return true // 继续使用中的 entry，寻找下一个
		}
		s.agentPool.Delete(key)
		entry.evicted = true
		entry.agent.Close()
		return false
	})
}
