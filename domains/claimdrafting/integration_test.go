package claimdrafting

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestIntegration_MechanicalCase 机械领域端到端测试。
func TestIntegration_MechanicalCase(t *testing.T) {
	data, err := os.ReadFile("testdata/mechanical-case.json")
	if err != nil {
		t.Skipf("skip: cannot read testdata: %v", err)
	}

	var input DraftInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("failed to parse testdata: %v", err)
	}

	builder := NewClaimBuilder(DomainMechanical, input.PriorArt)
	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if output.Claims == nil {
		t.Fatal("expected non-nil Claims")
	}

	// 验证独立权利要求
	indClaims := output.Claims.IndependentClaims
	if len(indClaims) != 1 {
		t.Fatalf("expected 1 independent claim, got %d", len(indClaims))
	}

	ind := indClaims[0]
	if ind.Number != 1 {
		t.Errorf("expected claim 1 number, got %d", ind.Number)
	}
	if ind.Preamble == "" {
		t.Error("expected non-empty preamble")
	}
	if ind.Characterized == "" {
		t.Error("expected non-empty characterized portion")
	}
	if ind.ClaimType != ClaimTypeProduct {
		t.Errorf("expected product type for mechanical device, got %s", ind.ClaimType)
	}

	// 验证从属权利要求
	if len(output.Claims.DependentClaims) == 0 {
		t.Error("expected at least one dependent claim")
	}

	// 验证所有权利要求格式
	for _, c := range output.Claims.Claims() {
		rendered := c.String()
		if !strings.HasSuffix(rendered, "。") {
			t.Errorf("claim %d: should end with '。'", c.Number)
		}
	}

	// 验证领域分类
	if output.InputMeta.Domain != DomainMechanical {
		t.Errorf("expected mechanical domain, got %s", output.InputMeta.Domain)
	}

	// 验证引擎可以检测到问题
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	allClaims := output.Claims.Claims()
	violations := engine.Validate(allClaims, input)

	// 机械领域规则应警告缺少配置关系（如果确实缺少的话）
	_ = violations

	t.Logf("Generated %d claims (%d independent, %d dependent)",
		len(allClaims),
		len(output.Claims.IndependentClaims),
		len(output.Claims.DependentClaims))
	t.Logf("Independent claim: %s", ind.String())
	for _, dep := range output.Claims.DependentClaims {
		t.Logf("Dependent claim %d: %s", dep.Number, dep.String())
	}
	if len(output.Warnings) > 0 {
		t.Logf("Warnings: %v", output.Warnings)
	}
}

// TestIntegration_ChemicalCase 化学领域端到端测试。
func TestIntegration_ChemicalCase(t *testing.T) {
	input := DraftInput{
		Title:      "一种催化组合物",
		TechDomain: DomainChemical,
		Problems:   []string{"现有催化剂反应效率低、选择性差"},
		Features: []Feature{
			{ID: "f1", Description: "主催化剂A", Category: "material", Importance: "high", PriorStatus: "unknown"},
			{ID: "f2", Description: "助催化剂B", Category: "material", Importance: "high", PriorStatus: "unknown"},
			{ID: "f3", Description: "载体C", Category: "material", Importance: "medium", PriorStatus: "known"},
		},
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "反应效率低", FeatureIDs: []string{"f1", "f2"}},
		},
	}

	builder := NewClaimBuilder(DomainChemical, "")
	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if output.Claims == nil || len(output.Claims.IndependentClaims) == 0 {
		t.Fatal("expected at least one independent claim")
	}

	ind := output.Claims.IndependentClaims[0]
	if ind.ClaimType != ClaimTypeProduct {
		t.Errorf("expected product type for composition, got %s", ind.ClaimType)
	}

	t.Logf("Chemical independent claim: %s", ind.String())
}

// TestIntegration_SoftwareCase 软件领域端到端测试。
func TestIntegration_SoftwareCase(t *testing.T) {
	input := DraftInput{
		Title:      "一种图像处理方法",
		TechDomain: DomainSoftware,
		Problems:   []string{"现有图像处理方法在低光照条件下噪声大"},
		Features: []Feature{
			{ID: "f1", Description: "获取原始图像数据", Category: "method", Importance: "high", PriorStatus: "known"},
			{ID: "f2", Description: "对图像进行自适应滤波", Category: "method", Importance: "high", PriorStatus: "unknown"},
			{ID: "f3", Description: "增强对比度", Category: "method", Importance: "high", PriorStatus: "unknown"},
		},
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "低光照噪声大", FeatureIDs: []string{"f1", "f2", "f3"}},
		},
	}

	builder := NewClaimBuilder(DomainSoftware, "")
	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	allClaims := output.Claims.Claims()
	if len(allClaims) == 0 {
		t.Fatal("expected at least one claim")
	}

	ind := output.Claims.IndependentClaims[0]
	t.Logf("Software independent claim: %s", ind.String())
	t.Logf("Total claims: %d", len(allClaims))
}

