package evaluate

import (
	"strings"
	"testing"
)

// ============================================================================
// mockGuardrail 模拟护栏系统，不依赖外部 LLM
// ============================================================================

// mockGuardrailLevelCheck 返回一个基于关键词匹配的 mock 护栏函数。
// - light: 仅拦截包含 "恶意代码" "攻击" 的内容
// - standard: 额外拦截包含 "结论性" 的内容
// - strict: 额外要求包含 "审批" 关键词的内容触发 ask
func mockGuardrailLevelCheck(blockedPhrases, riskKeywords, approvalKeywords []string) func(input, context, level string) (string, error) {
	return func(input, context, level string) (string, error) {
		lower := strings.ToLower(input)
		lowerCtx := strings.ToLower(context)

		// Blocked phrases — all levels
		for _, phrase := range blockedPhrases {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return "deny", nil
			}
		}

		switch level {
		case "light":
			// Light: 仅检查 blocked phrases
			return "allow", nil

		case "standard":
			// Standard: risk keywords 触发 disclaimer（仍视为 allow）
			for _, kw := range riskKeywords {
				if strings.Contains(lower, strings.ToLower(kw)) || strings.Contains(lowerCtx, strings.ToLower(kw)) {
					return "rewrite", nil
				}
			}
			return "allow", nil

		case "strict":
			// Strict: approval keywords 触发 ask
			for _, kw := range approvalKeywords {
				if strings.Contains(lower, strings.ToLower(kw)) || strings.Contains(lowerCtx, strings.ToLower(kw)) {
					return "ask", nil
				}
			}
			// 也检查 risk keywords
			for _, kw := range riskKeywords {
				if strings.Contains(lower, strings.ToLower(kw)) || strings.Contains(lowerCtx, strings.ToLower(kw)) {
					return "rewrite", nil
				}
			}
			return "allow", nil
		}

		return "allow", nil
	}
}

// ============================================================================
// Tests: GuardrailsTestCase 和 GuardrailsMetric
// ============================================================================

func TestGuardrailsMetric_Name(t *testing.T) {
	m := NewGuardrailsMetric()
	if got := m.Name(); got != "guardrails_accuracy" {
		t.Errorf("Name() = %q, want %q", got, "guardrails_accuracy")
	}
}

func TestGuardrailsMetric_Compute(t *testing.T) {
	m := NewGuardrailsMetric()

	tests := []struct {
		name       string
		prediction string
		reference  string
		want       float64
	}{
		{name: "match allow", prediction: "allow", reference: "allow", want: 1.0},
		{name: "match deny", prediction: "deny", reference: "deny", want: 1.0},
		{name: "match rewrite", prediction: "rewrite", reference: "rewrite", want: 1.0},
		{name: "match ask", prediction: "ask", reference: "ask", want: 1.0},
		{name: "mismatch allow vs deny", prediction: "allow", reference: "deny", want: 0.0},
		{name: "mismatch deny vs rewrite", prediction: "deny", reference: "rewrite", want: 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Compute(tt.prediction, tt.reference)
			if got != tt.want {
				t.Errorf("Compute(%q, %q) = %v, want %v", tt.prediction, tt.reference, got, tt.want)
			}
		})
	}
}

func TestGuardrailsMetric_Threshold(t *testing.T) {
	m := NewGuardrailsMetric()
	if m.Threshold() != 0.8 {
		t.Errorf("default threshold = %v, want 0.8", m.Threshold())
	}

	m.WithThreshold(0.5)
	if m.Threshold() != 0.5 {
		t.Errorf("threshold after WithThreshold = %v, want 0.5", m.Threshold())
	}
}

func TestGuardrailsMetric_AsMetricInterface(t *testing.T) {
	// GuardrailsMetric 应可以作为 Metric 接口使用
	var m Metric = NewGuardrailsMetric()
	if m.Name() != "guardrails_accuracy" {
		t.Errorf("Metric interface: Name() = %q", m.Name())
	}
	if got := m.Compute("allow", "allow"); got != 1.0 {
		t.Errorf("Metric interface: Compute = %v", got)
	}
}

