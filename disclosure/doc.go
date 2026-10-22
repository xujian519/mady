// Package disclosure 实现技术交底书（Invention Disclosure）分析管线，
// 基于 Pregel 图引擎驱动 14 个分析节点对专利交底书进行结构化解析。
//
// 分析管线节点：
//   - preprocess: 文本预处理与章节切分
//   - extract_problem / extract_features / extract_effects: 并行抽取技术问题、
//     技术特征与技术效果（各写独立 state key）
//   - merge_extractions: 合并三路抽取结果并建立 PFE 三元组引用
//   - check_clarity: 检查技术特征描述清晰度
//   - groundedness_filter: 评估特征在原文中的依据，回写置信度
//   - check_consistency: 技术方案一致性校验，最多回退 2 轮
//   - generate_keywords / retrieve_prior_art: 生成检索关键词并检索现有技术证据
//   - check_novelty: 基于证据的新颖性评估（逐特征或 batch 模式）
//   - generate_report: 生成结构化分析报告
//   - review_gate: 人工复核关卡，以 Pregel 中断信号交还调用方
//   - draft_claims: 生成权利要求草案
//
// 主要类型：
//   - AnalysisReport: 结构化分析报告
//   - ExtractionResult: 技术问题/特征/效果抽取结果
//   - EvidenceChunk: 现有技术证据片段
//   - NoveltyResult: 新颖性评估结果
//
// 使用示例：
//
//	compiled, err := disclosure.BuildDisclosureAnalysisGraph(provider)
//	if err != nil {
//		return err
//	}
//	state, err := compiled.Run(ctx, graph.PregelState{disclosure.StateKeyInput: text})
//	if err != nil && !agentcore.IsInterrupt(err) {
//		return err
//	}
//	report := disclosure.ExtractReportFromState(state)
package disclosure
