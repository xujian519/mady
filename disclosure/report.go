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
// check_novelty — 新颖性初判（基于技术特征分类与关键词匹配）
// =============================================================================

// noveltyStubNode 返回新颖性初判的 Pregel 节点。
// 基于已提取的技术特征、问题、效果进行结构化预评估。
// Phase 2 将增强为 retrieval 模块集成 + domains/reasoning 推理引擎。
func noveltyStubNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		result := assessNoveltyFromState(state)
		state[StateKeyNovelty] = result
		return state, nil
	}
}

// assessNoveltyFromState 基于 PregelState 中的提取结果和关键词进行新颖性预评估。
func assessNoveltyFromState(state graph.PregelState) *NoveltyResult {
	// Collect available data.
	var features []TechFeature
	var problems []string
	var effects []string
	var keywords []string

	if raw, ok := state[StateKeyExtraction]; ok {
		if ext, ok := raw.(*ExtractionResult); ok && ext != nil {
			features = ext.Features
			problems = ext.Problems
			effects = ext.Effects
		}
	}
	if raw, ok := state[StateKeySearchKeywords]; ok {
		if kw, ok := raw.([]string); ok {
			keywords = kw
		}
	}

	// Build assessment.
	var b strings.Builder
	b.WriteString("## 新颖性预评估（自动分析）\n\n")

	// Analyze features by category.
	catCount := make(map[TechFeatureCategory]int)
	highImportance := 0
	for _, f := range features {
		catCount[f.Category]++
		if f.Importance == "high" {
			highImportance++
		}
	}

	if len(features) == 0 {
		b.WriteString("未提取到可分析的技术特征。\n")
		b.WriteString("建议：确认技术交底书内容完整性，或手动补充技术特征描述。\n")
		return &NoveltyResult{
			Assessed:   true,
			Conclusion: "特征不足，无法评估",
			Notes:      b.String(),
		}
	}

	fmt.Fprintf(&b, "共识别 **%d** 个技术特征（其中重要特征 **%d** 个）：\n\n", len(features), highImportance)
	for cat, count := range catCount {
		label := string(cat)
		switch cat {
		case CatStructure:
			label = "结构特征"
		case CatMethod:
			label = "方法/工艺特征"
		case CatParameter:
			label = "参数特征"
		case CatMaterial:
			label = "材料特征"
		}
		fmt.Fprintf(&b, "- %s：%d 项\n", label, count)
	}
	b.WriteString("\n")

	// Feature-level breakdown.
	if highImportance > 0 {
		b.WriteString("**重要特征详情：**\n\n")
		for _, f := range features {
			if f.Importance != "high" {
				continue
			}
			fmt.Fprintf(&b, "- %s（[%s] %s）", f.Description, f.Category, f.Function)
			switch f.PriorArtStatus {
			case "known":
				b.WriteString(" — 可能为现有技术")
			case "unknown":
				b.WriteString(" — 需重点检索确认新颖性")
			case "partial":
				b.WriteString(" — 部分已知，需进一步对比")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Problem-Effect chain analysis.
	if len(problems) > 0 {
		b.WriteString("**要解决的技术问题：**\n")
		for _, p := range problems {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	if len(effects) > 0 {
		b.WriteString("**达到的技术效果：**\n")
		for _, e := range effects {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}

	// Search keyword guidance.
	if len(keywords) > 0 {
		b.WriteString("**检索关键词建议：**\n")
		b.WriteString(strings.Join(keywords, "、"))
		b.WriteString("\n\n")
	}

	// Overall assessment.
	overall := len(features)
	b.WriteString("**初步判断：**\n")
	switch {
	case overall == 0:
		b.WriteString("无法进行新颖性评估，缺少技术特征数据。\n")
	case highImportance > 0:
		fmt.Fprintf(&b,
			"交底书包含 %d 个重要技术特征，建议针对以下方面进行详细新颖性检索：\n", highImportance)
		for _, f := range features {
			if f.Importance == "high" {
				fmt.Fprintf(&b, "- %s\n", f.Description)
			}
		}
	default:
		b.WriteString("技术特征重要度均为中低，建议结合领域常规手段进行检索。\n")
	}

	b.WriteString("\n**注意：** 本评估为自动预分析，不构成正式新颖性判断。")
	b.WriteString("正式评估需要结合对比文件进行逐一比对。")

	// Determine conclusion.
	var conclusion string
	switch {
	case overall == 0:
		conclusion = "特征不足"
	case highImportance >= 3:
		conclusion = "待详细检索（重要特征较多）"
	case highImportance > 0:
		conclusion = "需针对性检索"
	default:
		conclusion = "可常规评估"
	}

	return &NoveltyResult{
		Assessed:   true,
		Conclusion: conclusion,
		Notes:      b.String(),
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
		defer agent.Close()
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