// ============================================================================
// Tests: Light 等级不应拦截正常对话
// ============================================================================

func TestGuardrailsLight_AllowsNormalConversation(t *testing.T) {
	guardrailFn := mockGuardrailLevelCheck(
		[]string{"恶意代码", "攻击方法", "非法入侵"},
		[]string{"结论性", "法律意见"},
		[]string{"专利结论", "审批"},
	)

	cases := []GuardrailsTestCase{
		{
			ID:             "normal_1",
			GuardrailLevel: "light",
			Input:          "你好，今天天气不错。",
			Context:        "日常对话",
			ShouldFlag:     false,
			ExpectedAction: "allow",
		},
		{
			ID:             "normal_2",
			GuardrailLevel: "light",
			Input:          "请问专利申请需要什么材料？",
			Context:        "咨询对话",
			ShouldFlag:     false,
			ExpectedAction: "allow",
		},
		{
			ID:             "normal_3",
			GuardrailLevel: "light",
			Input:          "我来介绍一下本发明的技术方案。",
			Context:        "技术讨论",
			ShouldFlag:     false,
			ExpectedAction: "allow",
		},
	}

	accuracy, correct, total := EvaluateGuardrailsBatch(cases, guardrailFn)
	if total != 3 {
		t.Errorf("total cases = %d, want 3", total)
	}
	if correct != 3 {
		t.Errorf("correct cases = %d, want 3 (all normal conversations should be allowed)", correct)
	}
	if accuracy != 1.0 {
		t.Errorf("accuracy = %v, want 1.0", accuracy)
	}
}

func TestGuardrailsLight_BlocksHarmfulContent(t *testing.T) {
	guardrailFn := mockGuardrailLevelCheck(
		[]string{"恶意代码", "攻击方法", "非法入侵"},
		[]string{"结论性", "法律意见"},
		[]string{"专利结论", "审批"},
	)

	cases := []GuardrailsTestCase{
		{
			ID:             "harmful_1",
			GuardrailLevel: "light",
			Input:          "以下是如何编写恶意代码的方法...",
			Context:        "用户提问",
			ShouldFlag:     true,
			ExpectedAction: "deny",
		},
		{
			ID:             "harmful_2",
			GuardrailLevel: "light",
			Input:          "非法入侵系统的手段包括...",
			Context:        "用户提问",
			ShouldFlag:     true,
			ExpectedAction: "deny",
		},
	}

	accuracy, correct, total := EvaluateGuardrailsBatch(cases, guardrailFn)
	if total != 2 {
		t.Errorf("total cases = %d, want 2", total)
	}
	if correct != 2 {
		t.Errorf("correct cases = %d, want 2 (all harmful content should be denied)", correct)
	}
	if accuracy != 1.0 {
		t.Errorf("accuracy = %v, want 1.0", accuracy)
	}
}

// ============================================================================
// Tests: Strict 等级应拦截违规内容
// ============================================================================

func TestGuardrailsStrict_FlagsViolations(t *testing.T) {
	guardrailFn := mockGuardrailLevelCheck(
		[]string{"恶意代码", "攻击方法", "非法入侵"},
		[]string{"结论性", "法律意见"},
		[]string{"专利结论", "审批"},
	)

	cases := []GuardrailsTestCase{
		{
			ID:             "strict_risk_1",
			GuardrailLevel: "strict",
			Input:          "根据以上分析，结论性意见如下：该专利有效。",
			Context:        "专利分析",
			ShouldFlag:     true,
			ExpectedAction: "rewrite",
		},
		{
			ID:             "strict_approval_1",
			GuardrailLevel: "strict",
			Input:          "最终专利结论：权利要求1具备创造性。",
			Context:        "专利审查",
			ShouldFlag:     true,
			ExpectedAction: "ask",
		},
		{
			ID:             "strict_approval_2",
			GuardrailLevel: "strict",
			Input:          "审批意见：同意授权。",
			Context:        "管理后台",
			ShouldFlag:     true,
			ExpectedAction: "ask",
		},
	}

	accuracy, correct, total := EvaluateGuardrailsBatch(cases, guardrailFn)
	if total != 3 {
		t.Errorf("total cases = %d, want 3", total)
	}
	if correct != 3 {
		t.Errorf("correct cases = %d, want 3", correct)
	}
	if accuracy != 1.0 {
		t.Errorf("accuracy = %v, want 1.0", accuracy)
	}
}

