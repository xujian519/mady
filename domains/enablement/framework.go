package enablement

import (
	"strings"

	"github.com/xujian519/mady/domains/analysiskit"
)

// =============================================================================
// ArticleFramework 查询（骨架复用 analysiskit，见 domains/analysiskit）
// =============================================================================

// ArticleFrameworkProvider 是法条框架查询的抽象接口（复用 analysiskit）。
type ArticleFrameworkProvider = analysiskit.ArticleFrameworkProvider

// ArticleFrameworkData 是法条框架的纯数据镜像（复用 analysiskit）。
type ArticleFrameworkData = analysiskit.ArticleFrameworkData

// ArticleStepData 是单步判断步骤的数据镜像（复用 analysiskit）。
type ArticleStepData = analysiskit.ArticleStepData

// Framework 是充分公开（A26.3）判断框架查询器（复用 analysiskit）。
type Framework = analysiskit.Framework

// NewFramework 创建绑定到 ArticleFrameworkProvider 的 Framework 查询器。
// provider 为 nil 时降级为内置默认框架文本。
func NewFramework(provider ArticleFrameworkProvider) *Framework {
	return analysiskit.NewFramework(provider, []string{"patent-law-a26.3", "A26.3"}, defaultA263Framework)
}

// defaultA263Framework 返回内嵌的默认 A26.3 判断框架。
// 当 rules.Engine 未加载 YAML 时作为降级方案。
func defaultA263Framework() string {
	return strings.Join([]string{
		"## 专利法第26条第3款——说明书充分公开判断框架",
		"",
		"**法条原文**：《中华人民共和国专利法》（2020 年修正）第 26 条第 3 款",
		"「说明书应当对发明或者实用新型作出清楚、完整的说明，以所属技术领域的技术人员能够实现为准。」",
		"",
		"**审查指南依据**：审查指南（2023 修订）第二部分第二章第 2.1 节",
		"",
		"### 判断步骤",
		"",
		"**第 1 步：检查说明书结构完整性**",
		"- 核对 5 项必要章节：技术领域、背景技术、发明内容、附图说明、具体实施方式",
		"- 缺失任一项即为结构不完整",
		"",
		"**第 2 步：检查说明书清楚性**",
		"- 技术术语是否含义明确、无歧义",
		"- 是否存在自造词（非领域常规术语且未给出定义）",
		"- 是否存在明显错误（只有唯一正确理解时才不影响充分公开）",
		"- PFE 因果链（问题→特征→效果）是否闭环",
		"- 是否存在孤立特征（无对应效果）或孤立效果（无对应特征）",
		"",
		"**第 3 步：检查能够实现性（核心标准）**",
		"- 本领域技术人员根据说明书记载能否无需创造性劳动即可实施",
		"- 「能够实现」要求三者同时满足：实现技术方案 + 解决技术问题 + 产生预期效果",
		"- 逐一检测六种公开不充分情形（审查指南 §2.1.3）：",
		"  1. 仅给出任务/设想，未给出任何技术手段",
		"  2. 技术手段含糊不清，无法具体实施",
		"  3. 给出了技术手段，但不能解决技术问题（如违背物理原理）",
		"  4. 多手段方案中某一手段不能实现",
		"  5. 缺少关键技术手段的说明",
		"  6. 方案须依赖实验结果但未给出实验证据",
		"",
		"### 结论模式",
		"- isSufficient: bool — 是否满足 26.3 充分公开要求",
		"- reasoning: string — 推理过程，引用审查指南条款和说明书段落",
		"- confidence: high/medium/low",
		"- deficiencies: []string — 具体缺陷清单",
		"",
		"**注意**：本判断由 AI 辅助生成，不构成正式法律意见。",
	}, "\n")
}
