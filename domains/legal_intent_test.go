package domains

import (
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
)

func TestDetectLegalIntent_ExplicitTrigger(t *testing.T) {
	result := DetectLegalIntent("@legal 帮我分析一下这个案子")

	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true for @legal prefix")
	}
	if !result.ExplicitTrigger {
		t.Error("expected ExplicitTrigger=true")
	}
	if result.Confidence != 1 {
		t.Errorf("confidence = %v, want 1", result.Confidence)
	}
	if result.CaseType != reasoning.CaseGeneralLegal {
		t.Errorf("caseType = %v, want general_legal", result.CaseType)
	}
}

func TestDetectLegalIntent_Invalidation(t *testing.T) {
	result := DetectLegalIntent("我要对该专利提出无效宣告请求")

	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true")
	}
	if result.CaseType != reasoning.CaseInvalidation {
		t.Errorf("caseType = %v, want invalidation", result.CaseType)
	}
	if result.SuggestedMode != ModeFlexiblePlan {
		t.Errorf("mode = %v, want flexible_plan", result.SuggestedMode)
	}
}

func TestDetectLegalIntent_NoveltyJudgment(t *testing.T) {
	result := DetectLegalIntent("请帮我做新颖性判断")

	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true")
	}
	if result.CaseType != reasoning.CaseNoveltySearch {
		t.Errorf("caseType = %v, want novelty_search", result.CaseType)
	}
	if result.SuggestedMode != ModeJudgment {
		t.Errorf("mode = %v, want judgment", result.SuggestedMode)
	}
}

func TestDetectLegalIntent_OAResponse(t *testing.T) {
	result := DetectLegalIntent("审查意见通知书到了，需要答复OA")

	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true")
	}
	if result.CaseType != reasoning.CaseOAResponse {
		t.Errorf("caseType = %v, want oa_response", result.CaseType)
	}
}

func TestDetectLegalIntent_OASubstringNoFalsePositive(t *testing.T) {
	result := DetectLegalIntent("I want to approach this case differently")

	if result.IsLegalIntent {
		t.Error("expected IsLegalIntent=false for 'approach' (oa substring)")
	}
}

func TestDetectLegalIntent_NoLegalIntent(t *testing.T) {
	result := DetectLegalIntent("今天天气真好")

	if result.IsLegalIntent {
		t.Error("expected IsLegalIntent=false for non-legal text")
	}
	if result.SuggestedMode != ModeDirect {
		t.Errorf("mode = %v, want direct", result.SuggestedMode)
	}
}

func TestDetectLegalIntent_ClarityRequiresPatentContext(t *testing.T) {
	result1 := DetectLegalIntent("这个表达不清楚")
	if result1.IsLegalIntent {
		t.Error("expected IsLegalIntent=false without patent context for 模糊关键词")
	}

	result2 := DetectLegalIntent("权利要求的保护范围不清楚，不符合A26.4")
	if !result2.IsLegalIntent {
		t.Error("expected IsLegalIntent=true with patent context")
	}
	if result2.CaseType != reasoning.CaseInvalidation {
		t.Errorf("caseType = %v, want invalidation", result2.CaseType)
	}
}

func TestDetectLegalIntent_ClarityWithArticleID(t *testing.T) {
	result := DetectLegalIntent("A26.4 该权利要求不清楚")
	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true when A26.4 article ID present")
	}
}

func TestDetectLegalIntent_Drafting(t *testing.T) {
	result := DetectLegalIntent("帮我写专利申请文件")
	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true")
	}
	if result.CaseType != reasoning.CaseDrafting {
		t.Errorf("caseType = %v, want drafting", result.CaseType)
	}
}

func TestDetectLegalIntent_FTO(t *testing.T) {
	result := DetectLegalIntent("需要做FTO自由实施分析")
	if !result.IsLegalIntent {
		t.Error("expected IsLegalIntent=true")
	}
	if result.CaseType != reasoning.CaseFTO {
		t.Errorf("caseType = %v, want fto", result.CaseType)
	}
}

func TestDetectLegalIntent_KeywordSubstringDedup(t *testing.T) {
	result := DetectLegalIntent("该专利权不清楚")
	if !result.IsLegalIntent {
		t.Fatal("expected IsLegalIntent=true")
	}

	// MatchedKeywords uses simple filter (no dedup), but the dedup
	// prevents over-counting: "不清楚" and "清楚" should count as 1,
	// not 2. Confidence should reflect count=1 out of 4 keywords.
	if result.Confidence > 0.3 {
		t.Errorf("confidence = %v, expected <= 0.3 (1 match out of 4 keywords after dedup)", result.Confidence)
	}
}

func TestSelectRunMode(t *testing.T) {
	tests := []struct {
		name               string
		hasPredefinedSteps bool
		userFlexible       bool
		want               RunMode
	}{
		{"predefined_no_flex", true, false, ModeJudgment},
		{"predefined_with_flex", true, true, ModeFlexiblePlan},
		{"no_predefined", false, false, ModeFlexiblePlan},
		{"no_predefined_flex", false, true, ModeFlexiblePlan},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectRunMode(reasoning.CaseInvalidation, tt.hasPredefinedSteps, tt.userFlexible)
			if got != tt.want {
				t.Errorf("SelectRunMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildLegalSuggestion(t *testing.T) {
	s := buildLegalSuggestion(reasoning.CaseOAResponse, ModeFlexiblePlan, []string{"OA", "审查意见"})
	if s == "" {
		t.Fatal("empty suggestion")
	}
}
