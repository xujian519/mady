package guardrails

import (
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// 本文件实现 deferred persist 机制（P2 T8）：
// Strict guardrail 标记 SuppressPersist=true 时，消息暂存而非丢弃。
// ApprovalGate 审批通过后 Commit 写入持久化，拒绝后 Discard 丢弃。

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
