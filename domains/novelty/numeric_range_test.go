package novelty

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func inputFor(claims, snippet string) *NoveltyInput {
	return &NoveltyInput{
		Claims: []ClaimText{{ID: "1", Text: claims, Type: "independent"}},
		PriorArtDocs: []PriorArtDoc{{
			DocID:   "D1",
			Title:   "对比文件1",
			Snippet: snippet,
		}},
		EvidenceCoverage: "full",
	}
}

func TestAnalyzeNumericRanges_OverlappedRange(t *testing.T) {
	// 情形二：权利要求 5-10wt% 与对比文件 8-12wt% 重叠 → 破坏新颖性。
	input := inputFor(
		"一种组合物，包含 5-10wt% 的分散剂和 20% 的填料。",
		"该组合物包含 8-12wt% 的分散剂。",
	)
	a := AnalyzeNumericRanges(input)
	if a.Verdict != NumericOverlapped {
		t.Fatalf("verdict = %s, want overlapped; summary=%s", a.Verdict, a.Summary)
	}
	if len(a.Overlaps) == 0 {
		t.Fatal("expected overlaps")
	}
	if a.Overlaps[0].Kind != NumericOverlapped {
		t.Errorf("overlap kind = %s", a.Overlaps[0].Kind)
	}
}

func TestAnalyzeNumericRanges_PointInsideNoEndpoint(t *testing.T) {
	// 情形五：权利要求 95mm 落在对比文件 70-105mm 内且无共同端点 → 不破坏。
	input := inputFor(
		"一种传动轴，长度为 95mm。",
		"一种传动轴，长度为 70-105mm。",
	)
	a := AnalyzeNumericRanges(input)
	if a.Verdict != NumericInsideWithoutEndpoint {
		t.Fatalf("verdict = %s, want inside_without_endpoint; summary=%s", a.Verdict, a.Summary)
	}
	if a.Overlaps[0].Kind != NumericInsideWithoutEndpoint {
		t.Errorf("overlap kind = %s", a.Overlaps[0].Kind)
	}
}

func TestAnalyzeNumericRanges_UnitMismatch(t *testing.T) {
	// 单位不同不可比：5-10小时 与 8-12mm 不构成重叠。
	input := inputFor(
		"反应时间为 5-10小时。",
		"结构长度为 8-12mm。",
	)
	a := AnalyzeNumericRanges(input)
	if a.Verdict != NumericNoOverlap {
		t.Fatalf("verdict = %s, want no_overlap; summary=%s", a.Verdict, a.Summary)
	}
}

func TestAnalyzeNumericRanges_BoundedExpressions(t *testing.T) {
	// 至少/不超过单边表述按闭区间近似（保守方向）。
	input := inputFor(
		"涂层的厚度至少为 5μm。",
		"涂层的厚度为 8μm。",
	)
	a := AnalyzeNumericRanges(input)
	if a.Verdict != NumericOverlapped {
		t.Fatalf("verdict = %s, want overlapped (conservative); summary=%s", a.Verdict, a.Summary)
	}
}

func TestAnalyzeNumericRanges_Inconclusive(t *testing.T) {
	// 无带单位数值 → inconclusive。
	input := inputFor(
		"一种装置，包括处理器和存储器。",
		"一种装置，包括处理单元。",
	)
	a := AnalyzeNumericRanges(input)
	if a.Verdict != NumericInconclusive {
		t.Fatalf("verdict = %s, want inconclusive", a.Verdict)
	}
	if a.LLMAgreement != LLMNA {
		t.Errorf("inconclusive must keep llm agreement n_a, got %s", a.LLMAgreement)
	}
}

func TestCrossCheckLLM_Disagree(t *testing.T) {
	// 语义轨说 no_overlap，确定性轨发现重叠 → disagree。
	input := inputFor(
		"包含 5-10wt% 的分散剂。",
		"包含 8-12wt% 的分散剂。",
	)
	a := AnalyzeNumericRanges(input)
	a.CrossCheckLLM(`{"numeric_range_result":"no_overlap"}`)
	if a.LLMAgreement != LLMDisagree {
		t.Fatalf("llm agreement = %s, want disagree; summary=%s", a.LLMAgreement, a.Summary)
	}
	if !strings.Contains(a.Summary, "不一致") {
		t.Errorf("summary should flag disagreement: %s", a.Summary)
	}
}

func TestCrossCheckLLM_AgreeAndEmpty(t *testing.T) {
	input := inputFor(
		"包含 5-10wt% 的分散剂。",
		"包含 8-12wt% 的分散剂。",
	)
	a := AnalyzeNumericRanges(input)
	a.CrossCheckLLM(`{"numeric_range_result":"overlapped"}`)
	if a.LLMAgreement != LLMAgree {
		t.Errorf("llm agreement = %s, want agree", a.LLMAgreement)
	}

	// 空 JSON / 坏 JSON：无法对照，保持 n_a，不 panic（fresh analysis）。
	fresh := AnalyzeNumericRanges(inputFor(
		"包含 5-10wt% 的分散剂。",
		"包含 8-12wt% 的分散剂。",
	))
	fresh.CrossCheckLLM("")
	if fresh.LLMAgreement != LLMNA {
		t.Errorf("empty compare should keep n_a, got %s", fresh.LLMAgreement)
	}
	fresh.CrossCheckLLM(`{broken`)
	if fresh.LLMAgreement != LLMNA {
		t.Errorf("broken compare should keep n_a, got %s", fresh.LLMAgreement)
	}
}

func TestNumericRangeNode_WritesState(t *testing.T) {
	state := graph.PregelState{
		StateKeyNoveltyInput: inputFor(
			"一种组合物，包含 5-10wt% 的分散剂。",
			"该组合物包含 8-12wt% 的分散剂。",
		),
		stateKeyCompare: `{"numeric_range_result":"overlapped"}`,
	}
	node := numericRangeNode()
	if _, err := node(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	a, ok := state[StateKeyNumericRange].(*NumericRangeAnalysis)
	if !ok || a == nil {
		t.Fatal("expected numeric range analysis in state")
	}
	if a.Verdict != NumericOverlapped || a.LLMAgreement != LLMAgree {
		t.Errorf("verdict=%s agreement=%s, want overlapped/agree", a.Verdict, a.LLMAgreement)
	}
}

func TestExtractFindings_WeakFindingsNotCounted(t *testing.T) {
	// "权利要求1-3" 这类编号是无单位的弱发现，不参与判定。
	fs := extractFindings("参见权利要求1-3。", "2", "")
	for _, f := range fs {
		if f.Strong {
			t.Errorf("unitless expression must be weak: %+v", f)
		}
	}
}
