package disclosure

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// maxConsistencyRetries 一致性校验最大回退次数。
const maxConsistencyRetries = 2

// consistencyCheckNode 返回一致性校验的 Pregel 节点（确定性，非 LLM）。
// 校验内容：
//  1. 特征-效果闭环：每个 feature 是否都有对应的 effect
//  2. 问题-特征闭环：每个 problem 是否都有解决的 feature
//  3. 孤立检查：是否存在未关联的特征或效果
func consistencyCheckNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		var result ConsistencyResult
		result.Pass = true
		result.OverallScore = 1.0

		raw, ok := state[StateKeyExtraction]
		if !ok || raw == nil {
			result.Pass = false
			result.Issues = append(result.Issues, ConsistencyIssue{
				Type: "missing_extraction", Description: "提取结果为空",
				Severity: "error", SourceNode: "check_consistency",
			})
			state[StateKeyConsistency] = &result
			return state, nil
		}

		extraction, ok := raw.(*ExtractionResult)
		if !ok {
			return state, fmt.Errorf("consistency: invalid extraction_result type: %T", raw)
		}

		issues := checkExtractionConsistency(extraction)
		result.Issues = issues

		if len(issues) > 0 {
			result.Pass = false
			result.OverallScore = calculateScore(issues)
			result.Feedback = buildFeedback(issues)
		}

		state[StateKeyConsistency] = &result
		return state, nil
	}
}

// checkExtractionConsistency 执行一致性检查逻辑。
func checkExtractionConsistency(ext *ExtractionResult) []ConsistencyIssue {
	var issues []ConsistencyIssue

	// 构建索引
	featureIDs := make(map[string]bool)
	for _, f := range ext.Features {
		featureIDs[f.ID] = true
	}

	hasProblem := make(map[string]bool)
	solvesProblem := make(map[string]bool) // feature -> 是否关联了 problem
	hasEffect := make(map[string]bool)     // feature -> 是否有对应 effect

	for _, t := range ext.PFETriples {
		if t.Problem != "" {
			hasProblem[t.Problem] = true
		}
		for _, fid := range t.FeatureIDs {
			solvesProblem[fid] = true
		}
		if t.Effect != "" {
			for _, fid := range t.FeatureIDs {
				hasEffect[fid] = true
			}
		}
	}

	// 检查 1: 是否存在无对应 effect 的 feature
	for _, f := range ext.Features {
		if !hasEffect[f.ID] {
			issues = append(issues, ConsistencyIssue{
				Type:        "orphan_feature",
				Description: fmt.Sprintf("技术特征「%s」没有对应的技术效果", f.Description),
				Severity:    "warning",
				SourceNode:  "extract_effects",
			})
		}
	}

	// 检查 2: 是否存在无关联 problem 的 feature
	for _, f := range ext.Features {
		if !solvesProblem[f.ID] {
			issues = append(issues, ConsistencyIssue{
				Type:        "unmatched_feature",
				Description: fmt.Sprintf("技术特征「%s」未关联任何技术问题", f.Description),
				Severity:    "warning",
				SourceNode:  "extract_problem",
			})
		}
	}

	// 检查 3: 是否存在无 feature 解决的 problem
	for _, p := range ext.Problems {
		if !hasProblem[p] {
			issues = append(issues, ConsistencyIssue{
				Type:        "unmatched_problem",
				Description: fmt.Sprintf("技术问题「%s」没有对应的技术特征", truncate(p, 50)),
				Severity:    "warning",
				SourceNode:  "extract_features",
			})
		}
	}

	// 检查 4: 效果-特征交叉校验
	effectToFeature := make(map[string]bool)
	for _, t := range ext.PFETriples {
		if len(t.FeatureIDs) > 0 && t.Effect != "" {
			effectToFeature[t.Effect] = true
		}
	}
	for _, e := range ext.Effects {
		if !effectToFeature[e] {
			issues = append(issues, ConsistencyIssue{
				Type:        "orphan_effect",
				Description: fmt.Sprintf("技术效果「%s」未关联任何技术特征", truncate(e, 50)),
				Severity:    "info",
				SourceNode:  "extract_features",
			})
		}
	}

	return issues
}

// calculateScore 根据 issue 数量计算得分。
func calculateScore(issues []ConsistencyIssue) float64 {
	if len(issues) == 0 {
		return 1.0
	}
	score := 1.0 - float64(len(issues))*0.2
	if score < 0.0 {
		score = 0.0
	}
	return score
}

// buildFeedback 生成回退重提取时的改进提示。
func buildFeedback(issues []ConsistencyIssue) string {
	var sb strings.Builder
	sb.WriteString("一致性校验发现以下问题，请针对性修正：\n")
	for _, issue := range issues {
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", issue.Severity, issue.SourceNode, issue.Description)
	}
	return sb.String()
}

// consistencyRouter 是 check_consistency 节点的 PregelEdgeRouter。
// 决定下一个 superstep 是前进到 generate_keywords 还是回退到三提取节点。
// 回退时：
//   - 清除旧的 ExtractionResult（避免 append 累积）
//   - 设置 retry feedback 供提取 Agent 注入上下文
//
// fail-open 时：
//   - 设置 RetriesExhausted 标志而非覆写 Pass
//   - 保留 Issues 供报告生成使用
func consistencyRouter(ctx context.Context, state graph.PregelState) []string {
	raw, ok := state[StateKeyConsistency]
	if !ok || raw == nil {
		return []string{"generate_keywords"} // fail-open
	}

	result, ok := raw.(*ConsistencyResult)
	if !ok {
		return []string{"generate_keywords"}
	}

	if result.Pass {
		// 清除重试状态
		delete(state, StateKeyRetryFeedback)
		delete(state, StateKeyRetryCount)
		return []string{"generate_keywords"}
	}

	retryRaw, _ := state[StateKeyRetryCount]
	retry, _ := retryRaw.(int)

	if retry < maxConsistencyRetries {
		state[StateKeyRetryCount] = retry + 1

		// 清除旧的 ExtractionResult（重试时 merge_extractions 会重建）
		delete(state, StateKeyExtraction)

		// 设置 retry feedback 供提取 Agent 读取
		state[StateKeyRetryFeedback] = result.Feedback

		return []string{"extract_problem", "extract_features", "extract_effects"}
	}

	// fail-open: 超过最大重试次数，保留问题但标记已尽力
	result.RetriesExhausted = true
	for i := range result.Issues {
		result.Issues[i].Description += " [未消解]"
	}
	state[StateKeyConsistency] = result
	delete(state, StateKeyRetryFeedback)
	delete(state, StateKeyRetryCount)
	return []string{"generate_keywords"}
}

// truncate 截断字符串到指定长度，保证不切断多字节 UTF-8 字符。
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
