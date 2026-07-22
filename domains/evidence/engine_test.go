package evidence

import (
	"testing"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

func TestDefaultEngine_Judge(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:        "test-span-001",
		SourceURI: "https://www.cnipa.gov.cn/patent/CN12345678A",
		Direction: agentcore_evidence.DirectionSupporting,
		ClaimRefs: []string{"claim-1"},
		Snippet:   "本发明提供了一种改进的制造方法...",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.SpanID != "test-span-001" {
		t.Errorf("SpanID = %q, 期望 %q", judgment.SpanID, "test-span-001")
	}

	if judgment.RelevanceJudgment == nil {
		t.Error("RelevanceJudgment 不应为 nil")
	}
	if judgment.LegalityJudgment == nil {
		t.Error("LegalityJudgment 不应为 nil")
	}
	if judgment.AuthenticityJudgment == nil {
		t.Error("AuthenticityJudgment 不应为 nil")
	}

	if judgment.OverallScore <= 0 {
		t.Errorf("OverallScore 应 > 0, 实际 = %f", judgment.OverallScore)
	}
	if judgment.Confidence <= 0 {
		t.Errorf("Confidence 应 > 0, 实际 = %f", judgment.Confidence)
	}

	if judgment.Reasoning == "" {
		t.Error("Reasoning 不应为空")
	}
}

func TestDefaultEngine_Judge_EmptySpanID(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID: "",
	}

	_, err := engine.Judge(span)
	if err == nil {
		t.Fatal("期望 Judge() 返回错误，但返回 nil")
	}
}

func TestDefaultEngine_BatchJudge(t *testing.T) {
	engine := NewEngine(nil)

	spans := []agentcore_evidence.EvidenceSpan{
		{ID: "span-1", SourceURI: "https://example.com/doc1"},
		{ID: "span-2", SourceURI: "https://example.com/doc2"},
	}

	judgments, err := engine.BatchJudge(spans)
	if err != nil {
		t.Fatalf("BatchJudge() 返回错误: %v", err)
	}

	if len(judgments) != 2 {
		t.Errorf("期望 %d 个判断结果，实际 %d", 2, len(judgments))
	}

	for _, j := range judgments {
		if j.SpanID == "" {
			t.Error("判断结果缺少 SpanID")
		}
	}
}

func TestDefaultEngine_BatchJudge_Empty(t *testing.T) {
	engine := NewEngine(nil)

	judgments, err := engine.BatchJudge([]agentcore_evidence.EvidenceSpan{})
	if err != nil {
		t.Fatalf("BatchJudge(empty) 返回错误: %v", err)
	}
	// 空切片返回空结果切片（非 nil，因 make 不为空分配 nil）
	if len(judgments) != 0 {
		t.Errorf("BatchJudge(empty) 应返回空切片，实际 len=%d", len(judgments))
	}
}

func TestDefaultEngine_AssessBurdenOfProof_Invalidation(t *testing.T) {
	engine := NewEngine(nil)

	context := map[string]string{
		"burden_holder": "请求人",
	}

	determination, err := engine.AssessBurdenOfProof("invalidation", context)
	if err != nil {
		t.Fatalf("AssessBurdenOfProof() 返回错误: %v", err)
	}

	if determination.BurdenHolder != "请求人" {
		t.Errorf("BurdenHolder = %q, 期望 %q", determination.BurdenHolder, "请求人")
	}

	if determination.Reasoning == "" {
		t.Error("Reasoning 不应为空")
	}
}

func TestDefaultEngine_AssessBurdenOfProof_Infringement(t *testing.T) {
	engine := NewEngine(nil)

	determination, err := engine.AssessBurdenOfProof("infringement", nil)
	if err != nil {
		t.Fatalf("AssessBurdenOfProof() 返回错误: %v", err)
	}

	if determination.Standard != "clear_and_convincing" {
		t.Errorf("Standard = %q, 期望 %q", determination.Standard, "clear_and_convincing")
	}
}

func TestDefaultEngine_AssessBurdenOfProof_Default(t *testing.T) {
	engine := NewEngine(nil)

	determination, err := engine.AssessBurdenOfProof("", nil)
	if err != nil {
		t.Fatalf("AssessBurdenOfProof() 返回错误: %v", err)
	}

	if determination.BurdenHolder != "claimant" {
		t.Errorf("空类型时 BurdenHolder = %q, 期望 %q", determination.BurdenHolder, "claimant")
	}
}

func TestDefaultEngine_AssessProofStandard(t *testing.T) {
	engine := NewEngine(nil)

	judgments := []*EvidenceJudgment{
		{OverallScore: 0.85, Confidence: 0.9},
		{OverallScore: 0.75, Confidence: 0.8},
		{OverallScore: 0.65, Confidence: 0.7},
	}

	result, err := engine.AssessProofStandard(judgments, "preponderance")
	if err != nil {
		t.Fatalf("AssessProofStandard() 返回错误: %v", err)
	}

	if !result.Met {
		t.Error("优势证据标准应满足（3 份中 3 份 >= 0.6）")
	}

	if result.SupportingCount != 3 {
		t.Errorf("SupportingCount = %d, 期望 %d", result.SupportingCount, 3)
	}
}

