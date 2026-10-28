// Package analysisreport 把专利域各引擎的「判断型结论」汇总为统一、可审计的分析产物。
//
// 输入包括：权利要求与摘要、checker 失败明细、IPC 分类、graph 三性结论、证据收据数、
// 知识图谱统计，以及调用方经模型补充的评分（novelty / technical_strength 等）。
// 输出包括：技术特征抽取与分布统计、多维质量评分（确定性 + 模型补分）、checker 聚合
// 判级、创新性洞察与专家考虑因素。
//
// 纯确定性聚合：clarity / completeness 由规则推导；novelty / technical_strength
// 由调用方经模型打分后注入。本包不持有上下文、不调用模型。
package analysisreport
