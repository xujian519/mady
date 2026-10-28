package specdrafting

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/domains/rulekit"
)

const validSpec = `## 技术领域
本发明涉及一种散热壳体。

## 背景技术
现有壳体的散热效率较低。

## 发明内容
本发明提供一种散热壳体。

## 附图说明
图1为本发明散热壳体的结构示意图。

## 具体实施方式
所述壳体10包括底板和侧板。
`

func ruleNames(violations []Violation) []string {
	names := make([]string, 0, len(violations))
	for _, violation := range violations {
		names = append(names, violation.RuleName)
	}
	return names
}

func hasRule(violations []Violation, name string) bool {
	for _, violation := range violations {
		if violation.RuleName == name {
			return true
		}
	}
	return false
}

func TestCheckSpecStyleValid(t *testing.T) {
	if violations := CheckSpecStyle(validSpec, "本发明公开了一种散热壳体。"); len(violations) != 0 {
		t.Fatalf("合规说明书不应有形式违规，got %v", ruleNames(violations))
	}
}

func TestCheckHeadingSet(t *testing.T) {
	text := validSpec + "\n## 要解决的技术问题\n如何提高散热效率\n"
	violations := CheckHeadingSet(text)
	if len(violations) != 1 || !strings.Contains(violations[0].Message, "要解决的技术问题") {
		t.Fatalf("violations = %+v", violations)
	}
	if violations[0].Severity != SeverityError {
		t.Errorf("severity = %q", violations[0].Severity)
	}
}

func TestCheckSectionOrder(t *testing.T) {
	reordered := `## 背景技术
现有技术。

## 技术领域
本发明涉及壳体。

## 发明内容
方案。

## 附图说明
图1。

## 具体实施方式
实施例。
`
	violations := CheckSectionOrder(reordered)
	if len(violations) != 1 {
		t.Fatalf("violations = %+v", violations)
	}
	if !strings.Contains(violations[0].Message, "实际为 背景技术") {
		t.Errorf("message 应写出实际顺序，got %q", violations[0].Message)
	}

	// 五部分不齐时不报顺序（缺失由结构检查负责）。
	incomplete := "## 背景技术\n现有技术。\n## 技术领域\n壳体。"
	if got := CheckSectionOrder(incomplete); len(got) != 0 {
		t.Errorf("五部分不齐时不应报顺序，got %+v", got)
	}
}

func TestCheckFigureMarkParentheses(t *testing.T) {
	text := "所述壳体（10）与盖板（20）连接；实施例（1）中图（2）所示；连接件（4）固定。"
	violations := CheckFigureMarkParentheses(text)
	names := ruleNames(violations)
	if len(violations) != 3 {
		t.Fatalf("violations = %v", names)
	}
	joined := ""
	for _, violation := range violations {
		joined += violation.Message
	}
	for _, want := range []string{"壳体（10）", "盖板（20）", "连接件（4）"} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少 %q：%s", want, joined)
		}
	}
	if strings.Contains(joined, "实施例（1）") || strings.Contains(joined, "图（2）") {
		t.Errorf("列举/引用形态不应报：%s", joined)
	}
	if violations[0].Severity != SeverityError {
		t.Errorf("severity = %q", violations[0].Severity)
	}
}

func TestCheckFigureNumberSpacing(t *testing.T) {
	if got := CheckFigureNumberSpacing("如图 1 所示，图2为剖视图。"); len(got) != 1 {
		t.Fatalf("violations = %+v", got)
	}
	if got := CheckFigureNumberSpacing("如图1所示。"); len(got) != 0 {
		t.Errorf("无空格不应报，got %+v", got)
	}
}

func TestCheckParagraphNumbering(t *testing.T) {
	violations := CheckParagraphNumbering("[0001] 本发明涉及一种壳体。")
	// 提示级（info）：判级策略下 info 达到 3 条才 needs_revision，单条不阻断。
	if len(violations) != 1 || violations[0].Severity != SeverityInfo {
		t.Fatalf("violations = %+v", violations)
	}
}

func TestCheckAbstractHeading(t *testing.T) {
	if got := CheckAbstractHeading("## 摘要\n本发明公开了一种壳体。"); len(got) != 1 {
		t.Fatalf("摘要标题应报错，got %+v", got)
	}
	if got := CheckAbstractHeading("本发明公开了一种壳体。"); len(got) != 0 {
		t.Errorf("无标题不应报，got %+v", got)
	}
}

