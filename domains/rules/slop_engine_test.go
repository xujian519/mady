package rules

import (
	"strings"
	"testing"
)

func TestAnalyzeSlop_PhraseReplacement(t *testing.T) {
	text := "进一步地，本领域技术人员能够理解，该技术方案具有显著进步。"
	a := AnalyzeSlop(text)

	if len(a.Changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	if strings.Contains(a.Cleaned, "进一步地") {
		t.Error("filler 「进一步地」 was not removed")
	}
	if strings.Contains(a.Cleaned, "本领域技术人员能够理解") {
		t.Error("qualifier 「本领域技术人员能够理解」 was not removed")
	}
	if strings.Contains(a.Cleaned, "具有显著进步") {
		t.Error("qualifier 「具有显著进步」 was not removed")
	}
}

func TestAnalyzeSlop_SystemModifier(t *testing.T) {
	text := "对该技术方案进行了系统分析。"
	a := AnalyzeSlop(text)

	found := false
	for _, c := range a.Changes {
		if c.Original == "空洞修饰「系统化」" {
			found = true
		}
	}
	if !found {
		t.Error("expected system modifier change, not found")
	}
	if strings.Contains(a.Cleaned, "系统分析") {
		t.Error("「系统分析」 was not replaced with 「分析」")
	}
	if !strings.Contains(a.Cleaned, "分析") {
		t.Error("expected 「分析」 in cleaned text")
	}
}

func TestAnalyzeSlop_StructureEmptyThreeStep(t *testing.T) {
	text := "区别特征："
	a := AnalyzeSlop(text)

	found := false
	for _, iss := range a.Issues {
		if iss.Type == IssueEmptyThreeStep {
			found = true
		}
	}
	if !found {
		t.Error("expected empty_three_step issue for short line with 区别特征")
	}
}

func TestAnalyzeSlop_StructureBinaryTurn(t *testing.T) {
	text := "这并不是新颖性问题，而是创造性问题。"
	a := AnalyzeSlop(text)

	found := false
	for _, iss := range a.Issues {
		if iss.Type == IssueBinaryTurn {
			found = true
		}
	}
	if !found {
		t.Error("expected binary_turn issue")
	}
}

func TestAnalyzeSlop_StructureOaFormula(t *testing.T) {
	text := "审查员认定有误，该区别技术特征未被对比文件公开。"
	a := AnalyzeSlop(text)

	found := false
	for _, iss := range a.Issues {
		if iss.Type == IssueOaFormula {
			found = true
		}
	}
	if !found {
		t.Error("expected oa_formula issue")
	}
}

func TestAnalyzeSlop_StructureReasonPile(t *testing.T) {
	text := "不符合第1条、第2条、第3条、第4条的规定"
	a := AnalyzeSlop(text)

	found := false
	for _, iss := range a.Issues {
		if iss.Type == IssueReasonPile {
			found = true
		}
	}
	if !found {
		t.Error("expected reason_pile issue for 4+ citations")
	}
}

func TestAnalyzeSlop_Score(t *testing.T) {
	text := `争点在于区别技术特征是否被D1公开。

D1¶0123公开了特征A。权利要求1的该特征与D1的区别在于B。

根据第22条第3款，该区别不具备创造性。`
	a := AnalyzeSlop(text)

	if a.Score.Total <= 0 {
		t.Errorf("score total = %d, expected > 0", a.Score.Total)
	}
	if a.Score.Directness != 8 {
		t.Errorf("directness = %d, expected 8 (争点 found in first 200 chars)", a.Score.Directness)
	}
}

func TestAnalyzeSlop_Exaggeration(t *testing.T) {
	text := "该技术方案具有革命性的质的飞跃。"
	a := AnalyzeSlop(text)

	found := false
	for _, c := range a.Checklist {
		if strings.Contains(c.Question, "显著") && !c.Passed {
			found = true
		}
	}
	if !found {
		t.Error("expected exaggeration checklist item to fail")
	}
}

func TestFormatSlopAnalysis_NotEmpty(t *testing.T) {
	text := "进一步地，显而易见地，该方案具有显著进步。"
	a := AnalyzeSlop(text)
	s := FormatSlopAnalysis(a)

	if s == "" {
		t.Fatal("FormatSlopAnalysis returned empty string")
	}
	if !strings.Contains(s, "反套话润色报告") {
		t.Error("missing report header")
	}
	if !strings.Contains(s, "评分") {
		t.Error("missing score section")
	}
}

func TestFormatSlopAnalysis_CleanText(t *testing.T) {
	text := "争点明确。D1¶0123公开了特征A。"
	a := AnalyzeSlop(text)
	s := FormatSlopAnalysis(a)

	if !strings.Contains(s, "✅ 通过") && !strings.Contains(s, "❌ 需修订") {
		t.Error("missing pass/fail indicator")
	}
}

func TestAnalyzeSlop_CleansWhitespace(t *testing.T) {
	text := "测试文本\n\n\n\n\n更多文本"
	a := AnalyzeSlop(text)

	if strings.Contains(a.Cleaned, "\n\n\n") {
		t.Error("triple newlines should be collapsed to double")
	}
}