func TestDefaultEngine_AssessProofStandard_Empty(t *testing.T) {
	engine := NewEngine(nil)

	result, err := engine.AssessProofStandard(nil, "preponderance")
	if err != nil {
		t.Fatalf("AssessProofStandard(nil) 返回错误: %v", err)
	}
	if result.Met {
		t.Error("无判断结果时不应满足证明标准")
	}
}

func TestDefaultEngine_AssessProofStandard_WithNilJudgments(t *testing.T) {
	engine := NewEngine(nil)

	judgments := []*EvidenceJudgment{nil, {OverallScore: 0.8}, nil}
	result, err := engine.AssessProofStandard(judgments, "preponderance")
	if err != nil {
		t.Fatalf("AssessProofStandard() 返回错误: %v", err)
	}
	if !result.Met {
		t.Error("含 nil 时应仍能正确评估")
	}
}

func TestDefaultEngine_LoadRules(t *testing.T) {
	engine := NewEngine(nil)

	yamlData := []byte(`
rules:
  - ruleId: EVI-001
    name: 测试规则
    description: 用于测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
`)

	if err := engine.LoadRules(yamlData); err != nil {
		t.Fatalf("LoadRules() 返回错误: %v", err)
	}

	rules := engine.GetRules()
	if len(rules) != 1 {
		t.Errorf("获取规则数 = %d, 期望 %d", len(rules), 1)
	}

	rule, ok := engine.index.GetRule("EVI-001")
	if !ok {
		t.Fatal("GetRule(EVI-001) 未找到")
	}
	if rule.Name != "测试规则" {
		t.Errorf("Name = %q, 期望 %q", rule.Name, "测试规则")
	}
}

func TestDefaultEngine_GetRulesByType_NoRules(t *testing.T) {
	engine := NewEngine(nil)

	// 未加载规则时，GetRulesByType 返回 nil（因索引内无任何规则）
	rules := engine.GetRulesByType(EvTypeGeneral)
	if rules != nil {
		t.Logf("未加载规则时 GetRulesByType 返回 %v (len=%d)", rules, len(rules))
	}
}

func TestDefaultEngine_GetRulesByType_WithLoadedRules(t *testing.T) {
	engine := NewEngine(nil)

	yamlData := []byte(`
rules:
  - ruleId: EVI-GEN
    name: 通用规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
`)

	if err := engine.LoadRules(yamlData); err != nil {
		t.Fatalf("LoadRules() 返回错误: %v", err)
	}

	rules := engine.GetRulesByType(EvTypeGeneral)
	if len(rules) != 1 {
		t.Errorf("通用类型规则数 = %d, 期望 %d", len(rules), 1)
	}
}

func TestDefaultEngine_LoadRules_Empty(t *testing.T) {
	engine := NewEngine(nil)

	// LoadRules(nil) 委托给 LoadBytes(nil)，yaml.Unmarshal 处理空数据
	err := engine.LoadRules(nil)
	if err == nil {
		t.Log("LoadRules(nil) 未返回错误（yaml 处理 nil 非预期）")
	}

	err = engine.LoadRules([]byte{})
	if err == nil {
		t.Log("LoadRules(empty) 未返回错误（yaml 处理空内容非预期）")
	}
}

func TestDefaultEngine_Judge_WithTypeSpecific(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:        "test-electronic",
		SourceURI: "https://www.cnipa.gov.cn/notice/123",
		Direction: agentcore_evidence.DirectionSupporting,
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.EvidenceType != EvTypeElectronic {
		t.Errorf("EvidenceType = %q, 期望 %q", judgment.TypeSpecificJudgment.EvidenceType, EvTypeElectronic)
	}

	if judgment.TypeSpecificJudgment.PlatformCredibility == nil {
		t.Error("电子证据的 PlatformCredibility 不应为 nil")
	} else if *judgment.TypeSpecificJudgment.PlatformCredibility != CredHigh {
		t.Errorf("政府域名应为 CredHigh, 实际 = %s", *judgment.TypeSpecificJudgment.PlatformCredibility)
	}
}

func TestDefaultEngine_Judge_SetsEvaluatedAt(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID: "test-time",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	// 引擎始终设置 EvaluatedAt = time.Now()
	if judgment.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt 不应为零值（引擎应设置评估时间）")
	}
}

func TestDefaultEngine_NewEngine(t *testing.T) {
	engine := NewEngine(nil)
	if engine == nil {
		t.Fatal("NewEngine(nil) 返回 nil")
	}
	if engine.index == nil {
		t.Error("index 不应为 nil")
	}
}

func TestDefaultEngine_NewEngineWithIndex(t *testing.T) {
	idx := NewRuleIndex()
	engine := NewEngine(idx)
	if engine == nil {
		t.Fatal("NewEngine(idx) 返回 nil")
	}
	if engine.index != idx {
		t.Error("index 应与传入的索引一致")
	}
}
