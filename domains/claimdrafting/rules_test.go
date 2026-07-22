package claimdrafting

import (
	"testing"
)

// =============================================================================
// 清楚性规则测试
// =============================================================================

func TestClarityClaimTypeRule_Valid(t *testing.T) {
	rule := &clarityClaimTypeRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种电子装置"},
		{Number: 2, ClaimType: ClaimTypeProduct, Kind: "dependent", DependsOn: []int{1}, Limitation: "所述装置还包括显示模块"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestClarityClaimType_MixedType(t *testing.T) {
	rule := &clarityClaimTypeRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种通信装置及其方法"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for mixed claim type, got none")
	}
}

func TestClarityWordingRule_CleanText(t *testing.T) {
	rule := &clarityWordingRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种加热装置", Characterized: "工作温度大于200摄氏度"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for clean text, got %d", len(violations))
	}
}

func TestClarityWordingRule_UncertainTerms(t *testing.T) {
	rule := &clarityWordingRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种加热装置", Characterized: "在高温下工作"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for '高温' uncertain term, got none")
	}
}

func TestClarityForbiddenWords_Example(t *testing.T) {
	rule := &clarityForbiddenWordsRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种装置", Characterized: "例如使用螺丝固定"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for '例如' forbidden word, got none")
	}
}

func TestClarityReferenceRule_ForwardRef(t *testing.T) {
	rule := &clarityReferenceRule{}
	claims := []Claim{
		{Number: 3, Kind: "dependent", DependsOn: []int{5}, ClaimType: ClaimTypeProduct, Limitation: "还包括显示模块"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for forward reference, got none")
	}
}

func TestClarityReferenceChainRule_CycleDetection(t *testing.T) {
	rule := &clarityReferenceChainRule{baseRule: newBaseRule("clarity-reference-chain", "desc", "law")}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种装置"},
		{Number: 2, Kind: "dependent", DependsOn: []int{3}, ClaimType: ClaimTypeProduct, Limitation: "A"},
		{Number: 3, Kind: "dependent", DependsOn: []int{1, 2}, ClaimType: ClaimTypeProduct, Limitation: "B"},
	}
	violations := rule.Check(claims, DraftInput{})
	if !hasCyclicViolation(violations) {
		t.Errorf("expected cyclic dependency violation, got: %v", violations)
	}
}

func hasCyclicViolation(violations []Violation) bool {
	for _, v := range violations {
		if v.RuleName == "clarity-reference-chain" {
			return true
		}
	}
	return false
}

// =============================================================================
// 形式规范规则测试
// =============================================================================

func TestFormalityNumbering_Valid(t *testing.T) {
	rule := &formalityNumberingRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent"},
		{Number: 2, Kind: "dependent", DependsOn: []int{1}, ClaimType: ClaimTypeProduct},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestFormalityNumbering_InvalidOrder(t *testing.T) {
	rule := &formalityNumberingRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent"},
		{Number: 3, Kind: "dependent", DependsOn: []int{1}, ClaimType: ClaimTypeProduct},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for non-sequential numbering, got none")
	}
}

func TestFormalityPeriod_Valid(t *testing.T) {
	rule := &formalityPeriodRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种装置", Characterized: "包括A"},
	}
	// Claim.String() 自动添加句号结尾
	violations := rule.Check(claims, DraftInput{})
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestFormalityMultipleDependent_MultiToMulti(t *testing.T) {
	rule := &formalityMultipleDependentRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种装置"},
		{Number: 2, Kind: "dependent", DependsOn: []int{1, 3}, ClaimType: ClaimTypeProduct,
			Limitation: "A"},
		{Number: 3, Kind: "dependent", DependsOn: []int{1, 2}, ClaimType: ClaimTypeProduct,
			Limitation: "B"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for multiple-to-multiple reference, got none")
	}
}

func TestFormalityThemeConsistency_Mismatch(t *testing.T) {
	rule := &formalityThemeConsistencyRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种装置"},
		{Number: 2, ClaimType: ClaimTypeMethod, Kind: "dependent", DependsOn: []int{1}, Limitation: "A"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for theme type mismatch, got none")
	}
}

// =============================================================================
// 支持性规则测试
// =============================================================================

func TestSupportEmbodiment_NoDescription(t *testing.T) {
	rule := &supportEmbodimentRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种装置", Characterized: "包括处理单元，用于执行计算"},
	}
	violations := rule.Check(claims, DraftInput{Description: ""})
	if len(violations) == 0 {
		t.Error("expected violations for functional feature without embodiment, got none")
	}
}

