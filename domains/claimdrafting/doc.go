// Package claimdrafting 实现独立的权利要求书撰写模块。
//
// 本模块提供规则驱动的权利要求生成、验证、评分及 LLM 增强撰写能力，
// 遵循《专利审查指南》《专利法实施细则》等规范要求。
//
// 核心组件：
//   - ClaimBuilder: 结构化权利要求构建器（五步法）
//   - RuleEngine:   可扩展的验证规则引擎
//   - LLMDrafter:   LLM 增强撰写器
//   - ClaimScorer:  质量评分器
//
// 输入数据与 disclosure 包中的 ExtractionResult 语义等价，但保持独立定义以避免循环依赖。
package claimdrafting
