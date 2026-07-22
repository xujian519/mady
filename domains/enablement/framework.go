package enablement

import (
	"fmt"
	"strings"
)

// =============================================================================
// ArticleFramework 查询
// =============================================================================

// ArticleFrameworkProvider 是法条框架查询的抽象接口。
// 生产环境由 domains/rules.Engine 实现，测试/降级场景由 nil 实现。
// 使用接口而非直接引用 domains/rules 包，避免引入 transitive build 依赖。
type ArticleFrameworkProvider interface {
	Article(id string) ArticleFrameworkData
}

// ArticleFrameworkData 是法条框架的纯数据镜像（避免依赖 domains/rules 包）。
type ArticleFrameworkData struct {
	Name             string
	LawRef           string
	GuidelineRef     string
	Steps            []ArticleStepData
	ConclusionSchema map[string]string
	ApplicableTo     []string
}

// ArticleStepData 是单步判断步骤的数据镜像。
type ArticleStepData struct {
	Order        int
	Name         string
	InputHint    string
	OutputSchema map[string]string
}

// Framework 返回专利法第26条第3款的判断框架。
// provider 为 nil 时降级为内置默认框架文本。
type Framework struct {
	provider ArticleFrameworkProvider
}

// NewFramework 创建绑定到 ArticleFrameworkProvider 的 Framework 查询器。
func NewFramework(provider ArticleFrameworkProvider) *Framework {
	return &Framework{provider: provider}
}

// GetArticleFramework 返回 A26.3 的法条判断框架。
func (f *Framework) GetArticleFramework() string {
	if f.provider != nil {
		if af := f.provider.Article("patent-law-a26.3"); af.Name != "" {
			return formatArticleData(af)
		}
		if af := f.provider.Article("A26.3"); af.Name != "" {
			return formatArticleData(af)
		}
	}
	return defaultA263Framework()
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
		"- PFE 因果链（问题→特征→效果）是否闭环",
		"- 是否存在孤立特征（无对应效果）或孤立效果（无对应特征）",
		"",
		"**第 3 步：检查能够实现性（核心标准）**",
		"- 本领域技术人员根据说明书记载能否无需创造性劳动即可实施",
		"- 逐一检测四种公开不充分情形：",
		"  1. 缺少关键技术手段的说明",
		"  2. 技术手段含糊不清",
		"  3. 仅给出任务/设想，未给出具体技术手段",
		"  4. 实验数据不足以证明技术效果",
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

// formatArticleData 将 ArticleFrameworkData 格式化为 Markdown 文本。
func formatArticleData(af ArticleFrameworkData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", af.Name)
	fmt.Fprintf(&b, "**法条依据**：%s\n\n", af.LawRef)
	if af.GuidelineRef != "" {
		fmt.Fprintf(&b, "**审查指南依据**：%s\n\n", af.GuidelineRef)
	}

	b.WriteString("### 判断步骤\n\n")
	for _, step := range af.Steps {
		fmt.Fprintf(&b, "**第 %d 步：%s**\n", step.Order, step.Name)
		if step.InputHint != "" {
			fmt.Fprintf(&b, "- 输入：%s\n", step.InputHint)
		}
		for key, desc := range step.OutputSchema {
			fmt.Fprintf(&b, "- %s：%s\n", key, desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("### 结论模式\n\n")
	for key, desc := range af.ConclusionSchema {
		fmt.Fprintf(&b, "- %s：%s\n", key, desc)
	}

	if len(af.ApplicableTo) > 0 {
		fmt.Fprintf(&b, "\n**适用场景**：%s\n", strings.Join(af.ApplicableTo, "、"))
	}

	return b.String()
}