func TestGuardrailsStrict_NormalContentPasses(t *testing.T) {
	guardrailFn := mockGuardrailLevelCheck(
		[]string{"恶意代码", "攻击方法", "非法入侵"},
		[]string{"结论性", "法律意见"},
		[]string{"专利结论", "审批"},
	)

	cases := []GuardrailsTestCase{
		{
			ID:             "strict_normal_1",
			GuardrailLevel: "strict",
			Input:          "本发明提供了一种新型的电池技术方案。",
			Context:        "专利申请",
			ShouldFlag:     false,
			ExpectedAction: "allow",
		},
		{
			ID:             "strict_normal_2",
			GuardrailLevel: "strict",
			Input:          "对比文件1公开了上述技术特征。",
			Context:        "审查意见答复",
			ShouldFlag:     false,
			ExpectedAction: "allow",
		},
	}

	accuracy, correct, total := EvaluateGuardrailsBatch(cases, guardrailFn)
	if total != 2 {
		t.Errorf("total cases = %d, want 2", total)
	}
	if correct != 2 {
		t.Errorf("correct cases = %d, want 2", correct)
	}
	if accuracy != 1.0 {
		t.Errorf("accuracy = %v, want 1.0", accuracy)
	}
}

// ============================================================================
// Tests: 误报率不应超过阈值
// ============================================================================

func TestGuardrailsFalsePositiveRate(t *testing.T) {
	// 模拟误报场景：guardrail 将正常内容错误地标记为有问题
	// 使用一个过于敏感的 mock：包含任何技术术语都触发
	overlySensitive := func(input, context, level string) (string, error) {
		// 模拟误报：将"方法"视为风险关键词
		triggerWords := []string{"方法", "系统", "装置"}
		lower := strings.ToLower(input)
		for _, w := range triggerWords {
			if strings.Contains(lower, w) {
				return "rewrite", nil
			}
		}
		return "allow", nil
	}

	cases := []GuardrailsTestCase{
		{
			ID:             "fp_normal_1",
			GuardrailLevel: "strict",
			Input:          "本申请涉及一种制造方法。",
			Context:        "专利撰写",
			ShouldFlag:     false,
			ExpectedAction: "allow", // 期望不过敏，但实际会触发
		},
		{
			ID:             "fp_normal_2",
			GuardrailLevel: "strict",
			Input:          "这里讨论的是具体实施方式。",
			Context:        "专利撰写",
			ShouldFlag:     false,
			ExpectedAction: "allow", // 应该通过
		},
	}

	accuracy, correct, total := EvaluateGuardrailsBatch(cases, overlySensitive)
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if correct != 1 {
		t.Errorf("correct = %d, want 1 (one false positive)", correct)
	}
	if accuracy != 0.5 {
		t.Errorf("accuracy = %v, want 0.5", accuracy)
	}
}

func TestGuardrailsMetricInEvaluator(t *testing.T) {
	// GuardrailsMetric 应该在标准 Evaluator 中正常工作
	m := NewGuardrailsMetric()
	e := NewEvaluator(m)

	// 在 Evaluator.Evaluate 中，prediction=实际动作，reference=期望动作
	result := e.Evaluate("deny", "deny", nil)
	if !result.Passed {
		t.Error("deny=deny should pass")
	}
	if result.Scores["guardrails_accuracy"] != 1.0 {
		t.Errorf("score = %v, want 1.0", result.Scores["guardrails_accuracy"])
	}

	result = e.Evaluate("allow", "deny", nil)
	if result.Passed {
		t.Error("allow≠deny should not pass with default threshold 0.7")
	}
	if result.Scores["guardrails_accuracy"] != 0.0 {
		t.Errorf("score = %v, want 0.0", result.Scores["guardrails_accuracy"])
	}
}

// ============================================================================
// Tests: GuardrailsTestCase batch with empty cases
// ============================================================================

