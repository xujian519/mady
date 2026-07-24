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

	var allClaims []Claim
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
		Timestamp: time.Now().UTC().Format(time.RFC3339),
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

// filterFeaturesByCategory 按类别过滤特征列表。
func filterFeaturesByCategory(features []Feature, category string) []Feature {
	var result []Feature
	for _, f := range features {
		if f.Category == category {
			result = append(result, f)
		}
	}
	return result
}

// formatParallelFeatureDesc 格式化并列独立权利要求的特征描述。
func formatParallelFeatureDesc(f Feature, mode string) string {
	desc := strings.TrimSpace(f.Description)
	if desc == "" {
		return "[特征]"
	}
	if mode == "method" || mode == "manufacturing" {
		return desc + "步骤"
	}
	if f.Function != "" {
		return desc + "，用于" + f.Function
	}
	return desc
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
	var deps []Claim
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

// buildPFECountMap 构建特征ID到PFE三元组关联数的映射。
// 一个特征关联到越多PFE三元组，说明其在发明中越核心。
func buildPFECountMap(triples []PFETriple) map[string]int {
	counts := make(map[string]int)
	for _, t := range triples {
		for _, fid := range t.FeatureIDs {
			counts[fid]++
		}
	}
	return counts
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

// sortFeaturesByScore 按综合得分排序特征（金字塔型布局的基础排序）。
// 得分 = importance 权重 × PFE 关联数加权。
// 高得分特征应写入靠前的从属权利要求（保护范围较宽）。
// 低得分特征应写入靠后的从属权利要求（递进限定）。
func sortFeaturesByScore(features []Feature, pfeCount map[string]int) []Feature {
	importanceWeight := map[string]int{"high": 100, "medium": 50, "low": 10}
	sorted := make([]Feature, len(features))
	copy(sorted, features)
	sort.SliceStable(sorted, func(i, j int) bool {
		scoreI := importanceWeight[sorted[i].Importance] + pfeCount[sorted[i].ID]*15
		scoreJ := importanceWeight[sorted[j].Importance] + pfeCount[sorted[j].ID]*15
		if scoreI != scoreJ {
			return scoreI > scoreJ // 高分优先
		}
		// 同分时按描述长度降序（更具体的在前）
		if len(sorted[i].Description) != len(sorted[j].Description) {
			return len(sorted[i].Description) > len(sorted[j].Description)
		}
		// 最终兜底：按 ID 字典序，确保严格弱序
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}