func TestCheckEffectQuantification(t *testing.T) {
	hits := CheckEffectQuantification("散热效果显著。热导率提升12%。性能大幅提升；成本降低5%。")
	if len(hits) != 2 {
		t.Fatalf("hits = %v", hits)
	}
	if hits[0] != "散热效果显著" {
		t.Errorf("hits[0] = %q", hits[0])
	}
	if strings.Contains(strings.Join(hits, ""), "12%") {
		t.Errorf("带数字的句子不应命中：%v", hits)
	}
}

func TestCheckChemicalCharacterization(t *testing.T) {
	missing := CheckChemicalCharacterization("本发明提供一种化合物。")
	if len(missing) != len(chemCharacterizationTerms) {
		t.Errorf("missing = %d, want %d", len(missing), len(chemCharacterizationTerms))
	}
	if got := CheckChemicalCharacterization("产物熔点为 120℃。"); len(got) != len(chemCharacterizationTerms)-1 {
		t.Errorf("命中「熔点」后应少一项，got %d", len(got))
	}
}

func TestCheckQuantification(t *testing.T) {
	general := CheckQuantification("散热效果显著。", DomainGeneral)
	if len(general) != 1 || general[0].RuleName != "effect_quantification" {
		t.Fatalf("general = %+v", general)
	}
	if general[0].Severity != SeverityInfo {
		t.Errorf("severity = %q, want info（提示级，不单独阻断）", general[0].Severity)
	}
	chemical := CheckQuantification("本发明提供一种化合物，效果显著。", DomainChemical)
	if !hasRule(chemical, "chemical_characterization") {
		t.Errorf("化学领域缺表征应报错，got %v", ruleNames(chemical))
	}
	withNMR := CheckQuantification("经核磁共振（NMR）确认结构，产率95%。", DomainChemical)
	for _, violation := range withNMR {
		if violation.RuleName == "chemical_characterization" {
			t.Errorf("已有 NMR 不应报表征缺失")
		}
	}
	if got := CheckQuantification("", DomainChemical); len(got) != 0 {
		t.Errorf("空文本不应报，got %v", ruleNames(got))
	}
}

func TestRenderSpecificationText(t *testing.T) {
	spec := &SpecOutput{Sections: []SpecSection{
		{Name: SecTechField, Content: "本发明涉及一种壳体。"},
		{Name: SecBackground, Title: "背景技术", Content: "现有技术。"},
	}}
	text := renderSpecificationText(spec)
	if !strings.Contains(text, "## 技术领域") || !strings.Contains(text, "## 背景技术") {
		t.Errorf("rendered = %q", text)
	}
}

func TestScoreWithFormViolationsKeepsScoreAndVerdictConsistent(t *testing.T) {
	scorer := NewSpecScorer(NewRuleEngine())
	spec := &SpecOutput{
		Title: "一种散热壳体",
		Sections: []SpecSection{
			{Name: SecTechField, Title: "技术领域", Content: "本发明涉及一种散热壳体。"},
			{Name: SecBackground, Title: "背景技术", Content: "现有壳体散热较差。"},
			{Name: SecContent, Title: "发明内容", Content: "本发明提供一种散热壳体。"},
			{Name: SecDrawings, Title: "附图说明", Content: "图1为结构示意图。"},
			{Name: SecEmbodiment, Title: "具体实施方式", Content: "所述壳体10包括底板。"},
		},
	}
	baseline := scorer.Score(spec, SpecInput{})
	extra := []Violation{{
		RuleName: "heading_set", Severity: SeverityError, Message: "多余标题",
	}}
	report := scorer.ScoreWith(spec, SpecInput{}, extra)
	if report.Verdict != rulekit.VerdictBlocked {
		t.Fatalf("verdict = %q, want blocked", report.Verdict)
	}
	if report.OverallScore >= baseline.OverallScore {
		t.Errorf("形式违规应参与扣分：baseline=%.1f report=%.1f", baseline.OverallScore, report.OverallScore)
	}
	if report.Grade == "A" && report.Verdict == rulekit.VerdictBlocked {
		t.Error("分数等级与判级不应自相矛盾：A 级不得同时 blocked")
	}
}