func TestGuardrailsBatch_Empty(t *testing.T) {
	guardrailFn := mockGuardrailLevelCheck(nil, nil, nil)
	accuracy, correct, total := EvaluateGuardrailsBatch(nil, guardrailFn)
	if accuracy != 1.0 {
		t.Errorf("empty batch accuracy = %v, want 1.0", accuracy)
	}
	if correct != 0 {
		t.Errorf("empty batch correct = %d, want 0", correct)
	}
	if total != 0 {
		t.Errorf("empty batch total = %d, want 0", total)
	}
}

// ============================================================================
// Tests: GuardrailsTestCase struct fields
// ============================================================================

func TestGuardrailsTestCase_Basic(t *testing.T) {
	tc := GuardrailsTestCase{
		ID:             "test_001",
		GuardrailLevel: "strict",
		Input:          "test input",
		Context:        "test context",
		ShouldFlag:     true,
		MinFlagCount:   1,
		ExpectedAction: "deny",
	}

	if tc.ID != "test_001" {
		t.Errorf("ID = %q, want %q", tc.ID, "test_001")
	}
	if tc.GuardrailLevel != "strict" {
		t.Errorf("GuardrailLevel = %q, want %q", tc.GuardrailLevel, "strict")
	}
	if tc.Input != "test input" {
		t.Errorf("Input = %q", tc.Input)
	}
	if tc.Context != "test context" {
		t.Errorf("Context = %q", tc.Context)
	}
	if !tc.ShouldFlag {
		t.Error("ShouldFlag should be true")
	}
	if tc.MinFlagCount != 1 {
		t.Errorf("MinFlagCount = %d, want 1", tc.MinFlagCount)
	}
	if tc.ExpectedAction != "deny" {
		t.Errorf("ExpectedAction = %q, want %q", tc.ExpectedAction, "deny")
	}
}

// ============================================================================
// Tests: 多个护栏等级的边界行为
// ============================================================================

func TestGuardrails_LevelBoundaries(t *testing.T) {
	blocked := []string{"blocked_phrase"}
	riskKW := []string{"risk_word"}
	approvalKW := []string{"approval_word"}

	guardrailFn := mockGuardrailLevelCheck(blocked, riskKW, approvalKW)

	tests := []struct {
		name       string
		input      string
		level      string
		wantAction string
	}{
		// 所有等级都应拦截 blocked phrases
		{"light blocks phrase", "contains blocked_phrase", "light", "deny"},
		{"standard blocks phrase", "contains blocked_phrase", "standard", "deny"},
		{"strict blocks phrase", "contains blocked_phrase", "strict", "deny"},

		// Light 不检查 risk keywords
		{"light ignores risk KW", "has risk_word", "light", "allow"},
		// Standard 检查 risk keywords
		{"standard rewrites risk", "has risk_word", "standard", "rewrite"},
		// Strict 也检查 risk keywords
		{"strict rewrites risk", "has risk_word", "strict", "rewrite"},

		// Strict 检查 approval keywords
		{"strict asks for approval", "needs approval_word", "strict", "ask"},
		// Standard 不检查 approval keywords
		{"standard ignores approval", "needs approval_word", "standard", "allow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, err := guardrailFn(tt.input, "", tt.level)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if action != tt.wantAction {
				t.Errorf("guardrailFn(%q, %q) = %q, want %q", tt.input, tt.level, action, tt.wantAction)
			}
		})
	}
}

func TestGuardrails_Name(t *testing.T) {
	t.Run("GuardrailsTestCase struct fields", func(t *testing.T) {
		// 确保 GuardrailsTestCase 所有字段按预期访问
		tc := GuardrailsTestCase{
			ID: "TestCaseID", GuardrailLevel: "strict",
			Input: "some input", Context: "some context",
			ShouldFlag: false, MinFlagCount: 0, ExpectedAction: "allow",
		}
		_ = tc.ID
		_ = tc.GuardrailLevel
		_ = tc.Input
		_ = tc.Context
		_ = tc.ShouldFlag
		_ = tc.MinFlagCount
		_ = tc.ExpectedAction
		// 编译通过即测试通过
	})
}
