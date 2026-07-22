package claimdrafting

import (
	"strings"
	"testing"
)

func TestClaimBuilder_Build(t *testing.T) {
	builder := NewClaimBuilder(DomainGeneral, "")

	input := DraftInput{
		Title:    "一种智能加热杯",
		Problems: []string{"现有技术中保温杯加热不均匀"},
		Features: []Feature{
			{ID: "f1", Description: "杯体", Category: "structure", Importance: "high", PriorStatus: "known"},
			{ID: "f2", Description: "加热单元", Category: "structure", Importance: "high", PriorStatus: "unknown"},
			{ID: "f3", Description: "温度传感器", Category: "structure", Importance: "high", PriorStatus: "unknown"},
			{ID: "f4", Description: "控制电路", Category: "structure", Importance: "known", PriorStatus: "medium"},
			{ID: "f5", Description: "保温层", Category: "structure", Importance: "medium", PriorStatus: "unknown"},
		},
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "加热不均匀", FeatureIDs: []string{"f1", "f2", "f3"}},
		},
	}

	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if output.Claims == nil {
		t.Fatal("Build returned nil Claims")
	}

	if len(output.Claims.IndependentClaims) != 1 {
		t.Fatalf("expected 1 independent claim, got %d", len(output.Claims.IndependentClaims))
	}

	indClaim := output.Claims.IndependentClaims[0]
	if indClaim.Number != 1 {
		t.Errorf("expected claim number 1, got %d", indClaim.Number)
	}
	if indClaim.Kind != "independent" {
		t.Errorf("expected independent kind, got %s", indClaim.Kind)
	}
	if indClaim.Preamble == "" {
		t.Error("expected non-empty preamble")
	}
	if indClaim.Characterized == "" {
		t.Error("expected non-empty characterized portion")
	}

	// 验证输出格式
	rendered := indClaim.String()
	if !strings.Contains(rendered, "其特征在于") {
		t.Error("rendered claim should contain '其特征在于'")
	}
	if !strings.HasSuffix(rendered, "。") {
		t.Error("rendered claim should end with '。'")
	}
}

func TestClaimBuilder_FromFeatures(t *testing.T) {
	builder := NewClaimBuilder(DomainGeneral, "")

	input := DraftInput{
		Title: "测试装置",
		Features: []Feature{
			{ID: "f1", Description: "底座", Importance: "high", PriorStatus: "known"},
			{ID: "f2", Description: "驱动电机", Importance: "high", PriorStatus: "unknown"},
			{ID: "f3", Description: "传动机构", Importance: "high", PriorStatus: "unknown"},
			{ID: "f4", Description: "控制面板", Importance: "medium", PriorStatus: "unknown"},
		},
		Problems: []string{"手动操作效率低"},
		Effects:  []string{"提高自动化程度"},
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "手动操作效率低", FeatureIDs: []string{"f1", "f2", "f3"}},
		},
	}

	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证从属权利要求
	if len(output.Claims.DependentClaims) == 0 {
		t.Error("expected at least one dependent claim")
	}

	// 验证每个从属权利要求的引用关系
	for _, dep := range output.Claims.DependentClaims {
		if len(dep.DependsOn) == 0 {
			t.Errorf("dependent claim %d has no dependencies", dep.Number)
		}
		for _, d := range dep.DependsOn {
			if d >= dep.Number {
				t.Errorf("dependent claim %d references forward claim %d", dep.Number, d)
			}
		}
	}

	// 验证编号连续性
	allClaims := output.Claims.Claims()
	for i, c := range allClaims {
		expectedNum := i + 1
		if c.Number != expectedNum {
			t.Errorf("expected claim %d to have number %d, got %d", i, expectedNum, c.Number)
		}
	}
}

func TestClaimBuilder_WithDomainSpecificRules(t *testing.T) {
	// 机械领域测试
	builder := NewClaimBuilder(DomainMechanical, "")
	input := DraftInput{
		Title:    "一种机械夹持装置",
		Problems: []string{"夹持力不稳定"},
		Features: []Feature{
			{ID: "f1", Description: "夹持臂", Importance: "high", PriorStatus: "known"},
			{ID: "f2", Description: "驱动气缸", Importance: "high", PriorStatus: "unknown"},
		},
	}

	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证领域分类
	if output.InputMeta.Domain != DomainMechanical {
		t.Errorf("expected DomainMechanical, got %s", output.InputMeta.Domain)
	}

	// 化学领域测试
	builder2 := NewClaimBuilder(DomainChemical, "")
	input2 := DraftInput{
		Title:    "一种催化组合物",
		Problems: []string{"反应效率低"},
		Features: []Feature{
			{ID: "f1", Description: "催化剂A", Category: "material", Importance: "high", PriorStatus: "unknown"},
		},
		TechDomain: DomainChemical,
	}

	output2, err2 := builder2.Build(input2)
	if err2 != nil {
		t.Fatalf("Build failed: %v", err2)
	}

	if output2.InputMeta.Domain != DomainChemical {
		t.Errorf("expected DomainChemical, got %s", output2.InputMeta.Domain)
	}
}

func TestClassifyDomain_Keywords(t *testing.T) {
	tests := []struct {
		name     string
		input    DraftInput
		expected TechDomain
	}{
		{
			name:     "mechanical keywords",
			input:    DraftInput{Title: "一种齿轮传动机构", Features: []Feature{{Category: "structure"}}},
			expected: DomainMechanical,
		},
		{
			name:     "electrical keywords",
			input:    DraftInput{Title: "一种电压调节电路", Features: []Feature{{Category: "parameter"}}},
			expected: DomainElectrical,
		},
		{
			name:     "chemical keywords",
			input:    DraftInput{Title: "一种高分子组合物", Features: []Feature{{Category: "material"}}},
			expected: DomainChemical,
		},
		{
			name:     "software keywords",
			input:    DraftInput{Title: "一种图像处理方法", Features: []Feature{{Category: "method"}}},
			expected: DomainSoftware,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyDomain(tt.input)
			if result != tt.expected {
				t.Errorf("classifyDomain(%s) = %s, want %s", tt.name, result, tt.expected)
			}
		})
	}
}

