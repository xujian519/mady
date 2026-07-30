package guardrails

import (
	"strings"
	"testing"

	iface "github.com/xujian519/mady/agentcore/iface"
)

// --- Pipeline Tests ---

func TestRulePipeline_AllPass(t *testing.T) {
	p := NewRulePipeline(NewConfidenceCheckRule())
	content, results := p.Apply("你好，今天天气不错。", nil)

	for _, r := range results {
		if !r.Passed {
			t.Errorf("expected all rules to pass, but %q failed: %s", r.RuleName, r.Message)
		}
	}
	if content != "你好，今天天气不错。" {
		t.Errorf("content should be unchanged, got: %s", content)
	}
}

func TestRulePipeline_InjectDisclaimer(t *testing.T) {
	rule := NewConfidenceCheckRule()
	p := NewRulePipeline(rule)

	// Content with conclusive phrase but no confidence marker
	content := "综上所述，本发明具有突出的实质性特点。"
	modified, results := p.Apply(content, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("expected rule to fail (missing confidence annotation)")
	}
	if results[0].Action != ActionInject {
		t.Errorf("expected ActionInject, got %s", results[0].Action)
	}
	if !strings.Contains(modified, "AI 生成") {
		t.Errorf("expected disclaimer injection, got: %s", modified)
	}
}

func TestRulePipeline_BlockPriority(t *testing.T) {
	blockRule := NewKeywordRule([]string{"恶意代码"}, nil, nil, "")
	injectRule := NewConfidenceCheckRule()
	p := NewRulePipeline(blockRule, injectRule)

	content := "恶意代码：综上所述，本发明。"
	modified, results := p.Apply(content, nil)

	if !HasBlocking(results) {
		t.Error("expected blocking result")
	}
	if strings.Contains(modified, "本发明") {
		t.Errorf("content should be replaced by block message, got: %s", modified)
	}
	if strings.Contains(modified, "AI 生成") {
		t.Error("inject disclaimer should not appear when blocked")
	}
}

func TestRulePipeline_FailedRules(t *testing.T) {
	p := NewRulePipeline(
		NewKeywordRule([]string{"恶意代码"}, nil, nil, ""),
		NewConfidenceCheckRule(),
	)

	content := "综上所述，本发明。"
	_, results := p.Apply(content, nil)

	failed := FailedRules(results)
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed rule, got %d: %v", len(failed), failed)
	}
	if failed[0] != "confidence-check" {
		t.Errorf("expected 'confidence-check' to fail, got %q", failed[0])
	}
}

// --- FactCheck Tests ---

func TestFactCheck_BogusArticle(t *testing.T) {
	rule := NewFactCheckRule()
	result := rule.Check("根据专利法第99条规定，申请人应当...", nil)

	if result.Passed {
		t.Error("expected rule to fail for non-existent article")
	}
	if !strings.Contains(result.Message, "99") {
		t.Errorf("expected message to mention article 99, got: %s", result.Message)
	}
}

func TestFactCheck_ValidArticle(t *testing.T) {
	rule := NewFactCheckRule()
	result := rule.Check("根据专利法第26条第3款规定...", nil)

	if !result.Passed {
		t.Errorf("expected valid article to pass, got: %s", result.Message)
	}
}

func TestFactCheck_MultipleSuspects(t *testing.T) {
	rule := NewFactCheckRule()
	rule.MaxArticles = map[string]int{"专利法": 82}
	rule.MaxFactualErrors = 1
	rule.BlockOnViolation = true
	result := rule.Check("专利法第99条，专利法第100条", nil)

	if result.Passed {
		t.Error("expected rule to fail")
	}
	if result.Action != ActionBlock {
		t.Errorf("expected ActionBlock, got %s", result.Action)
	}
}

func TestFactCheck_AbsoluteClaims(t *testing.T) {
	rule := NewFactCheckRule()
	// Absolute claim without supporting citation
	result := rule.Check("毫无疑问，这个技术方案具有创造性。", nil)

	if result.Passed {
		t.Error("expected rule to flag absolute claim")
	}
}

// --- Consistency Tests ---

func TestConsistency_InternalContradiction(t *testing.T) {
	rule := NewConsistencyRule()
	content := "该技术方案具有新颖性。另一方面，该技术方案不具有新颖性。"

	result := rule.Check(content, nil)
	if result.Passed {
		t.Error("expected contradiction to be detected")
	}
	if !strings.Contains(result.Message, "新颖性") {
		t.Errorf("expected message to mention the contradiction, got: %s", result.Message)
	}
}

func TestConsistency_NoContradiction(t *testing.T) {
	rule := NewConsistencyRule()
	result := rule.Check("该技术方案具有新颖性和创造性。", nil)

	if !result.Passed {
		t.Errorf("expected no contradiction, got: %s", result.Message)
	}
}

func TestConsistency_OutsideWindow(t *testing.T) {
	rule := NewConsistencyRule()
	rule.MaxWindow = 100 // small window

	// Create content using a pair where the affirmative is NOT a substring
	// of the negative. Use {"满足", "不满足"} which is unambiguous.
	longSep := strings.Repeat("x", 300)
	content := "该方案满足要求" + longSep + "该方案不满足另一要求"

	result := rule.Check(content, nil)
	if !result.Passed {
		t.Errorf("expected far-apart phrases to pass (window=100, distance>300), got: %s", result.Message)
	}
}

