// Package disclosure 实现技术交底书（Invention Disclosure）分析管线，
// 基于 Pregel 图引擎驱动 10 个分析节点对专利交底书进行结构化解析。
//
// 分析管线节点：
//   - Preprocess: 文本预处理与段落分割
//   - Extract: 关键信息抽取（技术问题/方案/效果）
//   - Graph: 技术要素关系图构建
//   - Novelty: 新颖性/创造性初步评估
//   - Consistency: 技术方案一致性校验
//   - Evidence: 支持证据链构建
//   - Report: 分析报告生成与导出
//
// 主要类型：
//   - Disclosure: 交底书完整表示（结构化字段+原始文本）
//   - Graph: 技术要素关系图
//   - Report: 分析报告（含新颖性/一致性评估）
//   - Evidence: 证据链（要素-原文引用映射）
//
// 使用示例：
//
//	d, _ := disclosure.New("path/to/disclosure.txt")
//	report, err := d.Analyze(ctx)
package disclosure
