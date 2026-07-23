// Package amendment 提供专利申请文件修改合规性检查。
//
// 本模块实现了专利法第33条的修改不超范围规则，提供：
//   - AmendmentChecker：编译型规则检查器，用于验证修改是否超出原始记载范围
//   - 与 domains/rules YAML 规则引擎的集成
//   - 与 claimdrafting/specdrafting 规则引擎的集成点
//
// 架构定位：
//
//	本模块处于领域扩展层，介于 domains/claimdrafting 与 domains/specdrafting 之间。
//	修改检查横跨权利要求书和说明书两个文档，因此独立为一个模块，
//	而非嵌入 claimdrafting 或 specdrafting 内部。
//
// 检查流程：
//  1. 修改依据合法性检查（基于白名单：原始文件/引证文件/背景技术）
//  2. 边界规则检查（编译型，可自动执行的规则）
//     - 禁止以摘要附图/公开文本/优先权文件为依据
//     - 必要技术特征删除检测（基于重要性标记）
//  3. 综合判断指引（输出给 LLM，由 LLM 完成需要"本领域技术人员"视角的判断）
//
// 使用方式：
//
//	checker := amendment.NewChecker()
//	result := checker.Check(original, amended)
//
// 与 YAML 规则引擎联动：
//
//	checker.LoadRules(engine) // 从 domains/rules.Engine 加载更丰富的规则信息
//
// 本模块不重复实现 YAML 规则引擎中已有的灵活判断，而是提供
// 编译型规则 + YAML 规则衔接层。
package amendment
