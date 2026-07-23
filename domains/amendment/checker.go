package amendment

import (
	"fmt"
	"strings"
)

// =============================================================================
// AmendmentChecker 修改合规性检查器
// =============================================================================

// AmendmentChecker 提供编译型修改规则检查。
// 负责"边界明确"的规则，以及为需要判断的规则提供结构化对比信息。
type AmendmentChecker struct{}

// NewChecker 创建 AmendmentChecker。
func NewChecker() *AmendmentChecker {
	return &AmendmentChecker{}
}

// Check 对输入执行所有编译型检查规则，返回完整检查结果。
func (c *AmendmentChecker) Check(input CheckInput) *CheckResult {
	result := &CheckResult{
		HasClaimChanges: input.OriginalClaims != "" &&
			input.AmendedClaims != "" &&
			input.OriginalClaims != input.AmendedClaims,
		HasSpecChanges: input.OriginalSpec != "" &&
			input.AmendedSpec != "" &&
			input.OriginalSpec != input.AmendedSpec,
		ModType: input.ModificationType,
	}

	// 完全无输入时直接返回
	if input.OriginalClaims == "" && input.OriginalSpec == "" &&
		input.AmendedClaims == "" && input.AmendedSpec == "" {
		result.Note = "未提供任何修改数据"
		result.IsCompliant = true
		return result
	}

	// 运行所有检查
	result.Violations = append(result.Violations, c.checkBasicInput(input)...)
	result.Violations = append(result.Violations, c.checkPassiveOA(input)...)

	result.TotalViolations = len(result.Violations)
	result.IsCompliant = result.TotalViolations == 0

	if !result.IsCompliant {
		var msgs []string
		for _, v := range result.Violations {
			msgs = append(msgs, fmt.Sprintf("[%s] %s", v.RuleName, v.Message))
		}
		result.Note = "发现修改合规问题：\n" + strings.Join(msgs, "\n")
	} else {
		result.Note = "编译型规则检查通过。需要人工判断（如是否超范围）的内容，请结合原始文件及 A33 规则框架综合评估。"
	}

	return result
}

// checkBasicInput 检查输入完整性。
func (c *AmendmentChecker) checkBasicInput(input CheckInput) []Violation {
	if input.OriginalClaims == "" && input.OriginalSpec == "" &&
		input.AmendedClaims != "" {
		return []Violation{{
			RuleName:  "amendment-basic-input",
			Severity:  "warning",
			Message:   "提供了修改后的文件但未提供原始文件，无法进行对比检查",
			Recommend: "请同时提供原始权利要求书和/或原始说明书内容",
		}}
	}
	return nil
}

// checkPassiveOA 检查被动修改的 OA 文本是否提供。
func (c *AmendmentChecker) checkPassiveOA(input CheckInput) []Violation {
	if input.ModificationType != ModPassive {
		return nil
	}
	if input.OfficeActionText == "" {
		return []Violation{{
			RuleName:  "amendment-passive-oa-required",
			Severity:  "warning",
			Message:   "被动修改（OA答复）应提供审查意见通知书文本，以验证修改是否针对通知书指出的缺陷",
			Recommend: "请提供审查意见通知书原文，或确认修改内容确实针对审查意见指出的缺陷",
		}}
	}
	// 拒绝关键词快速检测
	rejectionKeywords := []string{"创造性", "新颖性", "不清楚", "不支持", "公开不充分", "修改超范围", "33条", "26条"}
	oaText := strings.ToLower(input.OfficeActionText)
	for _, kw := range rejectionKeywords {
		if strings.Contains(oaText, strings.ToLower(kw)) {
			return nil
		}
	}
	return []Violation{{
		RuleName:  "amendment-passive-oa-content",
		Severity:  "info",
		Message:   "提供的审查意见文本中未能自动检测到驳回理由关键词，请确认文本内容完整",
		Recommend: "请确认审查意见通知书全文已提供，或手动判断驳回类型",
	}}
}
