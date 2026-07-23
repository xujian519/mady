package infringement

import (
	"context"
	"fmt"
	"strings"
)

// InfringementRule defines a deterministic check used alongside LLM analysis.
type InfringementRule interface {
	Name() string
	Description() string
	LegalBasis() string
	Severity() string
	Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error)
}

// RuleCheckInput carries the structured state for rule evaluation.
type RuleCheckInput struct {
	ClaimFeatures      []string
	ProductFeatures    []string
	FeatureMapping     []FeatureComparison
	LiteralResult      *LiteralResult
	EquivalenceResult  *EquivalenceResult
	ProsecutionHistory string
	PriorArtRefs       []string
}

// RuleCheckResult is the outcome of a single rule check.
type RuleCheckResult struct {
	RuleName    string   `json:"rule_name"`
	Passed      bool     `json:"passed"`
	Violations  []string `json:"violations,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// RuleEngine manages a collection of InfringementRule instances.
type RuleEngine struct {
	rules []InfringementRule
}

// NewRuleEngine creates a RuleEngine with all built-in rules.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules: allRules(),
	}
}

// Check runs all rules and returns only violations.
func (e *RuleEngine) Check(ctx context.Context, input *RuleCheckInput) []*RuleCheckResult {
	var results []*RuleCheckResult
	for _, r := range e.rules {
		res, err := r.Check(ctx, input)
		if err != nil {
			results = append(results, &RuleCheckResult{
				RuleName:   r.Name(),
				Passed:     false,
				Violations: []string{fmt.Sprintf("规则执行错误: %v", err)},
			})
			continue
		}
		if !res.Passed {
			results = append(results, res)
		}
	}
	return results
}

// Rules returns all registered rules.
func (e *RuleEngine) Rules() []InfringementRule {
	return e.rules
}

func allRules() []InfringementRule {
	return []InfringementRule{
		// L1 - Core determination rules
		&allElementsRule{},
		&claimConstructionRule{},
		&equivalenceTestRule{},
		&estoppelRule{},
		&dedicationRule{},
		// L2 - Defense rules
		&priorArtComparisonRule{},
		&priorUseTimingRule{},
		&legalSourceDualConditionRule{},
		&exhaustionBoundaryRule{},
		// L3 - Remedy rules
		&damageCascadeRule{},
		&preliminaryInjunctionFactorsRule{},
		&patentContributionRule{},
		// L4 - Strategy rules
		&jurisdictionRule{},
		&limitationPeriodRule{},
		&invalidationStayRule{},
	}
}

// --- L1: Core determination rules ---

type allElementsRule struct{}

func (r *allElementsRule) Name() string { return "all-elements-check" }
func (r *allElementsRule) Description() string {
	return "验证全部技术特征规则：被控方案是否包含权利要求的全部技术特征"
}
func (r *allElementsRule) LegalBasis() string {
	return "最高人民法院关于审理侵犯专利权纠纷案件应用法律若干问题的解释 第7条"
}
func (r *allElementsRule) Severity() string { return SeverityBlock }

func (r *allElementsRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	if input.LiteralResult == nil {
		return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
	}
	result := &RuleCheckResult{RuleName: r.Name(), Passed: true}
	if len(input.LiteralResult.MissingFeatures) > 0 {
		result.Passed = false
		for _, mf := range input.LiteralResult.MissingFeatures {
			result.Violations = append(result.Violations,
				fmt.Sprintf("缺少技术特征: %s — 字面侵权不成立，需进一步判断等同侵权", mf))
		}
	}
	if input.LiteralResult.AllElementsMet && len(input.LiteralResult.ExtraFeatures) > 0 {
		result.Suggestions = append(result.Suggestions,
			"被控方案包含额外技术特征不影响侵权认定（增加特征仍构成侵权）")
	}
	return result, nil
}

type claimConstructionRule struct{}

func (r *claimConstructionRule) Name() string { return "claim-construction" }
func (r *claimConstructionRule) Description() string {
	return "验证权利要求解释使用内部证据优先原则"
}
func (r *claimConstructionRule) LegalBasis() string { return "专利法 第59条第1款" }
func (r *claimConstructionRule) Severity() string   { return SeverityShould }

func (r *claimConstructionRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	if input.ProsecutionHistory != "" && len(input.ClaimFeatures) == 0 {
		return &RuleCheckResult{
			RuleName:   r.Name(),
			Passed:     false,
			Violations: []string{"审查历史可用但未在特征分解中引用——禁止反悔原则可能被遗漏"},
		}, nil
	}
	return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
}

type equivalenceTestRule struct{}

func (r *equivalenceTestRule) Name() string { return "equivalence-three-part-test" }
func (r *equivalenceTestRule) Description() string {
	return "验证等同判定使用手段/功能/效果三要素检验"
}
func (r *equivalenceTestRule) LegalBasis() string {
	return "最高人民法院关于审理专利纠纷案件适用法律问题的若干规定 第17条"
}
func (r *equivalenceTestRule) Severity() string { return SeverityMust }

func (r *equivalenceTestRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	if input.EquivalenceResult == nil || len(input.EquivalenceResult.EquivalentFeatures) == 0 {
		return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
	}
	result := &RuleCheckResult{RuleName: r.Name(), Passed: true}
	for _, ea := range input.EquivalenceResult.EquivalentFeatures {
		if ea.IsEquivalent && !ea.SameMeans && !ea.SameFunction && !ea.SameEffect {
			result.Passed = false
			result.Violations = append(result.Violations,
				fmt.Sprintf("特征 '%s' 被认定为等同但手段/功能/效果均不相同——存在逻辑矛盾", ea.ClaimFeature))
		}
	}
	return result, nil
}

type estoppelRule struct{}

func (r *estoppelRule) Name() string { return "prosecution-history-estoppel" }
func (r *estoppelRule) Description() string {
	return "验证禁止反悔原则：审查过程中放弃的技术方案不得通过等同原则重新主张"
}
func (r *estoppelRule) LegalBasis() string {
	return "最高人民法院关于审理侵犯专利权纠纷案件应用法律若干问题的解释 第6条"
}
func (r *estoppelRule) Severity() string { return SeverityMust }

func (r *estoppelRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	if input.ProsecutionHistory == "" {
		return &RuleCheckResult{
			RuleName:    r.Name(),
			Passed:      true,
			Suggestions: []string{"审查历史未提供——无法评估禁止反悔原则的适用可能性"},
		}, nil
	}
	if input.EquivalenceResult == nil || len(input.EquivalenceResult.EquivalentFeatures) == 0 {
		return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
	}
	if len(input.EquivalenceResult.EquivalentFeatures) > 0 &&
		!input.EquivalenceResult.EstoppelApplied &&
		input.EquivalenceResult.EstoppelDetails == "" {
		return &RuleCheckResult{
			RuleName:   r.Name(),
			Passed:     false,
			Violations: []string{"等同认定已做出但未审查禁止反悔原则——审查历史已提供的情况下应主动审查"},
		}, nil
	}
	return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
}

type dedicationRule struct{}

func (r *dedicationRule) Name() string { return "dedication-rule" }
func (r *dedicationRule) Description() string {
	return "验证捐献规则：仅在说明书中描述但未写入权利要求的技术方案不得通过等同原则重新纳入保护范围"
}
func (r *dedicationRule) LegalBasis() string {
	return "最高人民法院关于审理侵犯专利权纠纷案件应用法律若干问题的解释 第5条"
}
func (r *dedicationRule) Severity() string { return SeverityShould }

func (r *dedicationRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	if input.EquivalenceResult == nil {
		return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
	}
	if len(input.EquivalenceResult.EquivalentFeatures) > 0 &&
		!input.EquivalenceResult.DedicationApplied &&
		input.EquivalenceResult.DedicationDetails == "" {
		return &RuleCheckResult{
			RuleName:    r.Name(),
			Passed:      true,
			Suggestions: []string{"等同认定已做出——建议核实被控方案是否落入捐献规则范围"},
		}, nil
	}
	return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
}

// --- L2: Defense rules ---

type priorArtComparisonRule struct{}

func (r *priorArtComparisonRule) Name() string { return "prior-art-single-comparison" }
func (r *priorArtComparisonRule) Description() string {
	return "验证现有技术抗辩使用单独对比原则（不得组合多项现有技术）"
}
func (r *priorArtComparisonRule) LegalBasis() string { return "专利法 第62条" }
func (r *priorArtComparisonRule) Severity() string   { return SeverityMust }

func (r *priorArtComparisonRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	if len(input.PriorArtRefs) == 0 {
		return &RuleCheckResult{RuleName: r.Name(), Passed: true}, nil
	}
	result := &RuleCheckResult{RuleName: r.Name(), Passed: true}
	if len(input.PriorArtRefs) > 1 {
		result.Suggestions = append(result.Suggestions,
			fmt.Sprintf("提供了 %d 项现有技术——现有技术抗辩要求单独对比，需逐项独立分析", len(input.PriorArtRefs)))
	}
	return result, nil
}

type priorUseTimingRule struct{}

func (r *priorUseTimingRule) Name() string { return "prior-use-timing" }
func (r *priorUseTimingRule) Description() string {
	return "验证先用权的时间要件：必须在专利申请日（优先权日）之前"
}
func (r *priorUseTimingRule) LegalBasis() string { return "专利法 第69条第(二)项" }
func (r *priorUseTimingRule) Severity() string   { return SeverityMust }

func (r *priorUseTimingRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName:    r.Name(),
		Passed:      true,
		Suggestions: []string{"先用权抗辩需提供申请日前的制造/使用证据或必要准备证据"},
	}, nil
}

type legalSourceDualConditionRule struct{}

func (r *legalSourceDualConditionRule) Name() string { return "legal-source-dual-condition" }
func (r *legalSourceDualConditionRule) Description() string {
	return "验证合法来源抗辩需同时满足'不知道'和'合法来源'双重条件"
}
func (r *legalSourceDualConditionRule) LegalBasis() string { return "专利法 第70条" }
func (r *legalSourceDualConditionRule) Severity() string   { return SeverityMust }

func (r *legalSourceDualConditionRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"合法来源抗辩仅适用于使用者/销售者/许诺销售者，不适用于制造者",
			"需同时证明'不知道是侵权产品'和'产品具有合法来源'",
		},
	}, nil
}

type exhaustionBoundaryRule struct{}

func (r *exhaustionBoundaryRule) Name() string { return "exhaustion-boundary" }
func (r *exhaustionBoundaryRule) Description() string {
	return "验证权利用尽的边界条件：售出后的使用/转售不侵权，但重新制造不适用"
}
func (r *exhaustionBoundaryRule) LegalBasis() string { return "专利法 第69条第(一)项" }
func (r *exhaustionBoundaryRule) Severity() string   { return SeverityShould }

func (r *exhaustionBoundaryRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	result := &RuleCheckResult{RuleName: r.Name(), Passed: true}
	if strings.Contains(strings.ToLower(strings.Join(input.ProductFeatures, " ")), "回收") ||
		strings.Contains(strings.ToLower(strings.Join(input.ProductFeatures, " ")), "再造") {
		result.Suggestions = append(result.Suggestions,
			"检测到'回收'/'再造'关键词——权利用尽不适用于重新制造行为")
	}
	return result, nil
}

// --- L3: Remedy rules ---

type damageCascadeRule struct{}

func (r *damageCascadeRule) Name() string { return "damage-calculation-cascade" }
func (r *damageCascadeRule) Description() string {
	return "验证损害赔偿计算按法定四层递进顺序：实际损失→侵权所得→许可费倍数→法定赔偿"
}
func (r *damageCascadeRule) LegalBasis() string { return "专利法 第65条" }
func (r *damageCascadeRule) Severity() string   { return SeverityMust }

func (r *damageCascadeRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"损害赔偿计算须严格遵循法定递进顺序：实际损失→侵权所得→许可费倍数→法定赔偿(1万-500万)",
			"计算时应进行技术贡献率分割——专利技术仅体现在产品部分部件时需扣除其他因素贡献",
		},
	}, nil
}

type preliminaryInjunctionFactorsRule struct{}

func (r *preliminaryInjunctionFactorsRule) Name() string {
	return "preliminary-injunction-five-factors"
}
func (r *preliminaryInjunctionFactorsRule) Description() string {
	return "验证临时禁令申请需评估五大要素：侵权可能性/难以弥补的损害/双方困难权衡/担保/公共利益"
}
func (r *preliminaryInjunctionFactorsRule) LegalBasis() string { return "专利法 第66条" }
func (r *preliminaryInjunctionFactorsRule) Severity() string   { return SeverityMust }

func (r *preliminaryInjunctionFactorsRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"诉前禁令须提供担保，不提供则驳回申请",
			"申请人自法院采取措施之日起15日内不起诉的，法院应解除禁令",
			"实用新型/外观设计应提交专利权评价报告",
		},
	}, nil
}

type patentContributionRule struct{}

func (r *patentContributionRule) Name() string { return "patent-contribution-segmentation" }
func (r *patentContributionRule) Description() string {
	return "验证损害赔偿计算中进行了专利技术贡献率分割"
}
func (r *patentContributionRule) LegalBasis() string {
	return "最高人民法院关于审理侵犯专利权纠纷案件应用法律若干问题的解释 第16条"
}
func (r *patentContributionRule) Severity() string { return SeverityShould }

func (r *patentContributionRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"当专利技术仅体现在产品部分部件时，应以该部件价值为赔偿计算基数",
			"需区分侵权人营业利润(一般侵权)与销售利润(完全以侵权为业)",
		},
	}, nil
}

// --- L4: Strategy rules ---

type jurisdictionRule struct{}

func (r *jurisdictionRule) Name() string { return "jurisdiction-selection" }
func (r *jurisdictionRule) Description() string {
	return "验证诉讼管辖选择策略：被告所在地或侵权行为地"
}
func (r *jurisdictionRule) LegalBasis() string { return "民事诉讼法 第28条" }
func (r *jurisdictionRule) Severity() string   { return SeverityMay }

func (r *jurisdictionRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"可将制造者和销售者作为共同被告，通过销售地选择有利管辖法院",
			"专利侵权一审由省会中级法院或最高院指定的中院管辖",
		},
	}, nil
}

type limitationPeriodRule struct{}

func (r *limitationPeriodRule) Name() string { return "limitation-period-check" }
func (r *limitationPeriodRule) Description() string {
	return "验证诉讼时效：自得知或应知侵权行为之日起2年"
}
func (r *limitationPeriodRule) LegalBasis() string { return "专利法 第68条" }
func (r *limitationPeriodRule) Severity() string   { return SeverityShould }

func (r *limitationPeriodRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"诉讼时效为得知或应知侵权行为之日起2年",
			"持续性侵权行为在起诉日仍在继续的，可追究起诉前2年内的责任",
		},
	}, nil
}

type invalidationStayRule struct{}

func (r *invalidationStayRule) Name() string { return "invalidation-stay-strategy" }
func (r *invalidationStayRule) Description() string {
	return "验证无效宣告诉讼中止策略：被告可在答辩期满前提起无效宣告并申请中止审理"
}
func (r *invalidationStayRule) LegalBasis() string {
	return "最高人民法院关于审理专利纠纷案件适用法律问题的若干规定 第9-11条"
}
func (r *invalidationStayRule) Severity() string { return SeverityShould }

func (r *invalidationStayRule) Check(ctx context.Context, input *RuleCheckInput) (*RuleCheckResult, error) {
	return &RuleCheckResult{
		RuleName: r.Name(),
		Passed:   true,
		Suggestions: []string{
			"被告可向专利复审委提起无效宣告请求并申请法院中止侵权诉讼",
			"发明专利/复审委维持有效的专利：法院可以不中止",
			"未经复审的实用新型/外观设计：法院通常应当中止",
		},
	}, nil
}