func TestSupportPureFunctional_StructuralPresent(t *testing.T) {
	rule := &supportPureFunctionalRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种装置", Characterized: "包括处理器和存储器"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) != 0 {
		t.Errorf("expected 0 violations when structural features present, got %d", len(violations))
	}
}

func TestSupportPureFunctional_PureFunctional(t *testing.T) {
	rule := &supportPureFunctionalRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种实现优化的方法", Characterized: "用于提升效率"},
	}
	violations := rule.Check(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations for pure functional claim, got none")
	}
}

// =============================================================================
// 必要技术特征规则测试
// =============================================================================

func TestNecessityCompleteness_AllFeaturesIncluded(t *testing.T) {
	rule := &necessityCompletenessRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种装置", Characterized: "包括A和B"},
	}
	input := DraftInput{
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "效率低", FeatureIDs: []string{"f1", "f2"}},
		},
		Features: []Feature{
			{ID: "f1", Description: "A", Importance: "high"},
			{ID: "f2", Description: "B", Importance: "high"},
		},
	}
	violations := rule.Check(claims, input)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations when all features covered, got %d: %v", len(violations), violations)
	}
}

func TestNecessityCompleteness_MissingFeatures(t *testing.T) {
	rule := &necessityCompletenessRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种装置", Characterized: "包括A"},
	}
	input := DraftInput{
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "效率低", FeatureIDs: []string{"f1", "f2"}},
		},
		Features: []Feature{
			{ID: "f1", Description: "A", Importance: "high"},
			{ID: "f2", Description: "B", Importance: "high"},
		},
	}
	violations := rule.Check(claims, input)
	if len(violations) == 0 {
		t.Error("expected violations when features are missing, got none")
	}
}

// =============================================================================
// 领域特定规则测试
// =============================================================================

func TestDomainMechanical_ConfigRelation(t *testing.T) {
	rule := &domainMechanicalRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种机械装置", Characterized: "包括A"},
	}
	input := DraftInput{TechDomain: DomainMechanical}
	violations := rule.Check(claims, input)
	if len(violations) == 0 {
		t.Error("expected violations for mechanical claim without configuration relation, got none")
	}
}

func TestDomainChemical_ContentPercentage(t *testing.T) {
	rule := &domainChemicalRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种组合物", Characterized: "包括组分A、组分B"},
	}
	input := DraftInput{TechDomain: DomainChemical}
	violations := rule.Check(claims, input)
	if len(violations) == 0 {
		t.Error("expected violations for chemical claim without percentage, got none")
	}
}

func TestDomainChemical_WithPercentage(t *testing.T) {
	rule := &domainChemicalRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种组合物", Characterized: "包括A 50%、B 30%"},
	}
	input := DraftInput{TechDomain: DomainChemical}
	violations := rule.Check(claims, input)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestDomainUtilityModel_MethodClaim(t *testing.T) {
	rule := &domainUtilityModelRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeMethod, Kind: "independent", Preamble: "一种制造方法"},
	}
	input := DraftInput{Title: "一种实用新型装置"}
	violations := rule.Check(claims, input)
	if len(violations) == 0 {
		t.Error("expected violations for utility model with method claim, got none")
	}
}

// =============================================================================
// 规则引擎集成测试
// =============================================================================

func TestRuleEngine_Validate(t *testing.T) {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)

	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种加热装置", Characterized: "在高温下工作"},
		{Number: 2, Kind: "dependent", DependsOn: []int{1, 3}, ClaimType: ClaimTypeProduct,
			Limitation: "最好使用金属材料"},
	}

	violations := engine.Validate(claims, DraftInput{})
	if len(violations) == 0 {
		t.Error("expected violations from registered rules, got none")
	}

	ruleNames := make(map[string]bool)
	for _, v := range violations {
		ruleNames[v.RuleName] = true
	}

	if !ruleNames["clarity-wording"] {
		t.Error("expected clarity-wording violation")
	}
	if !ruleNames["clarity-forbidden-words"] {
		t.Error("expected clarity-forbidden-words violation")
	}
}

func TestRuleEngine_ValidateAndGroup(t *testing.T) {
	engine := NewRuleEngine()
	engine.Register(&clarityForbiddenWordsRule{})
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种装置", Characterized: "包括A"},
	}
	errors, warnings, infos := engine.ValidateAndGroup(claims, DraftInput{})
	if len(errors)+len(warnings)+len(infos) != 0 {
		t.Errorf("expected 0 total violations, got errors=%d warnings=%d infos=%d",
			len(errors), len(warnings), len(infos))
	}
}
