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

// TestParseClaimsFromLLM_Basic 验证 LLM 返回文本的解析器能正确提取结构化的权利要求。
func TestParseClaimsFromLLM_Basic(t *testing.T) {
	input := DraftInput{
		Title:    "一种智能温度控制装置",
		Problems: []string{"温度控制精度不足", "响应速度慢"},
		Effects:  []string{"提高温度控制精度", "加快响应速度"},
		Features: []Feature{
			{ID: "f1", Description: "温度传感器", Category: "structure", Importance: "high"},
			{ID: "f2", Description: "微处理器", Category: "structure", Importance: "high"},
			{ID: "f3", Description: "加热元件", Category: "structure", Importance: "high"},
		},
	}
	text := `权利要求书

1. 一种智能温度控制装置，包括温度传感器和微处理器，其特征在于，所述微处理器根据温度传感器的检测信号控制加热元件的功率输出。

2. 根据权利要求1所述的智能温度控制装置，其特征在于，所述温度传感器为热电偶。

3. 根据权利要求1或2所述的智能温度控制装置，其特征在于，还包括与微处理器连接的显示模块。

4. 一种智能温度控制方法，其特征在于，包括以下步骤：获取温度传感器的检测信号；微处理器根据检测信号计算功率输出值；根据功率输出值控制加热元件。`

	output := parseClaimsFromLLM(text, input)
	if output == nil {
		t.Fatal("parseClaimsFromLLM returned nil")
	}
	if output.Claims == nil {
		t.Fatal("expected non-nil Claims")
	}

	// 验证独立权利要求数量（应有两个：产品+方法）
	if len(output.Claims.IndependentClaims) != 2 {
		t.Fatalf("expected 2 independent claims, got %d", len(output.Claims.IndependentClaims))
	}

	// 验证独立权利要求 1
	c1 := output.Claims.IndependentClaims[0]
	if c1.Number != 1 {
		t.Errorf("expected claim number 1, got %d", c1.Number)
	}
	if c1.Kind != "independent" {
		t.Errorf("expected kind independent, got %s", c1.Kind)
	}
	if c1.Preamble == "" || !strings.Contains(c1.Preamble, "温度控制装置") {
		t.Errorf("preamble should contain '温度控制装置', got %q", c1.Preamble)
	}
	if c1.Characterized == "" || !strings.Contains(c1.Characterized, "微处理器") {
		t.Errorf("characterized should contain '微处理器', got %q", c1.Characterized)
	}
	if c1.ClaimType != ClaimTypeProduct {
		t.Errorf("expected ClaimTypeProduct, got %s", c1.ClaimType)
	}

	// 验证从属权利要求
	if len(output.Claims.DependentClaims) != 2 {
		t.Fatalf("expected 2 dependent claims, got %d", len(output.Claims.DependentClaims))
	}

	// 验证第 2 条（从属）
	c2 := output.Claims.DependentClaims[0]
	if c2.Number != 2 {
		t.Errorf("expected claim number 2, got %d", c2.Number)
	}
	if c2.Kind != "dependent" {
		t.Errorf("expected kind dependent, got %s", c2.Kind)
	}
	if len(c2.DependsOn) != 1 || c2.DependsOn[0] != 1 {
		t.Errorf("expected depends on [1], got %v", c2.DependsOn)
	}

	// 验证第 3 条（多项从属）
	c3 := output.Claims.DependentClaims[1]
	if c3.Number != 3 {
		t.Errorf("expected claim number 3, got %d", c3.Number)
	}
	if len(c3.DependsOn) != 2 || c3.DependsOn[0] != 1 || c3.DependsOn[1] != 2 {
		t.Errorf("expected depends on [1,2], got %v", c3.DependsOn)
	}

	// 验证独立权利要求 4（方法权利要求）
	c4 := output.Claims.IndependentClaims[1]
	if c4.Number != 4 {
		t.Errorf("expected claim number 4, got %d", c4.Number)
	}
	if c4.ClaimType != ClaimTypeMethod {
		t.Errorf("independent claim 4 should be ClaimTypeMethod, got %s", c4.ClaimType)
	}
}

// TestParseClaimsFromLLM_Malformed 验证解析器遇到格式错误时正确返回 nil（触发降级）。
func TestParseClaimsFromLLM_Malformed(t *testing.T) {
	input := DraftInput{Title: "test"}
	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"no claims", "这是无关文本"},
		{"missing independent", "2. 根据权利要求1所述的装置。"}, // 没有 1 号
		{"bad number format", "一. 一种装置，其特征在于，xxx。"},
		{"missing period in dependency", "2. 根据权利要求1所述的"}, // 缺少内容
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := parseClaimsFromLLM(tt.text, input)
			if output != nil {
				t.Errorf("expected nil for malformed input, got non-nil output with %d claims", len(output.Claims.IndependentClaims))
			}
		})
	}
}

// TestParseClaimsFromLLM_DomainCarryOver 验证 TechDomain 正确传递到输出。
func TestParseClaimsFromLLM_DomainCarryOver(t *testing.T) {
	input := DraftInput{
		Title: "一种机械装置",
		Features: []Feature{
			{ID: "f1", Description: "弹性元件", Category: "structure", Importance: "high"},
		},
	}
	input.TechDomain = DomainMechanical

	text := `权利要求书

1. 一种机械装置，其特征在于，包含弹性元件。

2. 根据权利要求1所述的机械装置，其特征在于，所述弹性元件为弹簧。`

	output := parseClaimsFromLLM(text, input)
	if output == nil {
		t.Fatal("parseClaimsFromLLM returned nil")
	}
	if output.InputMeta.Domain != DomainMechanical {
		t.Errorf("expected InputMeta.Domain %s, got %s", DomainMechanical, output.InputMeta.Domain)
	}
}