func TestClassifyFeatures(t *testing.T) {
	features := []Feature{
		{ID: "f1", Description: "A", Importance: "high"},
		{ID: "f2", Description: "B", Importance: "high"},
		{ID: "f3", Description: "C", Importance: "medium"},
		{ID: "f4", Description: "D", Importance: "low"},
	}
	triples := []PFETriple{
		{ID: "t1", FeatureIDs: []string{"f1", "f2"}},
	}

	essential, optional := classifyFeatures(features, triples)

	if len(essential) != 2 {
		t.Errorf("expected 2 essential features, got %d", len(essential))
	}
	if len(optional) != 2 {
		t.Errorf("expected 2 optional features, got %d", len(optional))
	}
}

func TestDetermineClaimType(t *testing.T) {
	t.Run("product by structure", func(t *testing.T) {
		features := []Feature{{Category: "structure"}, {Category: "parameter"}}
		result := determineClaimTypeByFeatures(features)
		if result != ClaimTypeProduct {
			t.Errorf("expected product type, got %s", result)
		}
	})

	t.Run("method when method category present", func(t *testing.T) {
		features := []Feature{{Category: "method"}}
		result := determineClaimTypeByFeatures(features)
		if result != ClaimTypeMethod {
			t.Errorf("expected method type, got %s", result)
		}
	})
}

func TestClaimSet_Claims(t *testing.T) {
	cs := &ClaimSet{
		IndependentClaims: []Claim{
			{Number: 1, Kind: "independent", ClaimType: ClaimTypeProduct},
		},
		DependentClaims: []Claim{
			{Number: 2, Kind: "dependent", DependsOn: []int{1}, ClaimType: ClaimTypeProduct},
			{Number: 3, Kind: "dependent", DependsOn: []int{1, 2}, ClaimType: ClaimTypeProduct},
		},
	}

	all := cs.Claims()
	if len(all) != 3 {
		t.Errorf("expected 3 claims, got %d", len(all))
	}
	if all[0].Number != 1 || all[1].Number != 2 || all[2].Number != 3 {
		t.Error("Claims() returned in wrong order")
	}
}

func TestClaimString_Independent(t *testing.T) {
	c := Claim{
		Number:        1,
		ClaimType:     ClaimTypeProduct,
		Kind:          "independent",
		Preamble:      "一种智能装置",
		Characterized: "包括处理器和存储器",
	}
	s := c.String()
	if !strings.Contains(s, "一种智能装置") {
		t.Error("rendered claim missing preamble text")
	}
	if !strings.Contains(s, "其特征在于") {
		t.Error("rendered claim missing '其特征在于'")
	}
	if !strings.Contains(s, "包括处理器和存储器") {
		t.Error("rendered claim missing characterized text")
	}
	if !strings.HasSuffix(s, "。") {
		t.Error("rendered claim should end with '。'")
	}
}

func TestClaimString_Dependent(t *testing.T) {
	c := Claim{
		Number:     2,
		ClaimType:  ClaimTypeProduct,
		Kind:       "dependent",
		DependsOn:  []int{1},
		Limitation: "所述处理器为ARM架构处理器",
	}
	s := c.String()
	if !strings.Contains(s, "根据权利要求") {
		t.Error("rendered dependent claim missing reference prefix")
	}
	if !strings.Contains(s, "所述的") {
		t.Error("rendered dependent claim missing '所述的'")
	}
	if !strings.Contains(s, "所述处理器为ARM架构处理器") {
		t.Error("rendered dependent claim missing limitation text")
	}
}

func TestClaimString_MultipleDependent(t *testing.T) {
	c := Claim{
		Number:    3,
		ClaimType: ClaimTypeProduct,
		Kind:      "dependent",
		DependsOn: []int{1, 2},

		Limitation: "温度范围在50-100度之间",
	}
	s := c.String()
	if !strings.Contains(s, "或") {
		t.Error("multiple dependent claim should use '或' for alternative reference")
	}
}

func TestEmptyClaimBuilder(t *testing.T) {
	builder := NewClaimBuilder(DomainGeneral, "")
	input := DraftInput{
		Title: "空测试",
	}
	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build with minimal input should not error: %v", err)
	}
	if output.Claims == nil {
		t.Fatal("expected Claims to be non-nil even with empty input")
	}
}

func TestBuilderWarnings(t *testing.T) {
	builder := NewClaimBuilder(DomainGeneral, "")
	input := DraftInput{
		Title: "加热装置",
		Features: []Feature{
			{ID: "f1", Description: "加热器在高温下工作", Importance: "high"},
		},
		PriorArt: "现有加热装置",
	}

	output, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 应该检测到"高温"这个不确定用语
	foundUncertainWarning := false
	for _, w := range output.Warnings {
		if strings.Contains(w, "clarity-wording") {
			foundUncertainWarning = true
			break
		}
	}
	if !foundUncertainWarning {
		t.Error("expected warning for uncertain term '高温', got warnings:", output.Warnings)
	}
}

func TestRuleEngine_EmptyRules(t *testing.T) {
	engine := NewRuleEngine()
	claims := []Claim{{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent"}}
	violations := engine.Validate(claims, DraftInput{})
	if len(violations) != 0 {
		t.Errorf("expected 0 violations from empty engine, got %d", len(violations))
	}
}
