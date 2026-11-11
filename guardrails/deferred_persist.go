package guardrails

import (
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// 本文件实现 deferred persist 机制（P2 T8）：
// Strict guardrail 标记 SuppressPersist=true 时，消息暂存而非丢弃。
//
// 接线状态（M20 2026-11-11 复核，两端均已接入）：
//   - Store 端：levels.go AfterModelCall 在 SuppressPersist=true 且配置了
//     DeferredQueue（WithDeferredQueue 注入）时调用 queue.Store；
//   - Commit/Discard 端：domains/assemble.go appendStrictGuardrails 在
//     withDQ=true 时创建队列并同步注入护栏与 ApprovalGate
//     （经 domains.DeferredPersistFuncs 适配）：RecordDecision 触发
//     Commit/Discard 遍历 Pending() 清空暂存（采纳/拒绝各一档）。
// 消息索引约定：levels.go 以固定 msgIndex=0 暂存（单回合单条消息），
// 重复 Store 同一回合会覆盖。

// DeferredPersistQueue 管理因 SuppressPersist 暂缓持久化的消息。
// 仅在 Agent Level = Strict 且 ApprovalGate 启用联动时有数据。
type DeferredPersistQueue struct {
	mu       sync.Mutex
	messages map[int]agentcore.Message // msgIndex → 暂存消息
}

// NewDeferredPersistQueue 创建暂存队列。
func NewDeferredPersistQueue() *DeferredPersistQueue {
	return &DeferredPersistQueue{
		messages: make(map[int]agentcore.Message),
	}
}

// Store 暂存被 SuppressPersist 标记的消息。
// 重复 Store 同一 msgIndex 会覆盖。
func (q *DeferredPersistQueue) Store(msgIndex int, msg agentcore.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages[msgIndex] = msg
}

// Commit 取出并移除已审批通过的消息，返回 ok=true。
// 若 msgIndex 未暂存，返回 ok=false。
func (q *DeferredPersistQueue) Commit(msgIndex int) (agentcore.Message, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	msg, ok := q.messages[msgIndex]
	if !ok {
		return agentcore.Message{}, false
	}
	delete(q.messages, msgIndex)
	return msg, true
}

// Discard 丢弃暂存消息（审批拒绝场景）。
func (q *DeferredPersistQueue) Discard(msgIndex int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.messages, msgIndex)
}

// Pending 返回所有待审批的消息索引列表。
func (q *DeferredPersistQueue) Pending() []int {
	q.mu.Lock()
	defer q.mu.Unlock()
	indices := make([]int, 0, len(q.messages))
	for i := range q.messages {
		indices = append(indices, i)
	}
	return indices
}

// Has 检查指定索引是否有暂存消息。
func (q *DeferredPersistQueue) Has(msgIndex int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.messages[msgIndex]
	return ok
}

// Len 返回暂存消息数量。
func (q *DeferredPersistQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages)
}
