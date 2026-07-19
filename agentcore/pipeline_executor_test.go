package agentcore

import (
	"context"
	"testing"
)

// mockProvider is a minimal Provider implementation for pipeline tests.
type mockProvider struct{}

func (m mockProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	return &ProviderResponse{Content: `{"conclusion": "mock conclusion"}`, Usage: TokenUsage{}}, nil
}

func (m mockProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta)
	close(ch)
	return ch, nil
}

func TestPipelineExecutor_RequiresManifest(t *testing.T) {
	e := NewPipelineExecutor(mockProvider{})
	_, err := e.Run(context.Background(), nil, PipelineState{})
	if err == nil {
		t.Fatal("expected error for nil manifest")
	}
}

func TestPipelineExecutor_EmptyPipeline(t *testing.T) {
	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:     "test",
		Domain:   "patent",
		Pipeline: PluginPipeline{Stages: []PluginStage{}},
	}
	state, err := e.Run(context.Background(), manifest, PipelineState{"input": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.GetString("input") != "hello" {
		t.Errorf("input should be preserved, got %q", state.GetString("input"))
	}
}

func TestPipelineExecutor_UnknownAtomDefaultSkip(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "test",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "s1", Atom: "nonexistent-atom", Description: "should be skipped"},
			},
		},
	}
	state, err := e.Run(context.Background(), manifest, PipelineState{"input": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	warnings, ok := state["_warnings"].([]string)
	if !ok || len(warnings) == 0 {
		t.Fatal("expected warnings for unknown atom")
	}
}

func TestPipelineExecutor_UnknownAtomFailOnUnknown(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{}, WithFailOnUnknown(true))
	manifest := &PluginManifest{
		Name:   "test",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "s1", Atom: "nonexistent-atom", Description: "should fail"},
			},
		},
	}
	_, err := e.Run(context.Background(), manifest, PipelineState{})
	if err == nil {
		t.Fatal("expected error for unknown atom with FailOnUnknown=true")
	}
	stageErr, ok := err.(*StageError)
	if !ok {
		t.Fatalf("expected *StageError, got %T: %v", err, err)
	}
	if stageErr.StageID != "s1" {
		t.Errorf("expected StageID s1, got %s", stageErr.StageID)
	}
}

func TestPipelineExecutor_StateIsolation(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "test",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "s1", Atom: "approval-gate", Description: "gate for state isolation test"},
			},
		},
	}

	// Ensure input mutation does not affect caller's map.
	input := PipelineState{"input": "original", "review_context": "test gate"}
	_, err := e.Run(context.Background(), manifest, input)
	if err == nil {
		t.Fatal("expected InterruptStageError from approval-gate")
	}
	if !IsInterruptStage(err) {
		t.Fatalf("expected InterruptStageError, got %T: %v", err, err)
	}

	// Verify original input is intact (not modified by executor).
	if input.GetString("input") != "original" {
		t.Errorf("caller's input was mutated: got %q", input.GetString("input"))
	}
}

func TestPipelineExecutor_ApprovalGateInterrupt(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "novelty-analysis",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "approval", Atom: "approval-gate", Description: "approval"},
			},
		},
	}

	state, err := e.Run(context.Background(), manifest, PipelineState{
		"review_context": "审查分析结果",
		"plugin_name":    "novelty-analysis",
	})
	if err == nil {
		t.Fatal("expected InterruptStageError")
	}
	if !IsInterruptStage(err) {
		t.Fatalf("expected InterruptStageError, got %T: %v", err, err)
	}
	if state.GetString("_interrupted_at") != "approval" {
		t.Errorf("expected _interrupted_at=approval, got %q", state.GetString("_interrupted_at"))
	}
}

func TestPipelineExecutor_ToolStageSkipped(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "test",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "draft", Tool: "write_file", Description: "tool stage"},
			},
		},
	}
	state, err := e.Run(context.Background(), manifest, PipelineState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	warnings, ok := state["_warnings"].([]string)
	if !ok || len(warnings) == 0 {
		t.Fatal("expected warning for tool-based stage")
	}
}

