package claimdrafting

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// ClaimBuilder 权利要求构建器
// =============================================================================

// ClaimBuilder 遵循五步法构建权利要求书。
type ClaimBuilder struct {
	domain   TechDomain
	priorArt string
	engine   *RuleEngine
}

// NewClaimBuilder 创建一个权利要求构建器。
// domain 为技术领域；priorArt 为最接近现有技术描述。
func NewClaimBuilder(domain TechDomain, priorArt string) *ClaimBuilder {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	return &ClaimBuilder{
		domain:   domain,
		priorArt: priorArt,
		engine:   engine,
	}
}

// RuleEngine 返回构建器关联的规则引擎。
func (b *ClaimBuilder) RuleEngine() *RuleEngine {
	return b.engine
}

// Build 执行完整的五步法构建流程，返回权利要求书输出。
//
// 五步流程：
//  1. 分析技术特征并分类（必要/附加、结构/方法）
//  2. 确定技术领域
//  3. 确定必要技术特征
//  4. 撰写独立权利要求（前序部分 + 特征部分）
//  5. 撰写从属权利要求（多层级布局）
func (b *ClaimBuilder) Build(input DraftInput) (*DraftOutput, error) {
	// 步骤1-3：分析特征并确定必要技术特征
	domain := b.domain
	if domain == "" {
		domain = classifyDomain(input)
	}
	essential, optional := classifyFeatures(input.Features, input.PFETriples)

	if len(essential) == 0 && len(input.Features) > 0 {
		// 降级：如果没有必要特征标记，将所有特征作为必要特征
		essential = make([]Feature, len(input.Features))
		copy(essential, input.Features)
		optional = nil
	}

	// 步骤4：撰写独立权利要求
	// 同步领域到 input（供规则引擎使用）
	input.TechDomain = domain
	indClaim, err := b.buildIndependent(input, domain, essential)
	if err != nil {
		return nil, fmt.Errorf("build independent claim: %w", err)
	}

	// 步骤5：撰写从属权利要求
	depClaims := b.buildDependents(indClaim, input, optional)

	allClaims := append([]Claim{indClaim}, depClaims...)

	// 规则验证
	violations := b.engine.Validate(allClaims, input)
	var warnings []string
	for _, v := range violations {
		if v.Severity == SeverityWarning || v.Severity == SeverityInfo {
			warnings = append(warnings, fmt.Sprintf("[%s] %s", v.RuleName, v.Message))
		}
	}

	claimSet := &ClaimSet{
		IndependentClaims: []Claim{indClaim},
		DependentClaims:   depClaims,
	}

	output := &DraftOutput{
		Claims:    claimSet,
		Warnings:  warnings,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	output.InputMeta.Domain = domain
	output.InputMeta.FeatureCount = len(input.Features)

	return output, nil
}

// buildIndependent 构建独立权利要求。
func (b *ClaimBuilder) buildIndependent(input DraftInput, domain TechDomain, essential []Feature) (Claim, error) {
	// 确定主题名称
	subject := b.determineSubject(input.Title, domain)

	// 构建前序部分（与最接近现有技术共有的必要技术特征）
	preamble := b.buildPreamble(subject, input, essential)

	// 构建特征部分（区别技术特征）
	characterized := b.buildCharacterized(input, essential)

	if characterized == "" {
		characterized = "[待确定：核心区别技术特征]"
	}

	return Claim{
		Number:        1,
		ClaimType:     determineClaimTypeByFeatures(essential),
		Kind:          "independent",
		Preamble:      preamble,
		Characterized: characterized,
	}, nil
}

// determineSubject 确定权利要求的主题名称。
func (b *ClaimBuilder) determineSubject(title string, domain TechDomain) string {
	if title != "" {
		return title
	}
	switch domain {
	case DomainMechanical:
		return "一种机械装置"
	case DomainElectrical:
		return "一种电路装置"
	case DomainChemical:
		return "一种组合物"
	case DomainSoftware:
		return "一种数据处理方法"
	default:
		return "一种技术方案"
	}
}

// buildPreamble 构建前序部分。
func (b *ClaimBuilder) buildPreamble(subject string, input DraftInput, essential []Feature) string {
	var commonParts []string

	// 从现有技术信息和共有特征构建前序部分
	for _, f := range essential {
		if f.PriorStatus == "known" {
			commonParts = append(commonParts, f.Description)
		}
	}

	if len(commonParts) == 0 && len(input.Problems) > 0 {
		// 如果无已知特征，使用问题上下文构建基础前序
		commonParts = append(commonParts, buildPreambleFromProblem(input.Problems[0]))
	}

	if len(commonParts) > 0 {
		return subject + "，包括" + strings.Join(commonParts, "，")
	}
	return subject
}

// buildCharacterized 构建特征部分。
func (b *ClaimBuilder) buildCharacterized(_ DraftInput, essential []Feature) string {
	var distinguishing []string
	for _, f := range essential {
		if f.PriorStatus == "unknown" || f.PriorStatus == "partial" {
			distinguishing = append(distinguishing, formatFeatureDesc(f))
		}
	}

	// 如果找不到区分特征，使用所有必要特征中未被标记为已知的
	if len(distinguishing) == 0 {
		for _, f := range essential {
			distinguishing = append(distinguishing, formatFeatureDesc(f))
		}
	}

	if len(distinguishing) == 0 {
		return ""
	}

	return strings.Join(distinguishing, "；")
}

// buildDependents 构建从属权利要求。
func (b *ClaimBuilder) buildDependents(indClaim Claim, input DraftInput, optional []Feature) []Claim {
	var deps []Claim
	claimNum := 2

	// 按重要性排序可选特征
	sorted := sortFeaturesByImportance(optional)

	// 布局策略：
	// 类型1：重要可选特征 → 直接引用独立权利要求
	// 类型2：进一步限定前序特征 → 引用独立权利要求
	// 类型3：结构细化 → 引用前一项从属权利要求

	var directRefs []string
	var chainRefs []string

	for _, f := range sorted {
		desc := formatFeatureDesc(f)
		if f.Importance == "high" {
			directRefs = append(directRefs, desc)
		} else {
			chainRefs = append(chainRefs, desc)
		}
	}

	// 类型1：直接引用独立权利要求
	for _, desc := range directRefs {
		deps = append(deps, Claim{
			Number:     claimNum,
			ClaimType:  indClaim.ClaimType,
			Kind:       "dependent",
			DependsOn:  []int{indClaim.Number},
			Limitation: desc,
		})
		claimNum++
	}

	// 类型2：引用独立权利要求（前序部分特征的进一步限定）
	for i, desc := range chainRefs {
		depOn := indClaim.Number
		if i > 0 && len(deps) > 0 {
			// 类型3：引用前一项从属权利要求（形成引用链）
			depOn = claimNum - 1
		}
		deps = append(deps, Claim{
			Number:     claimNum,
			ClaimType:  indClaim.ClaimType,
			Kind:       "dependent",
			DependsOn:  []int{depOn},
			Limitation: desc,
		})
		claimNum++
	}

	return deps
}

// =============================================================================
// 辅助函数
// =============================================================================

// classifyFeatures 将特征分类为必要特征和可选特征。
// 必要特征：直接关联到 PFE triple 且重要性为 high 的特征。
// 可选特征：其他特征（将放入从属权利要求）。
func classifyFeatures(features []Feature, triples []PFETriple) (essential, optional []Feature) {
	tripleFeatureIDs := make(map[string]bool)
	for _, t := range triples {
		for _, fid := range t.FeatureIDs {
			tripleFeatureIDs[fid] = true
		}
	}

	for _, f := range features {
		if tripleFeatureIDs[f.ID] && f.Importance == "high" {
			essential = append(essential, f)
		} else {
			optional = append(optional, f)
		}
	}
	return
}

// determineClaimTypeByFeatures 根据特征类型判断权利要求类型。
func determineClaimTypeByFeatures(features []Feature) ClaimType {
	for _, f := range features {
		if f.Category == "method" {
			return ClaimTypeMethod
		}
	}
	return ClaimTypeProduct
}

// classifyDomain 根据输入推断技术领域。
func classifyDomain(input DraftInput) TechDomain {
	// 基于特征类别统计
	catCount := make(map[string]int)
	for _, f := range input.Features {
		catCount[f.Category]++
	}

	// 基于关键词检测
	allText := input.Title + " " + strings.Join(input.Problems, " ")
	for _, f := range input.Features {
		allText += " " + f.Description
	}

	mechKeywords := []string{"机械", "装置", "机构", "连接", "固定", "支撑", "壳体", "弹簧", "齿轮"}
	elecKeywords := []string{"电路", "电压", "电流", "信号", "电极", "导线", "半导体", "放大", "传感器"}
	chemKeywords := []string{"组合物", "化合物", "组分", "含量", "重量", "百分比", "摩尔", "催化剂"}
	softKeywords := []string{"数据", "方法", "步骤", "程序", "处理", "计算", "算法", "图像", "信号处理"}

	score := map[TechDomain]int{
		DomainMechanical: 0,
		DomainElectrical: 0,
		DomainChemical:   0,
		DomainSoftware:   0,
	}

	score[DomainMechanical] += countKeywords(allText, mechKeywords) + catCount["structure"]*2
	score[DomainElectrical] += countKeywords(allText, elecKeywords) + catCount["parameter"]*2
	score[DomainChemical] += countKeywords(allText, chemKeywords) + catCount["material"]*3
	score[DomainSoftware] += countKeywords(allText, softKeywords) + catCount["method"]*2

	var bestDomain TechDomain
	bestScore := 0
	for d, s := range score {
		if s > bestScore {
			bestScore = s
			bestDomain = d
		}
	}

	if bestScore == 0 {
		return DomainGeneral
	}
	return bestDomain
}

// countKeywords 统计文本中包含的关键词数量。
func countKeywords(text string, keywords []string) int {
	count := 0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			count++
		}
	}
	return count
}

// buildPreambleFromProblem 从技术问题构建前序部分基础。
func buildPreambleFromProblem(problem string) string {
	problem = strings.TrimPrefix(problem, "技术问题：")
	problem = strings.TrimPrefix(problem, "现有技术中")
	problem = strings.TrimSuffix(problem, "的问题")
	problem = strings.TrimSuffix(problem, "的缺陷")
	problem = strings.TrimSuffix(problem, "的不足")

	if len(problem) > 3 {
		return problem
	}
	return ""
}

// formatFeatureDesc 格式化技术特征为权利要求表述。
func formatFeatureDesc(f Feature) string {
	desc := strings.TrimSpace(f.Description)
	if desc == "" {
		return "[特征]"
	}
	if f.Function != "" {
		return desc + "，用于" + f.Function
	}
	return desc
}

// sortFeaturesByImportance 按重要性排序特征：high → medium → low。
func sortFeaturesByImportance(features []Feature) []Feature {
	order := map[string]int{"high": 0, "medium": 1, "low": 2}
	sorted := make([]Feature, len(features))
	copy(sorted, features)
	sort.Slice(sorted, func(i, j int) bool {
		return order[sorted[i].Importance] < order[sorted[j].Importance]
	})
	return sorted
}
