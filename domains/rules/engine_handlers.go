package rules

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/amendment"
)

// =============================================================================
// 工具 Handler（被 engine.go 中 Tools() 的闭包调用）
// =============================================================================

func (e *RulesExtension) handleSearch(args json.RawMessage) (any, error) {
	var p struct {
		Keyword string `json:"keyword"`
		Domain  string `json:"domain"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Keyword == "" && p.Domain == "" {
		return "请提供搜索关键词或规则域", nil
	}
	var rules []Rule
	if p.Domain != "" {
		rules = e.engine.RulesByDomain(p.Domain)
	} else {
		rules = e.engine.SearchRules(p.Keyword)
	}
	if len(rules) == 0 {
		return "未找到匹配的规则", nil
	}
	return formatRules(rules), nil
}

func (e *RulesExtension) handleArticle(args json.RawMessage) (any, error) {
	var p struct {
		ArticleID string `json:"article_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	af := e.engine.Article(p.ArticleID)
	if af == nil {
		return fmt.Sprintf("未找到法条框架: %s", p.ArticleID), nil
	}
	return formatArticle(af), nil
}

func (e *RulesExtension) handleOrchestration(args json.RawMessage) (any, error) {
	var p struct {
		CaseType string `json:"case_type"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	orch := e.engine.Orchestration(p.CaseType)
	if orch == nil {
		return fmt.Sprintf("未找到事务编排: %s", p.CaseType), nil
	}
	return formatOrchestration(orch), nil
}

// handleValidateAmendment 验证修改是否符合专利法第33条。
// 使用 amendment.Checker 做编译型检查，叠加 YAML 规则参考和 A33 法条框架。
func (e *RulesExtension) handleValidateAmendment(args json.RawMessage) (any, error) {
	var p struct {
		OriginalClaims   string `json:"original_claims"`
		OriginalSpec     string `json:"original_specification"`
		AmendedClaims    string `json:"amended_claims"`
		AmendedSpec      string `json:"amended_specification"`
		ModificationType string `json:"modification_type"`
		OfficeActionText string `json:"office_action_text"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}

	// Step 1: 将参数转为 amendment.CheckInput 并运行编译型检查
	modType := amendment.ModType(p.ModificationType)
	checkInput := amendment.CheckInput{
		OriginalClaims:   p.OriginalClaims,
		OriginalSpec:     p.OriginalSpec,
		AmendedClaims:    p.AmendedClaims,
		AmendedSpec:      p.AmendedSpec,
		ModificationType: modType,
		OfficeActionText: p.OfficeActionText,
	}
	checker := amendment.NewChecker()
	result := checker.Check(checkInput)

	// 填充 handler 层的信息
	result.OriginalLength = len(p.OriginalClaims) + len(p.OriginalSpec)
	result.AmendedLength = len(p.AmendedClaims) + len(p.AmendedSpec)

	// 如有 OA 文本，解析并附加摘要
	if p.OfficeActionText != "" && p.ModificationType == "passive" {
		oa := ParseOfficeAction(p.OfficeActionText)
		result.OfficeActionSummary = FormatOaSummary(oa)
	}

	// Step 2: 生成修改时机说明
	timingCheck := timingCheckText(p.ModificationType)

	// Step 3: 收集相关修改规则
	var relevantRules []Rule
	if e.engine != nil {
		relevantRules = e.engine.RulesByDomain("patent_amendment")
	}

	// Step 4: 生成 A33 法条框架参考
	var a33Ref string
	if e.engine != nil {
		if af := e.engine.Article("patent-law-a33"); af != nil {
			a33Ref = formatArticleShort(af)
		}
	}

	return formatAmendmentAnalysis(result, timingCheck, relevantRules, a33Ref), nil
}

// =============================================================================
// 辅助函数
// =============================================================================

// timingCheckText 根据修改类型生成时机合规性说明文本。
func timingCheckText(modType string) string {
	switch modType {
	case "active":
		return "主动修改：需在提出实审请求时或收到进入实审通知书之日起3个月内进行；" +
			"实用新型需在申请日起2个月内。请确认当前时间节点是否满足上述时限要求。"
	case "passive":
		return "被动修改：需针对审查意见通知书指出的缺陷进行修改。已提供OA文本时，请确认修改内容与驳回缺陷相关。"
	case "ex_officio":
		return "依职权修改：仅限文字和符号的明显错误，不能对保护范围产生实质影响。"
	default:
		return "未指定修改类型。请指定：active（主动）/ passive（被动）/ ex_officio（依职权）。"
	}
}

// formatArticleShort 返回法条框架的简要版（仅包含步骤标题和结论模式）
func formatArticleShort(af *ArticleFramework) string {
	var b strings.Builder
	fmt.Fprintf(&b, "法条: %s — %s\n", af.ArticleID, af.Name)
	b.WriteString("判断步骤:\n")
	for _, step := range af.Steps {
		fmt.Fprintf(&b, "  %d. %s\n", step.Order, step.Name)
	}
	b.WriteString("结论模式:\n")
	for k := range af.ConclusionSchema {
		fmt.Fprintf(&b, "  - %s\n", k)
	}
	return b.String()
}
