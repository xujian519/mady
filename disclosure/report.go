package disclosure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// generate_keywords — 检索关键词生成（Phase 1 简化版）
// =============================================================================

// generateKeywordsNode 返回检索关键词生成的 Pregel 节点。
// Phase 1 实现为确定性规则提取，Phase 2 将替换为 LLM 混合模式。
func generateKeywordsNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		var keywords []string

		// 从提取结果中收集关键词
		if raw, ok := state[StateKeyExtraction]; ok {
			if ext, ok := raw.(*ExtractionResult); ok && ext != nil {
				keywords = collectKeywordsFromExtraction(ext)
			}
		}

		if len(keywords) == 0 {
			keywords = []string{"技术交底书分析"}
		}

		state[StateKeySearchKeywords] = keywords
		return state, nil
	}
}

// collectKeywordsFromExtraction 从提取的特征/问题/效果中收集关键词。
func collectKeywordsFromExtraction(ext *ExtractionResult) []string {
	seen := make(map[string]bool)
	var kw []string

	addIfNew := func(word string) {
		word = strings.TrimSpace(word)
		if word != "" && !seen[word] {
			seen[word] = true
			kw = append(kw, word)
		}
	}

	for _, p := range ext.Problems {
		for _, w := range strings.Fields(p) {
			if len([]rune(w)) >= 2 {
				addIfNew(w)
			}
		}
	}

	for _, f := range ext.Features {
		addIfNew(string(f.Category))
		for _, w := range strings.Fields(f.Description) {
			if len([]rune(w)) >= 2 {
				addIfNew(w)
			}
		}
	}

	return kw
}

// =============================================================================
// check_novelty — 新颖性初判（Phase 1 stub）
// =============================================================================

// noveltyStubNode 返回新颖性初判的 Pregel 节点（Phase 1 stub）。
// Phase 2 将替换为 retrieval 模块集成 + domains/reasoning 推理引擎。
func noveltyStubNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		result := &NoveltyResult{
			Assessed:   false,
			Conclusion: "未评估",
			Notes:      "新颖性初判功能尚未启用。请参照检索关键词自行在专利数据库中检索评估。",
		}
		state[StateKeyNovelty] = result
		return state, nil
	}
}

// =============================================================================
// generate_report — 报告生成 Agent
// =============================================================================

// generateReportNode 返回报告生成的 Pregel 节点。
// 汇总所有中间结果，生成结构化分析报告。
func generateReportNode(provider agentcore.Provider) graph.PregelNode {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        "disclosure-report",
			Model:       "default",
			Provider:    provider,
			Temperature: 0.0,
		},
		SystemPrompt: buildReportPrompt(),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 1,
		},
	}

	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := buildReportInput(state)
		agent := agentcore.New(cfg)
		reportText, err := agent.Run(ctx, input)
		if err != nil {
			return state, fmt.Errorf("generate_report: %w", err)
		}

		report := buildAnalysisReport(state, reportText)
		state[StateKeyReport] = report
		state[StateKeyOutput] = reportText
		return state, nil
	}
}

// buildReportPrompt 构造报告生成的 SystemPrompt。
func buildReportPrompt() string {
	return strings.Join([]string{
		"你是一名资深专利代理师，负责生成技术交底书分析报告。",
		"请基于以下汇总信息，生成结构化的分析报告。",
		"",
		"报告应包含以下章节：",
		"1. 文档概况 — 标题、格式、段落统计",
		"2. 技术问题 — 列出所有识别到的技术问题",
		"3. 技术特征 — 按最小技术单元分类列出",
		"4. 技术效果 — 列出所有有益效果",
		"5. 一致性分析 — 问题-特征-效果的因果闭环情况",
		"6. 检索建议 — 推荐的关键词",
		"7. 免责声明 — 「本报告由 AI 辅助生成，不构成正式法律意见」",
		"",
		"请用简体中文输出，专业、严谨、客观。",
	}, "\n")
}

