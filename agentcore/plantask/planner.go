package plantask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// Plan → tasklist 映射（02-spec §7 边界契约：plantask → agentcore/tasklist）
// ============================================================================

// TaskStore 是 plantask 使用的任务存储最小接口。
// agentcore/tasklist.Store 天然满足该接口（含 FileStore 实现），
// 由装配层注入，避免 plantask 直接耦合 tasklist 扩展实例。
type TaskStore interface {
	Create(ctx context.Context, t *agentcore.Task) error
	NextID(ctx context.Context) (string, error)
	Update(ctx context.Context, t *agentcore.Task) error
}

// PlanStepInput 是 plan_submit 工具接收的步骤输入。
type PlanStepInput struct {
	Order       int    `json:"order"`
	Strategy    string `json:"strategy"`
	Description string `json:"description"`
}

// ToSnapshot 将输入步骤转为持久化快照并计算哈希。
func (p PlanStepInput) ToSnapshot() StepSnapshot {
	return StepSnapshot{
		Order:       p.Order,
		Strategy:    p.Strategy,
		Description: p.Description,
		Hash:        StepHash(p.Order, p.Strategy, p.Description),
	}
}

// StepHash 计算步骤的 SHA-256（Order+Strategy+Description）。
// 用于 replan 时"已完成步骤是否可信"的判定（03-design §3.3 步骤 5）。
func StepHash(order int, strategy, description string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", order, strategy, description)))
	return hex.EncodeToString(sum[:])
}

// SyncPlanToTasks 将 Plan 步骤同步为 tasklist 任务。
// 已存在的步骤（按哈希匹配）保持原任务，新增步骤创建任务，并维护
// BlockedBy 依赖（顺序步骤 i 阻塞 i+1）。
// 返回任务 ID 列表（与 steps 同序）。
func SyncPlanToTasks(ctx context.Context, store TaskStore, session *PlanTaskSession, steps []StepSnapshot) ([]string, error) {
	if store == nil {
		return nil, fmt.Errorf("plantask: task store is nil")
	}

	ids := make([]string, len(steps))
	for i, step := range steps {
		// 已存在且哈希一致的任务（replan 场景）复用。
		if i < len(session.TaskIDs) && session.Plan.Steps != nil && i < len(session.Plan.Steps) &&
			session.Plan.Steps[i].Hash == step.Hash {
			ids[i] = session.TaskIDs[i]
			continue
		}
		id, err := store.NextID(ctx)
		if err != nil {
			return nil, fmt.Errorf("plantask: next task id: %w", err)
		}
		task := &agentcore.Task{
			ID:          id,
			Subject:     stepDescriptionToSubject(step.Description),
			Description: step.Description,
			Status:      agentcore.TaskPending,
			Priority:    agentcore.TaskPriorityNormal,
			ActiveForm:  "正在" + stepDescriptionToSubject(step.Description),
			CreatedAt:   session.CreatedAt,
			UpdatedAt:   session.UpdatedAt,
			Metadata: map[string]any{
				"plantask_session": session.ID,
				"step_order":       step.Order,
				"step_hash":        step.Hash,
			},
		}
		if err := store.Create(ctx, task); err != nil {
			return nil, fmt.Errorf("plantask: create task: %w", err)
		}
		ids[i] = id
	}

	// 维护顺序依赖：步骤 i 阻塞 i+1。
	for i := 0; i+1 < len(ids); i++ {
		if ids[i] == "" || ids[i+1] == "" {
			continue
		}
		if err := store.Update(ctx, &agentcore.Task{
			ID:        ids[i+1],
			BlockedBy: []string{ids[i]},
		}); err != nil {
			return nil, fmt.Errorf("plantask: set dependency: %w", err)
		}
	}

	return ids, nil
}

// stepDescriptionToSubject 将步骤描述压缩为任务标题（≤40 rune）。
func stepDescriptionToSubject(desc string) string {
	desc = strings.TrimSpace(desc)
	r := []rune(desc)
	if len(r) <= 40 {
		if desc == "" {
			return "未命名步骤"
		}
		return desc
	}
	return string(r[:40]) + "…"
}
