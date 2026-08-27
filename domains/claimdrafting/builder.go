package claimdrafting

import (
	"fmt"
	"strings"
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
		essential = make([]Feature, len(input.Features))
		copy(essential, input.Features)
		optional = nil
	}

	input.TechDomain = domain

	// 步骤4：撰写独立权利要求（支持并列独立权利要求策略）
	indClaims, err := b.buildIndependentClaims(input, domain, essential)
	if err != nil {
		return nil, fmt.Errorf("build independent claims: %w", err)
	}

	// 步骤5：撰写从属权利要求
	depClaims := b.buildDependents(indClaims, input, optional)

	allClaims := make([]Claim, 0, len(indClaims)+len(depClaims))
	allClaims = append(allClaims, indClaims...)
	allClaims = append(allClaims, depClaims...)

	// 规则验证
	violations := b.engine.Validate(allClaims, input)
	var warnings []string
	for _, v := range violations {
		if v.Severity == SeverityWarning || v.Severity == SeverityInfo {
			warnings = append(warnings, fmt.Sprintf("[%s] %s", v.RuleName, v.Message))
		}
	}

	output := &DraftOutput{
		Claims:    &ClaimSet{IndependentClaims: indClaims, DependentClaims: depClaims},
		Warnings:  warnings,
		Timestamp: timestamp(),
	}
	output.InputMeta.Domain = domain
	output.InputMeta.FeatureCount = len(input.Features)

	return output, nil
}

// buildIndependentClaims 根据撰写策略构建一个或多个独立权利要求。
func (b *ClaimBuilder) buildIndependentClaims(input DraftInput, domain TechDomain, essential []Feature) ([]Claim, error) {
	primary, err := b.buildIndependent(input, domain, essential)
	if err != nil {
		return nil, err
	}
	claims := []Claim{primary}
	switch input.Strategy {
	case StrategyProductAndMethod:
		methodClaim := b.buildParallelMethod(input, domain, primary.Number, essential)
		if methodClaim != nil {
			claims = append(claims, *methodClaim)
		}
		// 软件领域：额外生成"用步骤限定的装置"产品权利要求
		if domain == DomainSoftware {
			if d := b.buildSoftwareApparatus(input, domain, primary.Number, methodClaim, essential); d != nil {
				claims = append(claims, *d)
			}
		}
	case StrategyProductAndManufacturing:
		if p := b.buildParallelManufacturing(input, domain, primary.Number, essential); p != nil {
			claims = append(claims, *p)
		}
	case StrategyProductAndUse:
		if p := b.buildParallelUse(input, domain, primary.Number); p != nil {
			claims = append(claims, *p)
		}
	case StrategyPharmaUse:
		if p := b.buildPharmaUse(input, domain, primary.Number); p != nil {
			claims = append(claims, *p)
		}
	case StrategyMarkush:
		if p := b.buildMarkush(input, domain, primary.Number, essential); p != nil {
			// 马库什类型替换主权利要求的ClaimType
			claims[0].ClaimType = ClaimTypeProduct
			claims = append(claims, *p)
		}
	}
	return claims, nil
}

// buildParallelMethod 生成"一种实施权利要求1的方法"式并列独立权利要求。
func (b *ClaimBuilder) buildParallelMethod(input DraftInput, domain TechDomain, primaryNum int, essential []Feature) *Claim {
	methodFeatures := filterFeaturesByCategory(essential, "method")
	if len(methodFeatures) == 0 {
		methodFeatures = filterFeaturesByCategory(input.Features, "method")
	}
	if len(methodFeatures) == 0 {
		return nil
	}
	subject := b.determineSubject(input.Title, domain)
	claimNum := primaryNum + 1
	preamble := fmt.Sprintf("一种实施权利要求%d所述%s的方法", primaryNum, subject)
	var steps []string
	for _, f := range methodFeatures {
		steps = append(steps, formatParallelFeatureDesc(f, "method"))
	}
	if len(steps) == 0 {
		steps = append(steps, "[待确定：方法步骤]")
	}
	return &Claim{Number: claimNum, ClaimType: ClaimTypeMethod, Kind: "independent",
		Preamble: preamble, Characterized: strings.Join(steps, "；")}
}

