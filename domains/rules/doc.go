// Package rules 实现 YAML 规则引擎，支持 OA（Office Action）审查意见
// 解析器和反套话引擎（Slop Engine），用于专利审查意见的自动化处理。
//
// 主要功能：
//   - YAML 规则加载与校验（Loader）
//   - 规则匹配与执行引擎（Engine）
//   - OA 审查意见解析（OAParser）
//   - 反套话检测（SlopEngine）
//
// 主要类型：
//   - Engine: 规则引擎，匹配条件和执行动作
//   - OAParser: 专利审查意见结构化解析
//   - SlopEngine: 套话/模板化表述检测
//   - Rule: 规则定义（条件-动作对）
//
// 使用示例：
//
//	engine, _ := rules.NewEngine("path/to/rules.yaml")
//	result, _ := engine.Evaluate(ctx, facts)
package rules
