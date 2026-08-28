package claimchart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// ValidateQuote — 引文逐字子串校验
// =============================================================================

func TestValidateQuote_ExactSubstring(t *testing.T) {
	source := "所述壳体包括底座和上盖，所述底座设有散热孔。"
	if !ValidateQuote("所述底座设有散热孔", source) {
		t.Error("exact substring should validate")
	}
}

func TestValidateQuote_ToleratesWhitespaceAndLineBreaks(t *testing.T) {
	// PDF 折行：源文换行处插入空白后，引文仍须可验证。
	source := "所述壳体包括底座和\n  上盖，所述底座设有散热孔。"
	if !ValidateQuote("所述壳体包括底座和上盖", source) {
		t.Error("quote spanning a line break should validate")
	}
	if !ValidateQuote("底座 设有\n散热孔", source) {
		t.Error("quote with internal whitespace should validate against normalized source")
	}
}

func TestValidateQuote_Fabricated(t *testing.T) {
	source := "所述壳体包括底座和上盖。"
	if ValidateQuote("所述壳体包括铝合金上盖", source) {
		t.Error("fabricated quote must not validate")
	}
}

func TestValidateQuote_Empty(t *testing.T) {
	if ValidateQuote("", "some source") {
		t.Error("empty quote must not validate")
	}
}

// =============================================================================
// extractParagraphLocators / normalizeLocator
// =============================================================================

func TestExtractParagraphLocators(t *testing.T) {
	locs := extractParagraphLocators("见说明书第[0032]段及[0041]段，另见［0102］。")
	want := []string{"[0032]", "[0041]", "［0102］"}
	if len(locs) != len(want) {
		t.Fatalf("got %v, want %v", locs, want)
	}
	for i := range want {
		if locs[i] != want[i] {
			t.Errorf("loc[%d] = %q, want %q", i, locs[i], want[i])
		}
	}
	if got := extractParagraphLocators("无定位符文本"); got != nil {
		t.Errorf("expected nil for text without locators, got %v", got)
	}
}

func TestNormalizeLocator(t *testing.T) {
	if got := normalizeLocator("［0032］"); got != "0032" {
		t.Errorf("normalizeLocator = %q, want 0032", got)
	}
}

// =============================================================================
// validateRow / ValidateChart — 单行与整表校验
// =============================================================================

func TestValidateRow_QuoteMismatch(t *testing.T) {
	row := ChartRow{ElementID: "1a", TargetID: "D1", Quote: "源文中不存在的句子"}
	issues := validateRow(0, row, "源文：所述装置包括传感器。")
	found := false
	for _, iss := range issues {
		if iss.Kind == IssueQuoteMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected quote-mismatch issue, got %v", issues)
	}
}

func TestValidateRow_LocatorMissing(t *testing.T) {
	row := ChartRow{
		ElementID: "1a",
		TargetID:  "D1",
		Quote:     "所述装置包括传感器",
		PinCite:   "[D1 [0099]]", // 源文中只有 [0032]
	}
	source := "[0032] 所述装置包括传感器，所述传感器采集用户手势。"
	issues := validateRow(0, row, source)
	found := false
	for _, iss := range issues {
		if iss.Kind == IssueLocatorMissing {
			found = true
		}
	}
	if !found {
		t.Errorf("expected locator-missing issue, got %v", issues)
	}
}

func TestValidateRow_LocatorAbsent_WarnsOnlyWhenSourceHasLocators(t *testing.T) {
	row := ChartRow{ElementID: "1a", TargetID: "D1", Quote: "包括传感器", PinCite: "[D1 命中片段]"}

	// 源文含段号：未定位段号应提示。
	withLocs := validateRow(0, row, "[0032] 所述装置包括传感器。")
	if len(withLocs) != 1 || withLocs[0].Kind != IssueLocatorAbsent {
		t.Errorf("expected locator-absent issue, got %v", withLocs)
	}

	// 源文本身无段号（如产品说明）：不应产生定位问题。
	noLocs := validateRow(0, row, "所述装置包括传感器，用于采集手势。")
	if len(noLocs) != 0 {
		t.Errorf("source without locators must not produce issues, got %v", noLocs)
	}
}