// buildParallelManufacturing 生成"一种制造权利要求1的产品的方法"式并列独立权利要求。
func (b *ClaimBuilder) buildParallelManufacturing(input DraftInput, domain TechDomain, primaryNum int, essential []Feature) *Claim {
	methodFeatures := filterFeaturesByCategory(essential, "method")
	if len(methodFeatures) == 0 {
		methodFeatures = filterFeaturesByCategory(input.Features, "method")
	}
	if len(methodFeatures) == 0 {
		return nil
	}
	subject := b.determineSubject(input.Title, domain)
	claimNum := primaryNum + 1
	preamble := fmt.Sprintf("一种制造权利要求%d所述%s的方法", primaryNum, subject)
	var steps []string
	for _, f := range methodFeatures {
		steps = append(steps, formatParallelFeatureDesc(f, "manufacturing"))
	}
	if len(steps) == 0 {
		steps = append(steps, "[待确定：制造步骤]")
	}
	return &Claim{Number: claimNum, ClaimType: ClaimTypeMethod, Kind: "independent",
		Preamble: preamble, Characterized: strings.Join(steps, "；")}
}

// buildParallelUse 生成"一种权利要求1所述[产品]的用途"式用途权利要求（化学/医药领域）。
func (b *ClaimBuilder) buildParallelUse(input DraftInput, domain TechDomain, primaryNum int) *Claim {
	claimNum := primaryNum + 1
	preamble := fmt.Sprintf("一种权利要求%d所述%s的用途", primaryNum, b.determineSubject(input.Title, domain))
	return &Claim{Number: claimNum, ClaimType: ClaimTypeMethod, Kind: "independent",
		Preamble: preamble, Characterized: "[待确定：用途]"}
}

// buildPharmaUse 生成瑞士型权利要求（医药第二适应症）。
//
// 格式："物质X在制备治疗Y病的药物中的应用"
// 法律依据：专利法第25条第1款第(三)项——疾病的诊断和治疗方法不授予专利权，
// 但"用于制备药物的应用"（瑞士型权利要求）属于可授权客体。
func (b *ClaimBuilder) buildPharmaUse(input DraftInput, domain TechDomain, primaryNum int) *Claim {
	subject := b.determineSubject(input.Title, domain)
	claimNum := primaryNum + 1

	// 从问题和效果推断适应症
	disease := "疾病Y"
	if len(input.Problems) > 0 {
		problem := input.Problems[0]
		problem = strings.TrimPrefix(problem, "治疗")
		problem = strings.TrimPrefix(problem, "解决")
		if len([]rune(problem)) > 2 && len([]rune(problem)) < 50 {
			disease = problem
		}
	}

	preamble := fmt.Sprintf("一种%s在制备治疗%s的药物中的应用", subject, disease)

	// 从特征中提取剂量/用法等进一步限定
	var qualifiers []string
	for _, f := range input.Features {
		if f.Category == "parameter" {
			qualifiers = append(qualifiers, formatFeatureDesc(f))
		}
	}
	var characterized string
	switch {
	case len(qualifiers) > 0:
		characterized = strings.Join(qualifiers, "；")
	case len(input.Effects) > 0:
		characterized = "所述药物用于" + input.Effects[0]
	default:
		characterized = "[待确定：药物用途的进一步限定]"
	}

	return &Claim{Number: claimNum, ClaimType: ClaimTypeMethod, Kind: "independent",
		Preamble: preamble, Characterized: characterized}
}

// buildMarkush 生成马库什权利要求（通式化合物 + 取代基定义）。
//
// 马库什权利要求 = 通式化合物 + R1/R2...取代基定义 + 条件/排除
// 法律依据：审查指南第二部分第十章§4.3——马库什权利要求。
//
// 格式：
//
//	式(I)化合物：A-R1-B
//	其中，R1选自：H、C1-C6烷基、...；R2选自：OH、卤素、...
//	前提是R1和R2不同时为H。
func (b *ClaimBuilder) buildMarkush(input DraftInput, domain TechDomain, primaryNum int, essential []Feature) *Claim {
	subject := b.determineSubject(input.Title, domain)
	// 主权利要求采用马库什通式格式
	preamble := fmt.Sprintf("一种如式(I)所示的%s", subject)

	// 从前序问题或特征构建通式
	core := "[待确定：核心母核]"
	var substituents []string
	for _, f := range essential {
		if f.Category == "material" || f.Category == "structure" {
			if core == "[待确定：核心母核]" {
				core = f.Description
			} else {
				substituents = append(substituents, formatFeatureDesc(f))
			}
		}
	}

	characterized := core
	if len(substituents) > 0 {
		characterized += "，其中，" + strings.Join(substituents, "；")
	}

	return &Claim{
		Number:        primaryNum + 1,
		ClaimType:     ClaimTypeProduct,
		Kind:          "independent",
		Preamble:      preamble,
		Characterized: characterized,
	}
}