// buildReportInput 从 PregelState 构建报告生成的输入。
func buildReportInput(state graph.PregelState) string {
	var sb strings.Builder

	// 文档信息
	if doc, ok := state[StateKeyDoc].(*DisclosureDoc); ok && doc != nil {
		fmt.Fprintf(&sb, "【文档标题】%s\n", doc.Title)
		fmt.Fprintf(&sb, "【格式】%s\n", doc.Format)
		fmt.Fprintf(&sb, "【段落数】%d\n\n", len(doc.Sections))
	}

	// 提取结果
	if raw, ok := state[StateKeyExtraction]; ok {
		if ext, ok := raw.(*ExtractionResult); ok && ext != nil {
			sb.WriteString("【技术问题】\n")
			for _, p := range ext.Problems {
				fmt.Fprintf(&sb, "- %s\n", p)
			}
			sb.WriteString("\n【技术特征】\n")
			for _, f := range ext.Features {
				fmt.Fprintf(&sb, "- [%s] %s (%s)\n", f.Category, f.Description, f.Importance)
			}
			sb.WriteString("\n【技术效果】\n")
			for _, e := range ext.Effects {
				fmt.Fprintf(&sb, "- %s\n", e)
			}
			sb.WriteString("\n【PFE 三元组】\n")
			for _, t := range ext.PFETriples {
				fmt.Fprintf(&sb, "- %s → %s\n", t.Problem, t.Effect)
			}
		}
	}

	// 一致性结果
	if raw, ok := state[StateKeyConsistency]; ok {
		if cr, ok := raw.(*ConsistencyResult); ok && cr != nil {
			fmt.Fprintf(&sb, "\n【一致性得分】%.0f%%\n", cr.OverallScore*100)
			if cr.RetriesExhausted {
				sb.WriteString("【注意】一致性校验已达最大重试次数，以下问题未消解：\n")
			}
			if len(cr.Issues) > 0 {
				sb.WriteString("【一致性问题】\n")
				for _, issue := range cr.Issues {
					fmt.Fprintf(&sb, "- [%s] %s\n", issue.Severity, issue.Description)
				}
			}
		}
	}

	// 检索关键词
	if kw, ok := state[StateKeySearchKeywords]; ok {
		if kwList, ok := kw.([]string); ok && len(kwList) > 0 {
			fmt.Fprintf(&sb, "\n【检索关键词】%s\n", strings.Join(kwList, "、"))
		}
	}

	return sb.String()
}

// buildAnalysisReport 组装最终的 AnalysisReport。
func buildAnalysisReport(state graph.PregelState, reportText string) *AnalysisReport {
	report := &AnalysisReport{
		ID:          "rpt_" + time.Now().Format("20060102_150405"),
		ReportText:  reportText,
		GeneratedAt: time.Now(),
	}

	if doc, ok := state[StateKeyDoc].(*DisclosureDoc); ok {
		report.Document = doc
	}
	if raw, ok := state[StateKeyExtraction]; ok {
		if ext, ok := raw.(*ExtractionResult); ok {
			report.Extraction = ext
		}
	}
	if raw, ok := state[StateKeyConsistency]; ok {
		if cr, ok := raw.(*ConsistencyResult); ok {
			report.Consistency = cr
		}
	}
	if kw, ok := state[StateKeySearchKeywords]; ok {
		if kwList, ok := kw.([]string); ok {
			report.SearchKeywords = kwList
		}
	}
	if raw, ok := state[StateKeyNovelty]; ok {
		if nr, ok := raw.(*NoveltyResult); ok {
			report.Novelty = nr
		}
	}

	return report
}

// =============================================================================
// review_gate — 人工复核关卡
// =============================================================================

// reviewGateNode 返回人工复核关卡的 Pregel 节点。
// 标记流程已到复核节点，实际暂停由 ApprovalGate LifecycleHook 触发。
func reviewGateNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		state["_gate_ready"] = true
		if report, ok := state[StateKeyReport].(*AnalysisReport); ok && report != nil {
			report.ReviewedByHuman = false
		}
		return state, nil
	}
}