func TestValidateRow_Pass(t *testing.T) {
	row := ChartRow{
		ElementID: "1a",
		TargetID:  "D1",
		Quote:     "所述装置包括传感器",
		PinCite:   "[D1 [0032]]",
	}
	if issues := validateRow(0, row, "[0032] 所述装置包括传感器，用于采集手势。"); len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestValidateChart_SkipsMissingSources(t *testing.T) {
	chart := &ClaimChart{
		Rows: []ChartRow{
			{ElementID: "1a", TargetID: "D1", Quote: "虚构句子"},
			{ElementID: "1a", TargetID: "D2", Quote: "虚构句子"}, // D2 无源文
		},
	}
	issues := ValidateChart(chart, map[string]string{"D1": "源文内容"})
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (D1 only), got %v", issues)
	}
	if issues[0].TargetID != "D1" {
		t.Errorf("issue target = %q, want D1", issues[0].TargetID)
	}
}

// =============================================================================
// BuildClaimChart 集成：真实源文 + 段号升级 + Verified 标记
// =============================================================================

func TestBuildClaimChart_PinCiteIntegration(t *testing.T) {
	source := "[0032]本实施例的智能终端包括处理器、存储器，以及用于采集用户手势的传感器。[0033]所述处理器根据所述手势执行对应操作。\n"
	path := filepath.Join(t.TempDir(), "d1.txt")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	input := ChartInput{
		Mode: ModeInvalidity,
		ClaimText: `1. 一种智能终端，其特征在于，包括：
用于采集用户手势的传感器，所述处理器根据所述手势执行对应操作。`,
		Targets: []ChartTargetInput{
			{ID: "D1", Kind: "prior-art", SourcePath: path},
		},
	}

	chart, err := BuildClaimChart(input)
	if err != nil {
		t.Fatalf("BuildClaimChart failed: %v", err)
	}

	// 命中片段自带段号时，pin-cite 应升级为 [D1 [0032]] 形式。
	upgraded := 0
	verified := 0
	for _, row := range chart.Rows {
		if row.Quote == "" {
			continue
		}
		if strings.Contains(row.PinCite, "[0032]") || strings.Contains(row.PinCite, "[0033]") {
			upgraded++
		}
		if row.Verified {
			verified++
		}
	}
	if upgraded == 0 {
		t.Error("expected pin-cite upgraded with paragraph locators")
	}
	if verified == 0 {
		t.Error("expected rows with validated quotes to be marked verified")
	}
	if len(chart.PinCiteIssues) != 0 {
		t.Errorf("clean source should produce no issues, got %v", chart.PinCiteIssues)
	}
}

func TestBuildClaimChart_TruncatedQuoteStaysVerbatim(t *testing.T) {
	// 超长句子：截断后的引文（无省略号）必须仍是源文逐字子串，
	// 因此不应产生 quote-mismatch 问题。
	longSentence := "[0040]" + strings.Repeat("所述模块执行对应的数据处理与校验操作，", 40)
	source := longSentence + "\n[0041]其他内容。"
	path := filepath.Join(t.TempDir(), "d1.txt")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	input := ChartInput{
		Mode:      ModeInvalidity,
		ClaimText: "1. 一种装置，其特征在于，包括：数据处理与校验模块。",
		Targets: []ChartTargetInput{
			{ID: "D1", Kind: "prior-art", SourcePath: path},
		},
	}
	chart, err := BuildClaimChart(input)
	if err != nil {
		t.Fatalf("BuildClaimChart failed: %v", err)
	}
	for _, iss := range chart.PinCiteIssues {
		if iss.Kind == IssueQuoteMismatch {
			t.Errorf("verbatim-prefix truncation must not trigger quote-mismatch: %v", iss)
		}
	}
	for _, row := range chart.Rows {
		if row.Quote != "" && len([]rune(row.Quote)) > maxQuoteRunes {
			t.Errorf("quote length %d exceeds cap", len([]rune(row.Quote)))
		}
	}
}