// buildSoftwareApparatus 为软件领域生成"用步骤限定的装置"产品权利要求。
//
// 知识库要求：含计算机程序的发明可同时用方法权利要求和"用步骤限定的装置"
// 的产品权利要求保护。保护范围不变，但提供侵权场景覆盖。
//
// 格式：
//
//	"一种用于执行权利要求N所述方法的装置，包括：
//	  用于执行步骤A的模块/单元；
//	  用于执行步骤B的模块/单元；……"
func (b *ClaimBuilder) buildSoftwareApparatus(input DraftInput, domain TechDomain, primaryNum int, methodClaim *Claim, essential []Feature) *Claim {
	// 方法权利要求编号（primaryNum+1），装置权利要求引用方法权要
	methodClaimNum := primaryNum + 1
	// 装置权要编号：方法权要存在时+2，不存在时+1
	claimNum := primaryNum + 1
	if methodClaim != nil {
		claimNum = primaryNum + 2
	}
	subject := b.determineSubject(input.Title, domain)

	// 从方法特征构建"用于执行X步骤的模块"
	var modules []string
	for _, f := range essential {
		if f.Category == "method" {
			desc := strings.TrimSpace(f.Description)
			if desc != "" {
				modules = append(modules, "用于执行"+desc+"的单元")
			}
		}
	}
	// 如果方法权利要求有步骤描述，也从中提取
	if methodClaim != nil && len(modules) == 0 {
		steps := strings.Split(methodClaim.Characterized, "；")
		for _, s := range steps {
			s = strings.TrimSpace(s)
			if s != "" {
				modules = append(modules, "用于"+s+"的单元")
			}
		}
	}
	// 如果仍然为空，从所有特征中提取
	if len(modules) == 0 {
		for _, f := range input.Features {
			modules = append(modules, "用于"+strings.TrimSpace(f.Description)+"的模块")
		}
	}
	if len(modules) == 0 {
		modules = append(modules, "[待确定：功能模块]")
	}

	characterized := strings.Join(modules, "；")
	preamble := fmt.Sprintf("一种用于执行权利要求%d所述方法的%s，其特征在于，包括", methodClaimNum, subject)

	return &Claim{
		Number:        claimNum,
		ClaimType:     ClaimTypeProduct,
		Kind:          "independent",
		Preamble:      preamble,
		Characterized: characterized + "；其中，各单元分别用于执行所述方法中的对应步骤",
	}
}

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

// buildDependents 构建从属权利要求（金字塔型布局策略）。
//
// 布局策略遵循"从宽到窄"的梯度保护原则：
//
//	类型1（直接引用）：高重要性 + 高 PFE 关联度的特征 → 直接引用独立权利要求
//	类型2（前序限定）：中等重要性的特征 → 引用独立权利要求
//	类型3（递进链）：低重要性或细节性特征 → 引用前一项从属权利要求（形成引用链）
func (b *ClaimBuilder) buildDependents(indClaims []Claim, input DraftInput, optional []Feature) []Claim {
	claimNum := len(indClaims) + 1 // start after independent claims
	primaryInd := indClaims[0]

	// 构建特征 → PFE 关联度映射
	pfeCount := buildPFECountMap(input.PFETriples)

	// 按综合得分排序：重要性越高、PFE 关联数越多 → 越靠前
	sorted := sortFeaturesByScore(optional, pfeCount)

	// 将特征分为两个梯队
	//   tier1：高重要性 或 PFE 关联 ≥2 的特征 → 直接引用独立权利要求
	//   tier2：中低重要性特征 → 使用递进引用链
	var tier1 []Feature
	var tier2 []Feature
	for _, f := range sorted {
		if f.Importance == "high" || pfeCount[f.ID] >= 2 {
			tier1 = append(tier1, f)
		} else {
			tier2 = append(tier2, f)
		}
	}

	deps := make([]Claim, 0, len(tier1)+len(tier2))

	// 类型1：直接引用独立权利要求（tier1 特征）
	for _, f := range tier1 {
		deps = append(deps, Claim{
			Number:     claimNum,
			ClaimType:  primaryInd.ClaimType,
			Kind:       "dependent",
			DependsOn:  []int{primaryInd.Number},
			Limitation: formatFeatureDesc(f),
		})
		claimNum++
	}

	// 类型2→3：递进引用链（tier2 特征）
	//   第一个 tier2 特征引用独立权利要求
	//   后续特征依次引用前一从属权利要求，形成"从宽到窄"的递进链
	for i, f := range tier2 {
		depOn := primaryInd.Number
		if i > 0 && len(deps) > 0 {
			depOn = claimNum - 1
		}
		deps = append(deps, Claim{
			Number:     claimNum,
			ClaimType:  primaryInd.ClaimType,
			Kind:       "dependent",
			DependsOn:  []int{depOn},
			Limitation: formatFeatureDesc(f),
		})
		claimNum++
	}

	return deps
}
