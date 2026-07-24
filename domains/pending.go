package domains

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Pending Approval Store — 进程重启后恢复待审批状态
// ---------------------------------------------------------------------------

// PendingStatus 表示待审批请求的当前状态。
type PendingStatus string

const (
	PendingStatusPending   PendingStatus = "pending"
	PendingStatusResponded PendingStatus = "responded"
	PendingStatusExpired   PendingStatus = "expired"
)

// PendingApproval 记录一次 ApprovalGate 触发后、人工响应前的活动审批请求。
// 与 ApprovalRecord（已决审计日志）不同，PendingApproval 的生命周期是：
// 创建 → 等待人工决策 → 决策后转为 ApprovalRecord 并删除/标记为已响应。
type PendingApproval struct {
	ID             string        // 唯一标识
	SessionID      string        // 触发审批的 Agent 会话
	CaseID         string        // 可选案件标识
	TriggerKeyword string        // 触发审批门的关键词
	OriginalOutput string        // 触发时的 AI 输出全文
	ToolCallsJSON  string        // 关联工具调用的 JSON 序列化
	Status         PendingStatus // pending / responded / expired
	CreatedAt      time.Time
	ExpiresAt      *time.Time // 可选超时时间
	RespondedAt    *time.Time // 响应时间
}

// PendingStore 管理活动审批请求的持久化。
// 与 ApprovalStore（审计日志，只增不删）不同，PendingStore 需要
// 响应后删除或标记，以维持"当前有哪些待审批"的查询准确。
type PendingStore interface {
	// SavePending 创建或更新待审批请求。
	SavePending(ctx context.Context, p PendingApproval) error
	// LoadPending 加载一个待审批请求。
	LoadPending(ctx context.Context, id string) (*PendingApproval, error)
	// ListPending 列出所有状态为 pending 的请求（启动时恢复用）。
	ListPending(ctx context.Context) ([]PendingApproval, error)
	// ListPendingBySession 列出某会话的待审批请求。
	ListPendingBySession(ctx context.Context, sessionID string) ([]PendingApproval, error)
	// DeletePending 删除（已响应或取消的）待审批请求。
	DeletePending(ctx context.Context, id string) error
	// Respond 原子地将待审批标记为已响应并写入审批记录。
	// 即：pending_approvals.status = 'responded' + approval_records INSERT。
	// 保证两者不分裂（同一事务）。
	Respond(ctx context.Context, id string, record ApprovalRecord) error
	// ExpirePending 将超时的待审批请求标记为 expired，
	// 返回实际过期的行数。
	ExpirePending(ctx context.Context) (int64, error)
	// Close 释放底层资源。
	Close() error
}