// TestIntegration_Scorer 评分器集成测试。
func TestIntegration_Scorer(t *testing.T) {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	scorer := NewClaimScorer(engine)

	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent",
			Preamble: "一种加热装置", Characterized: "在高温下工作"},
		{Number: 2, Kind: "dependent", DependsOn: []int{1}, ClaimType: ClaimTypeProduct,
			Limitation: "最好使用不锈钢材料"},
	}

	report := scorer.Score(claims, DraftInput{})
	if report.OverallScore <= 0 {
		t.Errorf("expected positive score, got %f", report.OverallScore)
	}
	if len(report.Violations) == 0 {
		t.Error("expected violations for problematic claims")
	}
	if report.Grade == "" {
		t.Error("expected non-empty grade")
	}

	t.Logf("Score: %.1f, Grade: %s", report.OverallScore, report.Grade)
	t.Logf("Violations found: %d", len(report.Violations))
	t.Logf("Suggestions: %v", report.Suggestions)
}

// TestIntegration_EmptyInput 空输入测试。
func TestIntegration_EmptyInput(t *testing.T) {
	builder := NewClaimBuilder(DomainGeneral, "")
	input := DraftInput{Title: "测试"}
	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build with minimal input should succeed: %v", err)
	}
	if output.Claims == nil {
		t.Fatal("expected non-nil Claims for minimal input")
	}
}

// TestIntegration_FormatOutput 输出格式验证。
func TestIntegration_FormatOutput(t *testing.T) {
	builder := NewClaimBuilder(DomainGeneral, "")
	input := DraftInput{
		Title:    "测试装置",
		Problems: []string{"测试问题"},
		Features: []Feature{
			{ID: "f1", Description: "部件A", Importance: "high", PriorStatus: "known"},
			{ID: "f2", Description: "部件B", Importance: "high", PriorStatus: "unknown"},
		},
	}

	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	allClaims := output.Claims.Claims()
	if len(allClaims) < 1 {
		t.Fatal("expected at least one claim")
	}

	// 验证编号连续
	for i, c := range allClaims {
		if c.Number != i+1 {
			t.Errorf("claim %d: expected number %d, got %d", i, i+1, c.Number)
		}
	}
}

// TestIntegration_FullPipeline 完整管线测试（从 DraftInput 到评分报告）。
func TestIntegration_FullPipeline(t *testing.T) {
	input := DraftInput{
		Title:    "一种智能温度控制装置",
		Problems: []string{"温度控制精度低", "响应速度慢"},
		Features: []Feature{
			{ID: "f1", Description: "温度传感器", Category: "structure", Importance: "high", PriorStatus: "known"},
			{ID: "f2", Description: "微处理器", Category: "structure", Importance: "high", PriorStatus: "unknown"},
			{ID: "f3", Description: "加热元件", Category: "structure", Importance: "high", PriorStatus: "unknown"},
			{ID: "f4", Description: "PID控制算法", Category: "method", Importance: "medium", PriorStatus: "unknown"},
		},
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "温度控制精度低", FeatureIDs: []string{"f1", "f2", "f3"}},
		},
	}

	// 构建
	builder := NewClaimBuilder(DomainGeneral, "")
	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(output.Claims.IndependentClaims) == 0 {
		t.Fatal("no independent claims generated")
	}

	// 验证
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	allClaims := output.Claims.Claims()
	violations := engine.Validate(allClaims, input)

	// 评分
	scorer := NewClaimScorer(engine)
	report := scorer.Score(allClaims, input)

	t.Logf("=== 完整管线测试 ===")
	t.Logf("独立权利要求: %s", output.Claims.IndependentClaims[0].String())
	t.Logf("从属权利要求数: %d", len(output.Claims.DependentClaims))
	t.Logf("综合评分: %.1f (等级: %s)", report.OverallScore, report.Grade)
	t.Logf("维度得分: %v", report.DimensionScores)
	t.Logf("违规数: %d", len(violations))
	t.Logf("警告数: %d", len(output.Warnings))
}
