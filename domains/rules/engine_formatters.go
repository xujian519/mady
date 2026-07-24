package rules

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/amendment"
)

// =============================================================================
// 格式化函数（将规则/法条/编排/修改验证 输出为可读文本）
// =============================================================================

// formatRules 格式化规则列表。
func formatRules(rules []Rule) string {
	var b strings.Builder
	for _, r := range rules {
		fmt.Fprintf(&b, "### %s — %s\n", r.RuleID, r.Name)
		fmt.Fprintf(&b, "- 描述: %s\n", r.Description)
		fmt.Fprintf(&b, "- 法律依据: %s\n", r.LegalBasis)
		fmt.Fprintf(&b, "- 域: %s\n", r.Domain)
		fmt.Fprintf(&b, "- 严重度: %s | 动作: %s\n", r.Severity, r.Action)
		fmt.Fprintf(&b, "- 检查类型: %s\n", r.Check.Type)
		if len(r.Check.Principles) > 0 {
			b.WriteString("- 原则:\n")
			for _, p := range r.Check.Principles {
				fmt.Fprintf(&b, "  - %s\n", p)
			}
		}
		if len(r.Check.Rules) > 0 {
			b.WriteString("- 规则:\n")
			for _, r2 := range r.Check.Rules {
				fmt.Fprintf(&b, "  - %s\n", r2)
			}
		}
		if len(r.Check.Assessment) > 0 {
			b.WriteString("- 评估:\n")
			for k, v := range r.Check.Assessment {
				fmt.Fprintf(&b, "  - %s → %s\n", k, v)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// formatArticle 格式化法条框架。
func formatArticle(af *ArticleFramework) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n", af.ArticleID, af.Name)
	fmt.Fprintf(&b, "法律依据: %s\n", af.LawRef)
	if af.GuidelineRef != "" {
		fmt.Fprintf(&b, "审查指南: %s\n", af.GuidelineRef)
	}
	b.WriteString("\n## 判断步骤\n")
	for _, step := range af.Steps {
		fmt.Fprintf(&b, "### 步骤%d: %s\n", step.Order, step.Name)
		fmt.Fprintf(&b, "规则参考: %s\n", step.RuleRef)
		fmt.Fprintf(&b, "输入提示: %s\n", step.InputHint)
		if len(step.OutputSchema) > 0 {
			b.WriteString("输出:\n")
			for k, v := range step.OutputSchema {
				fmt.Fprintf(&b, "  - %s: %s\n", k, v)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 结论模式\n")
	for k, v := range af.ConclusionSchema {
		fmt.Fprintf(&b, "- %s: %s\n", k, v)
	}
	return b.String()
}

// formatOrchestration 格式化事务编排方案。
func formatOrchestration(orch *Orchestration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n", orch.ID, orch.Name)
	fmt.Fprintf(&b, "事务类型: %s\n", orch.CaseType)
	fmt.Fprintf(&b, "描述: %s\n\n", orch.Description)
	b.WriteString("## 发现阶段\n")
	for i, stage := range orch.DiscoveryStages {
		fmt.Fprintf(&b, "### %d. %s\n", i+1, stage.Name)
		fmt.Fprintf(&b, "目标: %s\n", stage.Goal)
		if len(stage.Suggestions) > 0 {
			b.WriteString("建议:\n")
			for _, s := range stage.Suggestions {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## 可用法条\n")
	for _, aa := range orch.AvailableArticles {
		fmt.Fprintf(&b, "%d. %s — %s\n", aa.Priority, aa.ArticleID, aa.Description)
	}
	fmt.Fprintf(&b, "\n## 执行模板\n")
	fmt.Fprintf(&b, "产出物: %s\n", orch.ExecutionTemplate.ArtifactType)
	fmt.Fprintf(&b, "章节:\n")
	for _, s := range orch.ExecutionTemplate.Sections {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	return b.String()
}

// formatAmendmentAnalysis 格式化修改验证报告。
// 参数来自 amendment.Checker 的检查结果 + handler 层附加信息。
func formatAmendmentAnalysis(result *amendment.CheckResult, timingCheck string, rules []Rule, a33Ref string) string {
	if result == nil {
		return "未提供修改验证数据"
	}
	var b strings.Builder
	b.WriteString("# 修改不超范围验证报告\n\n")

	// 基本信息
	b.WriteString("## 基本信息\n")
	//nolint:gosec // 中文业务标签，非硬编码凭据
	modTypeLabel := map[string]string{
		"active": "主动修改", "passive": "被动修改（审查意见答复）", "ex_officio": "依职权修改",
	}
	label := modTypeLabel[string(result.ModType)]
	if label == "" {
		label = string(result.ModType)
	}
	fmt.Fprintf(&b, "- 修改类型: %s\n", label)
	fmt.Fprintf(&b, "- 原始文件大小: %d 字符\n", result.OriginalLength)
	fmt.Fprintf(&b, "- 修改后文件大小: %d 字符\n", result.AmendedLength)
	fmt.Fprintf(&b, "- 权利要求修改: %s\n", boolYesNo(result.HasClaimChanges))
	fmt.Fprintf(&b, "- 说明书修改: %s\n\n", boolYesNo(result.HasSpecChanges))

	// 修改时机合规性
	if timingCheck != "" {
		b.WriteString("## 修改时机合规性\n")
		b.WriteString(timingCheck)
		b.WriteString("\n\n")
	}

	// 编译型检查结果
	if len(result.Violations) > 0 {
		b.WriteString("## 编译型规则检查发现的问题\n")
		sevLabel := map[string]string{"error": "🔴", "warning": "🟡", "info": "ℹ️"}
		for _, v := range result.Violations {
			fmt.Fprintf(&b, "%s [%s] %s\n", sevLabel[v.Severity], v.RuleName, v.Message)
			if v.Recommend != "" {
				fmt.Fprintf(&b, "  建议: %s\n", v.Recommend)
			}
		}
		b.WriteString("\n")
	}

	// OA 解析摘要
	if result.OfficeActionSummary != "" {
		b.WriteString("## 审查意见通知书解析\n")
		b.WriteString(result.OfficeActionSummary)
		b.WriteString("\n\n")
	}

	// A33 法条框架参考
	if a33Ref != "" {
		b.WriteString("## 法条框架参考（专利法第33条）\n")
		b.WriteString(a33Ref)
		b.WriteString("\n\n")
	}

	// 相关修改规则（YAML）
	if len(rules) > 0 {
		b.WriteString("## 相关修改规则\n")
		b.WriteString("以下规则适用于本案修改判断，请结合原始文件内容逐条检查：\n\n")
		for _, r := range rules {
			sevLabel := map[string]string{
				"critical": "🔴 严重", "major": "🟡 主要",
				"minor": "🟢 次要", "info": "ℹ️ 参考",
			}
			fmt.Fprintf(&b, "### %s [%s]\n", r.Name, sevLabel[string(r.Severity)])
			fmt.Fprintf(&b, "- 规则ID: %s | 动作: %s\n", r.RuleID, r.Action)
			if len(r.Check.Principles) > 0 {
				b.WriteString("- 原则:\n")
				for _, p := range r.Check.Principles {
					fmt.Fprintf(&b, "  - %s\n", p)
				}
			}
			if len(r.Check.Rules) > 0 {
				b.WriteString("- 规则:\n")
				for _, rule := range r.Check.Rules {
					fmt.Fprintf(&b, "  - %s\n", rule)
				}
			}
			b.WriteString("\n")
		}
	}

	// 检查清单
	b.WriteString("## 修改合规检查清单\n")
	b.WriteString("请 LLM 逐项评估：\n")
	b.WriteString("1. [ ] 修改依据合法性：修改内容是否可在原始文件中找到依据（文字记载或直接确定）\n")
	b.WriteString("2. [ ] 修改未使用摘要/附图/PCT公开文本等非法依据\n")
	b.WriteString("3. [ ] 删除的特征是否为非必要技术特征\n")
	b.WriteString("4. [ ] 增加的特征是否属于可直接确定的范围\n")
	b.WriteString("5. [ ] 术语替换是否含义相同或更准确\n")
	b.WriteString("6. [ ] 被动修改时是否针对审查意见指出的缺陷\n")
	b.WriteString("7. [ ] 修改时机是否在法定期限内\n")

	b.WriteString("\n---\n")
	b.WriteString("⚠️ 本报告由 AI 辅助生成，基于已加载的修改规则库，仅供参考。")
	b.WriteString("\n  最终修改是否超范围应以原始申请文件记载为基础，参照审查指南进行判断。")

	return b.String()
}

// boolYesNo 将布尔值转为中文"是/否"。
func boolYesNo(v bool) string {
	if v {
		return "是"
	}
	return "否"
}
