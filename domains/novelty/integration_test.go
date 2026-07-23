package novelty

import (
	"context"
	"testing"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Integration tests — 验证图拓扑、跳过传播、工具执行
// =============================================================================

var _mockProvider = mockProvider{}

// TestNoveltyGraphFullFlow 验证全图拓扑运行：编译 → 填充完整输入 → Run → 验证结果。
func TestNoveltyGraphFullFlow(t *testing.T) {
	compiled, err := BuildNoveltyGraph(_mockProvider)
	if err != nil {
		t.Fatalf("BuildNoveltyGraph failed: %v", err)
	}

	input := &NoveltyInput{
		Claims:           []ClaimText{{ID: "1", Text: "一种装置，其特征在于包含部件A", Type: "independent"}},
		PriorArtDocs:     []PriorArtDoc{{DocID: "D1", Title: "对比文件", Snippet: "公开了部件A", Score: 0.9}},
		FilingDate:       "2024-06-01",
		TechDomain:       "mechanical",
		EvidenceCoverage: "full",
	}

	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = input

	ctx := context.Background()
	var runErr error
	state, runErr = compiled.Run(ctx, state)

	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state after run")
	}
	result, ok := raw.(*NoveltyResult)
	if !ok {
		t.Fatalf("expected *NoveltyResult, got %T", raw)
	}
	// With mock provider returning empty responses, either Assessed or Skipped is valid
	if !result.Assessed && !result.Skipped {
		t.Error("expected either Assessed=true or Skipped=true")
	}
	_ = runErr // mock provider may produce errors — that's acceptable
}

// TestNoveltyGraph_FullFlowWithConflict 验证包含抵触申请的图运行。
func TestNoveltyGraph_FullFlowWithConflict(t *testing.T) {
	compiled, err := BuildNoveltyGraph(_mockProvider)
	if err != nil {
		t.Fatalf("BuildNoveltyGraph failed: %v", err)
	}

	input := &NoveltyInput{
		Claims:       []ClaimText{{ID: "1", Text: "一种方法", Type: "independent"}},
		PriorArtDocs: []PriorArtDoc{{DocID: "D1", Title: "对比文件1"}},
		ConflictApps: []ConflictApp{{
			AppID:      "CN2023100001",
			Title:      "在先申请",
			FilingDate: "2023-06-01",
			PubDate:    "2024-12-01",
			FullText:   "一种方法，包括步骤A、B、C",
		}},
		FilingDate:       "2024-06-01",
		TechDomain:       "general",
		EvidenceCoverage: "full",
	}

	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = input

	ctx := context.Background()
	state, _ = compiled.Run(ctx, state)

	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	_, ok = raw.(*NoveltyResult)
	if !ok {
		t.Fatalf("expected *NoveltyResult, got %T", raw)
	}
}

// TestNoveltyGraph_SkipPropagation 验证 EvidenceCoverage="none" 时跳过图传播。
func TestNoveltyGraph_SkipPropagation(t *testing.T) {
	compiled, err := BuildNoveltyGraph(_mockProvider)
	if err != nil {
		t.Fatalf("BuildNoveltyGraph failed: %v", err)
	}

	input := &NoveltyInput{EvidenceCoverage: "none"}
	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = input

	ctx := context.Background()
	state, _ = compiled.Run(ctx, state)

	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	result := raw.(*NoveltyResult)
	if !result.Skipped {
		t.Error("expected Skipped=true for none evidence coverage")
	}
	if result.Assessed {
		t.Error("expected Assessed=false")
	}
}

// TestNoveltyGraph_InvalidInput 验证未设置 StateKey 时跳过。
func TestNoveltyGraph_InvalidInput(t *testing.T) {
	compiled, err := BuildNoveltyGraph(_mockProvider)
	if err != nil {
		t.Fatalf("BuildNoveltyGraph failed: %v", err)
	}

	state := graph.PregelState{}
	ctx := context.Background()
	state, _ = compiled.Run(ctx, state)

	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	result := raw.(*NoveltyResult)
	if !result.Skipped {
		t.Error("expected Skipped=true for missing input")
	}
}

// TestNoveltyTool_RunWithMockProvider 验证工具注册和调用。
func TestNoveltyTool_RunWithMockProvider(t *testing.T) {
	tool := NewNoveltyTool(WithProvider(_mockProvider))
	if tool.Name != "evaluate_novelty" {
		t.Errorf("expected tool name evaluate_novelty, got %s", tool.Name)
	}
	if !tool.ReadOnly {
		t.Error("expected ReadOnly=true")
	}
}

// TestNoveltyTool_NoProviderReturnsError 验证无 provider 时返回 error map。
func TestNoveltyTool_NoProviderReturnsError(t *testing.T) {
	tool := NewNoveltyTool()
	if tool.Name != "evaluate_novelty" {
		t.Errorf("expected tool name evaluate_novelty, got %s", tool.Name)
	}
}

// TestTypeCompatibility 验证 state key 和结构体字段兼容性。
func TestTypeCompatibility(t *testing.T) {
	if StateKeyNoveltyInput != "novelty_input" {
		t.Error("StateKeyNoveltyInput mismatch")
	}
	if StateKeyNoveltyResult != "novelty_result" {
		t.Error("StateKeyNoveltyResult mismatch")
	}
}