func TestPipelineExecutor_ReasoningHandler(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "test-reasoning",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "conclude", Atom: "reasoning", Description: "reasoning stage"},
			},
		},
	}

	state, err := e.Run(context.Background(), manifest, PipelineState{
		"reasoning_input":  "分析这个技术方案的新颖性",
		"reasoning_prompt": "你是一名专利审查员",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.GetString("reasoning_output") == "" {
		t.Error("expected reasoning_output to be non-empty")
	}
	if state.GetString("conclusion") == "" {
		t.Error("expected conclusion to be non-empty")
	}
}

func TestPipelineExecutor_ExtractHandler(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "test-extract",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "extract", Atom: "extract", Description: "extract features"},
			},
		},
	}

	state, err := e.Run(context.Background(), manifest, PipelineState{
		"text":            "本发明提供一种基于深度学习的人脸识别方法",
		"extraction_type": "features",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.GetString("extraction_result") == "" {
		t.Error("expected extraction_result to be non-empty")
	}
}

func TestPipelineExecutor_MultiStagePipeline(t *testing.T) {
	RegisterBuiltinStageHandlers()

	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "novelty-analysis",
		Domain: "patent",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				// search is skipped because _retriever is not set
				{ID: "search", Atom: "search", Description: "search"},
				// extract features
				{ID: "extract", Atom: "extract", Description: "extract"},
				// reasoning for conclusion
				{ID: "conclude", Atom: "reasoning", Description: "conclude"},
				// approval gate
				{ID: "approval", Atom: "approval-gate", Description: "approval"},
			},
		},
	}

	state, err := e.Run(context.Background(), manifest, PipelineState{
		"text":            "一种基于深度学习的图像识别方法",
		"extraction_type": "features",
		"reasoning_input": "基于以上特征，评估新颖性",
		"review_context":  "请审阅新颖性分析结论",
	})
	if err == nil {
		t.Fatal("expected InterruptStageError at approval-gate")
	}
	if !IsInterruptStage(err) {
		t.Fatalf("expected InterruptStageError, got %T: %v", err, err)
	}
	if state.GetString("_interrupted_at") != "approval" {
		t.Errorf("expected _interrupted_at=approval, got %q", state.GetString("_interrupted_at"))
	}
	// Verify intermediate stage outputs exist.
	if state.GetString("extraction_result") == "" {
		t.Error("expected extraction_result from extract stage")
	}
	if state.GetString("reasoning_output") == "" {
		t.Error("expected reasoning_output from reasoning stage")
	}
	// Search handler should have produced an error because no retriever.
	if state.GetString("prior_art") != "" {
		t.Error("expected no prior_art (no retriever configured)")
	}
	// Check that search wasn't silently skipped — it should produce _error.
	executed := state["_executed_stages"].([]string)
	if len(executed) != 3 {
		t.Errorf("expected 3 executed stages, got %d: %v", len(executed), executed)
	}
}

func TestPipelineExecutor_ManifestMetadataInState(t *testing.T) {
	e := NewPipelineExecutor(mockProvider{})
	manifest := &PluginManifest{
		Name:   "my-plugin",
		Domain: "legal",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "dummy", Atom: "reasoning", Description: "metadata test"},
			},
		},
	}
	state, err := e.Run(context.Background(), manifest, PipelineState{
		"reasoning_input": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.GetString("plugin_name") != "my-plugin" {
		t.Errorf("expected plugin_name=my-plugin, got %q", state.GetString("plugin_name"))
	}
	if state.GetString("plugin_domain") != "legal" {
		t.Errorf("expected plugin_domain=legal, got %q", state.GetString("plugin_domain"))
	}
}

func TestLookupStageHandler(t *testing.T) {
	RegisterBuiltinStageHandlers()

	cases := []struct {
		name      string
		wantFound bool
	}{
		{"search", true},
		{"extract", true},
		{"compare", true},
		{"reasoning", true},
		{"approval-gate", true},
		{"nonexistent", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := LookupStageHandler(tc.name)
			if tc.wantFound && h == nil {
				t.Errorf("expected to find handler %q", tc.name)
			}
			if !tc.wantFound && h != nil {
				t.Errorf("expected not to find handler %q", tc.name)
			}
		})
	}
}

func TestPipelineStateGetString(t *testing.T) {
	var nilState PipelineState
	if nilState.GetString("x") != "" {
		t.Error("nil state should return empty string")
	}

	state := PipelineState{"a": "hello", "b": 42}
	if state.GetString("a") != "hello" {
		t.Errorf("expected hello, got %q", state.GetString("a"))
	}
	if state.GetString("b") != "" {
		t.Errorf("expected empty for non-string value, got %q", state.GetString("b"))
	}
	if state.GetString("nonexistent") != "" {
		t.Errorf("expected empty for missing key")
	}
}

func TestPipelineStateSetString(t *testing.T) {
	var nilState PipelineState
	nilState.SetString("k", "v") // should not panic

	state := PipelineState{}
	state.SetString("greeting", "你好")
	if state.GetString("greeting") != "你好" {
		t.Errorf("expected 你好, got %q", state.GetString("greeting"))
	}
}

func TestExtractJSONFromText(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`{"a": 1}`, `{"a": 1}`},
		{`text before {"a": 1} text after`, `{"a": 1}`},
		{`no json here`, ``},
		{`{`, ``},
	}
	for _, tc := range cases {
		got := extractJSONFromText(tc.input)
		if got != tc.want {
			t.Errorf("extractJSONFromText(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
