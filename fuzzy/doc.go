// Package fuzzy 提供字符串模糊匹配引擎，支持 Levenshtein 编辑距离、
// Jaccard 相似度等算法，用于法律文本中的模糊检索和名称匹配。
//
// 主要功能：
//   - Levenshtein 编辑距离计算
//   - 基于相似度阈值的模糊匹配
//   - 中文文本支持（按字/按词匹配）
//   - 批量匹配与 Top-K 排序
//
// 主要类型：
//   - Matcher: 模糊匹配器，支持配置算法和阈值
//   - Result: 匹配结果（目标字符串、相似度分数）
//
// 使用示例：
//
//	m := fuzzy.NewMatcher(fuzzy.Levenshtein, 0.8)
//	results := m.Search(ctx, "计算机", []string{"电脑", "算计机", "计算器"})
package fuzzy