// --- FormatCheck Tests ---

func TestFormatCheck_PatentAllSections(t *testing.T) {
	rule := PatentFormatCheck()
	content := "技术领域：xxx\n背景技术：xxx\n发明内容：xxx\n具体实施方式：xxx"

	result := rule.Check(content, map[string]any{"domain": "patent"})
	if !result.Passed {
		t.Errorf("expected all sections present, got: %s", result.Message)
	}
}

func TestFormatCheck_PatentMissingSections(t *testing.T) {
	rule := PatentFormatCheck()
	content := "技术领域：xxx\n背景技术：xxx" // Missing 发明内容, 具体实施方式

	result := rule.Check(content, map[string]any{"domain": "patent"})
	if result.Passed {
		t.Error("expected missing sections to be flagged")
	}
	if !strings.Contains(result.Message, "发明内容") {
		t.Errorf("expected missing 发明内容, got: %s", result.Message)
	}
}

func TestFormatCheck_WrongDomain(t *testing.T) {
	rule := PatentFormatCheck()
	content := "Just some legal content." // no patent sections

	result := rule.Check(content, map[string]any{"domain": "legal"})
	if !result.Passed {
		t.Errorf("patent rule should pass on legal domain, got: %s", result.Message)
	}
}

func TestFormatCheck_MinSectionCount(t *testing.T) {
	rule := PatentFormatCheck()
	rule.MinSectionCount = 2        // only need 2 of 4
	content := "技术领域：xxx\n背景技术：xxx" // 2 sections present

	result := rule.Check(content, map[string]any{"domain": "patent"})
	if !result.Passed {
		t.Errorf("expected 2/4 sections to pass with MinSectionCount=2, got: %s", result.Message)
	}
}

// --- ConfidenceCheck Tests ---

func TestConfidenceCheck_WithMarker(t *testing.T) {
	rule := NewConfidenceCheckRule()
	// Conclusive phrase WITH confidence marker nearby
	content := "综上所述，本发明具有创造性。注意：以上分析为AI生成，置信度中等。"

	result := rule.Check(content, nil)
	if !result.Passed {
		t.Errorf("expected confidence marker to satisfy check, got: %s", result.Message)
	}
}

func TestConfidenceCheck_WithoutMarker(t *testing.T) {
	rule := NewConfidenceCheckRule()
	content := "综上所述，本发明具有创造性。"

	result := rule.Check(content, nil)
	if result.Passed {
		t.Error("expected missing confidence marker to be flagged")
	}
	if result.Action != ActionInject {
		t.Errorf("expected ActionInject, got %s", result.Action)
	}
}

func TestConfidenceCheck_NoConclusivePhrase(t *testing.T) {
	rule := NewConfidenceCheckRule()
	content := "本技术方案包括A、B、C三个特征。" // No conclusive phrase

	result := rule.Check(content, nil)
	if !result.Passed {
		t.Errorf("expected content without conclusive phrase to pass, got: %s", result.Message)
	}
}

// --- KeywordRule Tests ---

func TestKeywordRule_Blocked(t *testing.T) {
	rule := NewKeywordRule([]string{"恶意代码"}, nil, nil, "")
	result := rule.Check("这里包含恶意代码的内容。", nil)

	if result.Passed {
		t.Error("expected blocked phrase to be detected")
	}
	if result.Action != ActionBlock {
		t.Errorf("expected ActionBlock, got %s", result.Action)
	}
}

func TestKeywordRule_RiskKeyword(t *testing.T) {
	rule := NewKeywordRule(nil, []string{"侵权"}, nil, "请咨询律师。")
	result := rule.Check("该产品可能构成侵权。", nil)

	if result.Passed {
		t.Error("expected risk keyword to trigger")
	}
	if result.Action != ActionInject {
		t.Errorf("expected ActionInject, got %s", result.Action)
	}
	if !strings.Contains(result.Message, "请咨询律师") {
		t.Errorf("expected disclaimer in message, got: %s", result.Message)
	}
}

func TestKeywordRule_NoMatch(t *testing.T) {
	rule := NewKeywordRule([]string{"恶意代码"}, []string{"侵权"}, nil, "")
	result := rule.Check("这是正常的技术分析内容。", nil)

	if !result.Passed {
		t.Errorf("expected clean content to pass, got: %s", result.Message)
	}
}

// --- PipelineHook Tests ---

func TestPipelineHook_Integration(t *testing.T) {
	pipeline := NewRulePipeline(
		NewKeywordRule([]string{"恶意代码"}, []string{"侵权"}, nil, "请咨询律师。"),
		NewConfidenceCheckRule(),
	)
	hook := NewPipelineHook(pipeline)

	// Simulate model call context
	originalContent := "综上所述，该产品可能构成侵权。"
	mcc := &iface.ModelCallContext{Content: originalContent}
	hook.AfterModelCall(nil, nil, mcc)

	if mcc.Content == originalContent {
		t.Error("expected content to be modified by pipeline")
	}
	if !strings.Contains(mcc.Content, "AI 生成") {
		t.Errorf("expected confidence disclaimer, got: %s", mcc.Content)
	}
	if !strings.Contains(mcc.Content, "请咨询律师") {
		t.Errorf("expected risk keyword disclaimer, got: %s", mcc.Content)
	}
}
