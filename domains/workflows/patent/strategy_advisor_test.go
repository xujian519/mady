package patent

import (
	"context"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// AllInvalidationStats — data integrity
// -----------------------------------------------------------------------------

func TestAllInvalidationStats_ReturnsAll(t *testing.T) {
	stats := AllInvalidationStats()
	if len(stats) != len(invalidationStats) {
		t.Errorf("got %d stats, want %d", len(stats), len(invalidationStats))
	}

	// Verify each stat has required fields populated.
	for _, s := range stats {
		if s.TotalCases <= 0 {
			t.Errorf("stat %s: TotalCases = %d, want > 0", s.GroundType, s.TotalCases)
		}
		if s.InvalidationRate <= 0 || s.InvalidationRate > 1.0 {
			t.Errorf("stat %s: InvalidationRate = %f, want (0, 1.0]", s.GroundType, s.InvalidationRate)
		}
		if s.Description == "" {
			t.Errorf("stat %s: description is empty", s.GroundType)
		}
		if s.Source == "" {
			t.Errorf("stat %s: source is empty", s.GroundType)
		}
	}
}

func TestAllInvalidationStats_CoversAllGroundTypes(t *testing.T) {
	stats := AllInvalidationStats()
	seen := make(map[string]bool)
	for _, s := range stats {
		seen[s.GroundType] = true
	}

	for _, gt := range []InvalidationGroundType{
		GroundNovelty, GroundInventiveness, GroundDisclosure,
		GroundClaimClarity, GroundAmendment,
	} {
		if !seen[string(gt)] {
			t.Errorf("missing stat for ground type %s", gt)
		}
	}
}

// -----------------------------------------------------------------------------
// AllReasoningPatternStats — data integrity
// -----------------------------------------------------------------------------

func TestAllReasoningPatternStats_ReturnsAll(t *testing.T) {
	stats := AllReasoningPatternStats()
	if len(stats) != len(reasoningPatternStats) {
		t.Errorf("got %d pattern stats, want %d", len(stats), len(reasoningPatternStats))
	}

	for _, s := range stats {
		if s.TotalCases <= 0 {
			t.Errorf("pattern %s: TotalCases = %d, want > 0", s.PatternID, s.TotalCases)
		}
		if s.InvalidationRate <= 0 || s.InvalidationRate > 1.0 {
			t.Errorf("pattern %s: InvalidationRate = %f, want (0, 1.0]", s.PatternID, s.InvalidationRate)
		}
		if s.Frequency <= 0 || s.Frequency > 1.0 {
			t.Errorf("pattern %s: Frequency = %f, want (0, 1.0]", s.PatternID, s.Frequency)
		}
	}
}

// -----------------------------------------------------------------------------
// GetStrategyAdvice — known ground types
// -----------------------------------------------------------------------------

func TestGetStrategyAdvice_Novelty(t *testing.T) {
	advice := GetStrategyAdvice(string(GroundNovelty), "")
	if advice.GroundType != string(GroundNovelty) {
		t.Errorf("GroundType = %q, want %q", advice.GroundType, GroundNovelty)
	}
	if advice.Stat == "" {
		t.Error("Stat should not be empty")
	}
	if advice.Probability == "" {
		t.Error("Probability should not be empty")
	}
	if advice.Recommendation == "" {
		t.Error("Recommendation should not be empty")
	}
	if advice.Source == "" {
		t.Error("Source should not be empty")
	}
	if !strings.Contains(advice.Stat, "8410") {
		t.Errorf("novelty stat should mention 8410 cases, got: %s", advice.Stat)
	}
	if strings.Contains(advice.Probability, "绝对") || strings.Contains(advice.Probability, "一定") {
		t.Errorf("probability should not use absolute phrasing: %s", advice.Probability)
	}
}

func TestGetStrategyAdvice_Inventiveness(t *testing.T) {
	advice := GetStrategyAdvice(string(GroundInventiveness), "")
	if advice.Stat == "" {
		t.Error("Stat should not be empty")
	}
	if !strings.Contains(advice.Stat, "12798") {
		t.Errorf("inventiveness stat should mention 12798 cases, got: %s", advice.Stat)
	}
	if !strings.Contains(advice.Probability, "通常") && !strings.Contains(advice.Probability, "大概率") {
		t.Errorf("probability should use non-absolute phrasing: %s", advice.Probability)
	}
}

func TestGetStrategyAdvice_AllGroundTypes(t *testing.T) {
	for _, gt := range []InvalidationGroundType{
		GroundNovelty, GroundInventiveness, GroundDisclosure,
		GroundClaimClarity, GroundAmendment,
	} {
		advice := GetStrategyAdvice(string(gt), "")
		if advice.Stat == "" {
			t.Errorf("%s: stat is empty", gt)
		}
		if advice.Probability == "" {
			t.Errorf("%s: probability is empty", gt)
		}
		if advice.Recommendation == "" {
			t.Errorf("%s: recommendation is empty", gt)
		}
		if advice.Source == "" {
			t.Errorf("%s: source is empty", gt)
		}
		// Verify non-absolute phrasing.
		if strings.Contains(advice.Probability, "绝对") || strings.Contains(advice.Probability, "一定") || strings.Contains(advice.Probability, "百分百") {
			t.Errorf("%s: uses absolute phrasing: %s", gt, advice.Probability)
		}
	}
}

func TestGetStrategyAdvice_UnknownGroundType(t *testing.T) {
	advice := GetStrategyAdvice("A99.99_unknown", "")
	if advice.Stat != "暂无该无效理由类型的统计数据" {
		t.Errorf("expected fallback stat, got: %s", advice.Stat)
	}
	if advice.Probability != "无法评估" {
		t.Errorf("expected fallback probability, got: %s", advice.Probability)
	}
}

func TestGetStrategyAdvice_WithPattern(t *testing.T) {
	advice := GetStrategyAdvice(string(GroundInventiveness), "single-doc-common-knowledge")
	if !strings.Contains(advice.Stat, "单对比文件+公知常识") {
		t.Errorf("stat should include pattern description, got: %s", advice.Stat)
	}
	if !strings.Contains(advice.Stat, "54.0%") {
		t.Errorf("stat should include pattern frequency, got: %s", advice.Stat)
	}
	if !strings.Contains(advice.Source, "5,805") {
		t.Errorf("source should include pattern source, got: %s", advice.Source)
	}
}

// -----------------------------------------------------------------------------
// FormatStrategySection — output formatting
// -----------------------------------------------------------------------------

func TestFormatStrategySection_EmptyGrounds(t *testing.T) {
	result := FormatStrategySection(nil)
	if result != "" {
		t.Errorf("expected empty string for nil grounds, got %q", result)
	}

	result = FormatStrategySection([]InvGround{})
	if result != "" {
		t.Errorf("expected empty string for empty grounds, got %q", result)
	}
}

func TestFormatStrategySection_NoveltyOnly(t *testing.T) {
	grounds := []InvGround{
		{
			Type:        GroundNovelty,
			Article:     "专利法第22条第2款",
			Description: "新颖性无效",
		},
	}

	result := FormatStrategySection(grounds)
	if result == "" {
		t.Fatal("result should not be empty")
	}

	// Verify section structure.
	if !strings.Contains(result, "统计数据策略建议") {
		t.Error("should contain section title")
	}
	if !strings.Contains(result, "说明") {
		t.Error("should contain explanatory note")
	}
	if !strings.Contains(result, "新颖性") {
		t.Error("should mention novelty")
	}
	if !strings.Contains(result, "8410") {
		t.Error("should mention 8410 case count")
	}
	if !strings.Contains(result, "来源") {
		t.Error("should contain source attribution")
	}

	// Should NOT contain reasoning pattern table (no inventiveness ground).
	if strings.Contains(result, "推理模式") {
		t.Error("should not contain reasoning pattern section for non-inventiveness grounds")
	}
}

func TestFormatStrategySection_AllGrounds(t *testing.T) {
	grounds := []InvGround{
		{Type: GroundNovelty, Article: "专利法第22条第2款", Description: "新颖性无效"},
		{Type: GroundInventiveness, Article: "专利法第22条第3款", Description: "创造性无效"},
		{Type: GroundDisclosure, Article: "专利法第26条第3款", Description: "公开不充分无效"},
		{Type: GroundClaimClarity, Article: "专利法第26条第4款", Description: "权利要求不清楚"},
		{Type: GroundAmendment, Article: "专利法第33条", Description: "修改超范围"},
	}

	result := FormatStrategySection(grounds)
	if result == "" {
		t.Fatal("result should not be empty")
	}

	// Verify each ground type is covered.
	for _, label := range []string{"新颖性", "创造性", "公开不充分", "不清楚", "修改超范围"} {
		if !strings.Contains(result, label) {
			t.Errorf("should mention %s", label)
		}
	}

	// Verify reasoning pattern table is present (inventiveness ground exists).
	if !strings.Contains(result, "推理模式") {
		t.Error("should contain reasoning pattern section when inventiveness ground present")
	}
	if !strings.Contains(result, "单对比文件+公知常识") {
		t.Error("should list single-doc-common-knowledge pattern")
	}
	if !strings.Contains(result, "多对比文件结合") {
		t.Error("should list multi-doc-combination pattern")
	}
	if !strings.Contains(result, "技术启示判断") {
		t.Error("should list technical-motivation pattern")
	}
}

func TestFormatStrategySection_NonAbsolutePhrasing(t *testing.T) {
	grounds := []InvGround{
		{Type: GroundNovelty, Article: "专利法第22条第2款", Description: "新颖性"},
	}

	result := FormatStrategySection(grounds)
	// Check no absolute phrasing in probability assessments.
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "- 概率：") {
			if strings.Contains(line, "绝对") || strings.Contains(line, "一定") || strings.Contains(line, "百分百") {
				t.Errorf("probability line uses absolute phrasing: %s", line)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// GetDesignStat
// -----------------------------------------------------------------------------

func TestGetDesignStat(t *testing.T) {
	stat := GetDesignStat()
	if stat == "" {
		t.Fatal("design stat should not be empty")
	}
	if !strings.Contains(stat, "14028") {
		t.Errorf("should mention 14028 cases, got: %s", stat)
	}
	if !strings.Contains(stat, "整体视觉效果") {
		t.Errorf("should mention 整体视觉效果, got: %s", stat)
	}
}

// -----------------------------------------------------------------------------
// End-to-end integration: strategy section in invConcludeNode output
// -----------------------------------------------------------------------------

func TestInvConcludeNode_IncludesStrategySection(t *testing.T) {
	grounds := []InvGround{
		{
			Type:        GroundInventiveness,
			Article:     "专利法第22条第3款",
			Description: "创造性无效",
			ClaimRefs:   []int{1},
		},
		{
			Type:        GroundNovelty,
			Article:     "专利法第22条第2款",
			Description: "新颖性无效",
			ClaimRefs:   []int{1},
		},
	}

	state := buildMinimalStateForConclude(grounds)
	out, err := invConcludeNode(context.Background(), state)
	if err != nil {
		t.Fatalf("invConcludeNode: %v", err)
	}

	output := out.GetString(InvStateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}

	// Strategy section should appear before the disclaimer.
	if !strings.Contains(output, "统计数据策略建议") {
		t.Error("output should contain strategy section title")
	}

	// Strategy section should be before the disclaimer.
	disclaimerPos := strings.Index(output, "---")
	strategyPos := strings.Index(output, "统计数据策略建议")
	if strategyPos < 0 || disclaimerPos < 0 {
		t.Fatal("could not find strategy section or disclaimer markers")
	}
	if strategyPos > disclaimerPos {
		t.Error("strategy section should appear before the disclaimer")
	}

	// Should include inventiveness pattern table (since one ground is inventiveness).
	if !strings.Contains(output, "单对比文件+公知常识") {
		t.Error("output should contain reasoning pattern reference")
	}

	// Non-absolute phrasing check.
	if strings.Contains(output, "绝对成功") || strings.Contains(output, "一定无效") {
		t.Error("output should not contain absolute phrasing")
	}
}

// buildMinimalStateForConclude creates a minimal PregelState with pre-populated
// analysis and rule check fields so invConcludeNode can run without needing
// the full graph pipeline.
func buildMinimalStateForConclude(grounds []InvGround) map[string]interface{} {
	var analysis strings.Builder
	analysis.WriteString("## 无效理由逐项分析\n\n")
	for _, g := range grounds {
		analysis.WriteString("### ")
		analysis.WriteString(g.Description)
		analysis.WriteString("\n\n分析内容摘要。\n\n")
	}

	ruleCheck := "## 规则引擎检查\n\n检查结论: ✅ 通过\n\n所有规则检查均通过。\n"

	return map[string]interface{}{
		InvStateAnalysis:    analysis.String(),
		InvStateRuleCheck:   ruleCheck,
		InvStateRuleVerdict: string(VerdictPass),
		InvStateGrounds:     grounds,
		InvStateInput:       "1. 一种方法。",
		InvStateClaims:      "1. 一种方法。",
		InvStateClaimTree: []InvClaimNode{
			{Number: 1, IsIndependent: true, Text: "1. 一种方法。"},
		},
	}
}
