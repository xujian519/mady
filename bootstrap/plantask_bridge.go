package bootstrap

import (
	"context"

	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/domains/reasoning"
)

// PlantaskBridge 实现 plantask.Replanner（03-design §3.3 合并算法落地）。
//
// 职责：
//  1. 经 GenerateSteps（可插拔）生成新步骤——装配层可注入调用
//     domains/reasoning.Planner 的实现（LLM 规划）；未注入时保持当前步骤
//     （仍处理"重跑:"显式语法）
//  2. 调用 plantask.ReplanMerge 合并已完成步骤
//  3. 写回会话（Plan / CompletedIDs）并同步 tasklist 任务
//
// plantask 不依赖 domains/reasoning（分层红线），bootstrap 是允许跨层的
// 装配点，故 Bridge 放在此处。
type PlantaskBridge struct {
	// TaskStore 用于任务同步（与 plantask.Config.TaskStore 同一实例）。
	TaskStore plantask.TaskStore
	// GenerateSteps 生成新步骤；nil 时回退 runner（若已接入），否则保持当前步骤。
	GenerateSteps func(ctx context.Context, s *plantask.PlanTaskSession, feedback string) ([]plantask.StepSnapshot, error)
	// runner 是五步推理引擎实例（真实 LLM 规划链）。经 SetRunner 接入。
	runner *reasoning.FiveStepRunner
}

// SetRunner 接入五步推理引擎：GenerateSteps 未显式设置时，
// 反馈经 runner.GenerateReplanSteps 走真实规划管线（模板/KG/LLM）。
func (b *PlantaskBridge) SetRunner(r *reasoning.FiveStepRunner) {
	b.runner = r
}

// NewPlantaskBridge 创建 bridge。
func NewPlantaskBridge(taskStore plantask.TaskStore) *PlantaskBridge {
	return &PlantaskBridge{TaskStore: taskStore}
}

// Replan 实现 plantask.Replanner。
func (b *PlantaskBridge) Replan(ctx context.Context, s *plantask.PlanTaskSession, feedback string) ([]string, []string, error) {
	newSteps := s.Plan.Steps
	switch {
	case b.GenerateSteps != nil:
		generated, err := b.GenerateSteps(ctx, s, feedback)
		if err != nil {
			return nil, nil, err
		}
		newSteps = generated
	case b.runner != nil:
		steps, err := b.runner.GenerateReplanSteps(ctx, feedback)
		if err != nil {
			return nil, nil, err
		}
		newSteps = toSnapshots(steps)
	}

	skip, removed := plantask.ReplanMerge(s.Plan.Steps, s.CompletedIDs, newSteps, feedback)

	// 写回 Plan 与 CompletedIDs（keptDone = 新 Plan 中保持 done 的步骤）。
	s.Plan = plantask.PlanSnapshot{Steps: newSteps}
	kept := make([]string, 0, len(newSteps))
	for _, st := range newSteps {
		if skip[st.Hash] {
			kept = append(kept, st.Hash)
		}
	}
	s.CompletedIDs = kept

	// 同步任务（新增步骤创建任务，顺序依赖维护）。
	ids, err := plantask.SyncPlanToTasks(ctx, b.TaskStore, s, newSteps)
	if err != nil {
		return nil, nil, err
	}
	s.TaskIDs = ids

	return mapKeys(skip), mapKeys(removed), nil
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// toSnapshots 将 reasoning.PlanStep 转换为 plantask.StepSnapshot（含哈希）。
func toSnapshots(steps []reasoning.PlanStep) []plantask.StepSnapshot {
	out := make([]plantask.StepSnapshot, len(steps))
	for i, s := range steps {
		out[i] = plantask.StepSnapshot{
			Order:       s.Order,
			Strategy:    string(s.Strategy),
			Description: s.Description,
		}
		out[i].Hash = plantask.StepHash(out[i].Order, out[i].Strategy, out[i].Description)
	}
	return out
}
