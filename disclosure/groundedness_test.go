package disclosure

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestGroundednessFilterNode_NoExtraction(t *testing.T) {
	// 空 state（无提取结果）-> 跳过，不崩溃
	node := groundednessFilterNode(nil)
	state := graph.PregelState{}

	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gr, ok := result[StateKeyGroundedness].(*GroundednessResult)
	if !ok || !gr.Skipped {
		t.Error("expected GroundednessResult with Skipped=true")
	}
}

func TestGroundednessFilterNode_WithExtraction(t *testing.T) {
	// 有提取结果但无 LLM（provider=nil）
	// provider=nil 时 agentcore.New(cfg) 会用默认 provider（无 LLM），
	// agent.Run 会尝试创建连接失败，但我们期望 fail-open 行为（设置 Skipped 而非崩溃）
	ext := &ExtractionResult{
		Features: []TechFeature{
			{ID: "f1", Description: "cooling system", Category: CatStructure},
		},
	}
	state := graph.PregelState{
		StateKeyExtraction: ext,
		StateKeyDoc: &DisclosureDoc{
			Sections: map[DocSection]string{
				SecSolution: "The cooling system comprises a heat sink...",
			},
		},
	}

	node := groundednessFilterNode(nil)
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该 fail-open（Skipped 或无法连 LLM 但没崩溃）
	gr, ok := result[StateKeyGroundedness].(*GroundednessResult)
	if !ok {
		t.Fatal("expected GroundednessResult in state")
	}
	if !gr.Skipped {
		// 若 LLM 意外可用，跳过此断言
		t.Log("groundedness filter did not skip (LLM available?)")
	}
}

func TestBuildGroundednessInput(t *testing.T) {
	ext := &ExtractionResult{
		Features: []TechFeature{
			{ID: "f1", Description: "heat sink", Category: CatStructure},
			{ID: "f2", Description: "fan", Category: CatStructure, Function: "cooling"},
		},
	}
	state := graph.PregelState{
		StateKeyDoc: &DisclosureDoc{
			Sections: map[DocSection]string{
				SecProblem:     "overheating issue",
				SecSolution:    "heat sink and fan",
				SecEmbodiments: "detailed implementation",
			},
		},
		StateKeyExtraction: ext,
	}

	input := buildGroundednessInput(state, ext)
	if !strings.Contains(input, "f1") || !strings.Contains(input, "heat sink") {
		t.Error("expected feature f1 in input")
	}
	if !strings.Contains(input, "f2") || !strings.Contains(input, "cooling") {
		t.Error("expected feature f2 with function in input")
	}
	if !strings.Contains(input, "overheating issue") {
		t.Error("expected document content in input")
	}
	if !strings.Contains(input, "detailed implementation") {
		t.Error("expected embodiments content in input")
	}
}

func TestParseGroundednessOutput_Valid(t *testing.T) {
	output := `{"assessments":[{"feature_id":"f1","score":0.9,"reasoning":"clearly in text"},{"feature_id":"f2","score":0.3,"reasoning":"not found"}],"overall_note":"done"}`
	ext := &ExtractionResult{
		Features: []TechFeature{
			{ID: "f1"}, {ID: "f2"},
		},
	}

	result := parseGroundednessOutput(output, ext)
	if result.Skipped {
		t.Error("expected not skipped")
	}
	if len(result.Scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(result.Scores))
	}
	if result.Scores["f1"] != 0.9 {
		t.Errorf("expected f1=0.9, got %f", result.Scores["f1"])
	}
	if result.Scores["f2"] != 0.3 {
		t.Errorf("expected f2=0.3, got %f", result.Scores["f2"])
	}
	if result.LowCount != 1 {
		t.Errorf("expected lowCount=1 (f2<0.6), got %d", result.LowCount)
	}
}

func TestParseGroundednessOutput_InvalidJSON(t *testing.T) {
	ext := &ExtractionResult{
		Features: []TechFeature{
			{ID: "f1"}, {ID: "f2"},
		},
	}

	result := parseGroundednessOutput("not json", ext)
	// Invalid JSON 应该 fallback 到默认值
	if len(result.Scores) != 2 {
		t.Fatalf("expected 2 fallback scores, got %d", len(result.Scores))
	}
	if result.Scores["f1"] != 0.3 {
		t.Errorf("expected f1=0.3 fallback, got %f", result.Scores["f1"])
	}
}

func TestParseGroundednessOutput_Partial(t *testing.T) {
	// LLM 只返回了部分特征，缺失的应该补默认值 0.3
	output := `{"assessments":[{"feature_id":"f1","score":0.9,"reasoning":"ok"}]}`
	ext := &ExtractionResult{
		Features: []TechFeature{
			{ID: "f1"}, {ID: "f2"}, {ID: "f3"},
		},
	}

	result := parseGroundednessOutput(output, ext)
	if result.Scores["f1"] != 0.9 {
		t.Errorf("expected f1=0.9, got %f", result.Scores["f1"])
	}
	if result.Scores["f2"] != 0.3 {
		t.Errorf("expected f2=0.3 default, got %f", result.Scores["f2"])
	}
	if result.Scores["f3"] != 0.3 {
		t.Errorf("expected f3=0.3 default, got %f", result.Scores["f3"])
	}
}

func TestParseGroundednessOutput_ScoreClamping(t *testing.T) {
	output := `{"assessments":[{"feature_id":"f1","score":1.5,"reasoning":"above max"},{"feature_id":"f2","score":-0.5,"reasoning":"below min"}]}`
	ext := &ExtractionResult{
		Features: []TechFeature{
			{ID: "f1"}, {ID: "f2"},
		},
	}

	result := parseGroundednessOutput(output, ext)
	if result.Scores["f1"] != 1.0 {
		t.Errorf("expected f1 clamped to 1.0, got %f", result.Scores["f1"])
	}
	if result.Scores["f2"] != 0.0 {
		t.Errorf("expected f2 clamped to 0.0, got %f", result.Scores["f2"])
	}
}

func TestGroundednessGraphIntegration(t *testing.T) {
	// 验证 groundedness_filter 节点在 Pregel 图中的注册和路由正确。
	// 使用 mock provider 验证节点能正常执行完整流程。
	provider := newTestProvider()
	cpg, err := BuildDisclosureAnalysisGraph(provider)
	if err != nil {
		t.Fatalf("BuildDisclosureAnalysisGraph failed: %v", err)
	}

	// 观察编译后的图是否包含了 groundedness_filter 节点
	initial := graph.PregelState{StateKeyInput: sampleDisclosure}
	_, err = cpg.Run(context.Background(), initial)
	// review_gate 会返回 InterruptError — 这是预期行为
	if err == nil {
		t.Fatal("expected interrupt at review_gate")
	}
	if !strings.Contains(err.Error(), "interrupt") {
		t.Fatalf("expected interrupt error, got: %v", err)
	}
}
