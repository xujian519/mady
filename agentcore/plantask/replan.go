package plantask

import (
	"context"
	"strings"
)

// ============================================================================
// Replan 闭环（02-spec §3.2 / 03-design §3.3）
// ============================================================================

// Replanner 由装配层注入：根据用户反馈生成新步骤并合并。
// 具体实现可调用 domains/reasoning.Planner（bootstrap 层组装，见
// bootstrap/plantask_bridge.go）；测试用确定性实现。
//
// plantask 不依赖 domains/reasoning（分层红线），仅依赖此接口。
type Replanner interface {
	// Replan 基于会话当前状态与反馈生成新计划并写回会话。
	// 返回应跳过（保持 done）与应重跑（移除完成标记）的步骤哈希。
	// 返回时，会话应处于可执行状态（Plan/TaskIDs/CompletedIDs 已更新）。
	Replan(ctx context.Context, s *PlanTaskSession, feedback string) (skipHashes, removedHashes []string, err error)
}

// rerunPrefix 是反馈文本中显式重跑语法的前缀（03-design §3.3 步骤 2）。
// 例："重跑:step1,step3" 或 "重跑:检索,比对"（按描述子串匹配）。
const rerunPrefix = "重跑:"

// parseRerunTargets 解析反馈中的显式重跑目标。
// 返回的每个目标按描述子串（或步骤序号）匹配步骤。
func parseRerunTargets(feedback string) []string {
	if !strings.Contains(feedback, rerunPrefix) {
		return nil
	}
	var targets []string
	for _, line := range strings.Split(feedback, "\n") {
		idx := strings.Index(line, rerunPrefix)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+len(rerunPrefix):])
		for _, part := range strings.Split(rest, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				targets = append(targets, part)
			}
		}
	}
	return targets
}

// matchRerunTarget 判断步骤是否命中重跑目标。
// 目标按以下顺序匹配：步骤描述子串 → 步骤序号字符串（"1"、"2"…）。
func matchRerunTarget(step StepSnapshot, target string) bool {
	if strings.Contains(step.Description, target) {
		return true
	}
	if order := strings.TrimPrefix(target, "step"); order != "" {
		if intStr := strings.TrimSpace(order); intStr == intString(step.Order) {
			return true
		}
	}
	return false
}

func intString(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// ReplanMerge 按 03-design §3.3 合并旧已完成步骤与新步骤：
//
//  1. completed = 旧 Plan 中已完成（CompletedIDs 命中其 Hash）的步骤
//  2. 显式重跑集 = 解析反馈中的"重跑:"目标
//  3. keptDone  = completed - 显式重跑集（且在新 Plan 中哈希一致）
//  4. 最终执行序列 = 新 Plan 步骤（keptDone 标记为跳过）
//
// 返回：
//   - skipHashes：应保持 done（跳过执行）的步骤哈希集合
//   - removedHashes：被移除完成标记（需重跑）的步骤哈希集合
func ReplanMerge(oldSteps []StepSnapshot, completedIDs []string, newSteps []StepSnapshot, feedback string) (skipHashes, removedHashes map[string]bool) {
	skipHashes = make(map[string]bool)
	removedHashes = make(map[string]bool)

	// 旧步骤按哈希索引（完成标记存的是步骤哈希）。
	oldByHash := make(map[string]StepSnapshot, len(oldSteps))
	for _, s := range oldSteps {
		oldByHash[s.Hash] = s
	}
	newByHash := make(map[string]StepSnapshot, len(newSteps))
	for _, s := range newSteps {
		newByHash[s.Hash] = s
	}

	rerunTargets := parseRerunTargets(feedback)

	for _, id := range completedIDs {
		old, ok := oldByHash[id]
		if !ok {
			continue // 完成标记对应的旧步骤已不存在
		}
		// 步骤 3：显式重跑 → 移除完成标记。
		rerun := false
		for _, target := range rerunTargets {
			if matchRerunTarget(old, target) {
				rerun = true
				break
			}
		}
		// 步骤 5：新 Plan 中哈希一致才保留 done；路径变更后旧完成不再可信。
		if _, inNew := newByHash[id]; !inNew {
			rerun = true
		}
		if rerun {
			removedHashes[id] = true
		} else {
			skipHashes[id] = true
		}
	}
	return skipHashes, removedHashes
}
