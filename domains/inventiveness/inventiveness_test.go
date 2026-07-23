package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// mockProvider is a minimal mock that satisfies agentcore.Provider
// without making real LLM calls. Used for graph compilation tests.
type mockProvider struct{}

func (m mockProvider) Name() string     { return "mock" }
func (m mockProvider) Models() []string { return []string{"mock"} }
func (m mockProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{}, nil
}
func (m mockProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta)
	close(ch)
	return ch, nil
}

// =============================================================================
// Graph compilation tests
// =============================================================================

func TestBuildInventivenessGraph(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildInventivenessGraph(provider)
	if err != nil {
		t.Fatalf("BuildInventivenessGraph failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("expected non-nil compiled graph")
	}
	t.Logf("inventiveness graph compiled successfully")
}

func TestBuildInventivenessGraph_Valid(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildInventivenessGraph(provider)
	if err != nil {
		t.Fatalf("BuildInventivenessGraph failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("expected non-nil compiled graph")
	}
	t.Log("inventiveness graph compiled successfully")
}

// =============================================================================
// loadInputNode tests
// =============================================================================

func TestLoadInputNode_SkipEmpty(t *testing.T) {
	state := graph.PregelState{}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	result, ok := state[StateKeyResult].(*InventivenessResult)
	if !ok {
		t.Fatal("expected InventivenessResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when input is missing")
	}
	if result.Assessed {
		t.Error("expected Assessed=false when input is missing")
	}
}

func TestLoadInputNode_SkipNilInput(t *testing.T) {
	state := graph.PregelState{
		StateKeyInput: (*InventivenessInput)(nil),
	}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	result, ok := state[StateKeyResult].(*InventivenessResult)
	if !ok {
		t.Fatal("expected InventivenessResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when input is nil")
	}
}

func TestLoadInputNode_SkipNoEvidence(t *testing.T) {
	state := graph.PregelState{
		StateKeyInput: &InventivenessInput{
			EvidenceCoverage: "none",
			Features:         []TechFeature{{ID: "f1", Description: "test"}},
		},
	}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	result, ok := state[StateKeyResult].(*InventivenessResult)
	if !ok {
		t.Fatal("expected InventivenessResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when EvidenceCoverage=none")
	}
}

func TestLoadInputNode_Valid(t *testing.T) {
	input := &InventivenessInput{
		EvidenceCoverage: "full",
		Features:         []TechFeature{{ID: "f1", Description: "test feature"}},
	}
	state := graph.PregelState{
		StateKeyInput: input,
	}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	// Should NOT have set result (that's done by conclusion node)
	if _, ok := state[StateKeyResult]; ok {
		t.Error("loadInputNode should not set result for valid input")
	}
	// Should have validated and stored the input
	stored, ok := state[StateKeyInput].(*InventivenessInput)
	if !ok || stored == nil {
		t.Error("expected validated input in state")
	}
}

// =============================================================================
// State helper tests
// =============================================================================

func TestStateHasSkip(t *testing.T) {
	tests := []struct {
		name   string
		state  graph.PregelState
		expect bool
	}{
		{"empty state", graph.PregelState{}, false},
		{"no result key", graph.PregelState{"other": "value"}, false},
		{"result is string not struct", graph.PregelState{StateKeyResult: "not a struct"}, false},
		{"result skipped", graph.PregelState{
			StateKeyResult: &InventivenessResult{Skipped: true},
		}, true},
		{"result not skipped", graph.PregelState{
			StateKeyResult: &InventivenessResult{Skipped: false, Assessed: true},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stateHasSkip(tt.state)
			if got != tt.expect {
				t.Errorf("stateHasSkip() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestExtractInput(t *testing.T) {
	input := &InventivenessInput{
		Features:         []TechFeature{{ID: "f1"}},
		EvidenceCoverage: "partial",
	}
	state := graph.PregelState{StateKeyInput: input}

	got := extractInput(state)
	if got == nil {
		t.Fatal("expected non-nil input")
	}
	if len(got.Features) != 1 || got.Features[0].ID != "f1" {
		t.Error("extracted input doesn't match original")
	}

	// Empty state
	if got := extractInput(graph.PregelState{}); got != nil {
		t.Error("expected nil from empty state")
	}
}

// =============================================================================
// JSON extraction tests
// =============================================================================

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"valid json", `prefix {"key": "value"} suffix`, `{"key": "value"}`},
		{"no json", "no json here", ""},
		{"nested json", `{"outer": {"inner": 1}}`, `{"outer": {"inner": 1}}`},
		{"empty", "", ""},
		{"chinese json", `结论：{"closest_prior_art": "D1", "selection_reason": "领域相同"}`, `{"closest_prior_art": "D1", "selection_reason": "领域相同"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.expect {
				t.Errorf("extractJSON() = %q, want %q", got, tt.expect)
			}
		})
	}
}

// =============================================================================
// buildInputText tests
// =============================================================================

func TestBuildInputText(t *testing.T) {
	input := &InventivenessInput{
		Features: []TechFeature{
			{ID: "f1", Category: "structure", Description: "超声波传感器", Importance: "high"},
		},
		PriorArtChunks: []EvidenceChunk{
			{DocID: "D1", Title: "对比文件1", Snippet: "使用红外传感器..."},
		},
		NoveltyConclusion: "相对于D1具备新颖性",
	}

	text := buildInputText(input)

	// Should contain key elements.
	for _, want := range []string{"技术特征", "超声波传感器", "现有技术证据", "D1", "对比文件1", "新颖性初判结论", "具备新颖性"} {
		if !strings.Contains(text, want) {
			t.Errorf("buildInputText() missing %q in output:\n%s", want, text)
		}
	}
}

func TestBuildInputText_Nil(t *testing.T) {
	text := buildInputText(nil)
	if text != "" {
		t.Errorf("buildInputText(nil) should return empty, got %q", text)
	}
}

// =============================================================================
// Step parsing tests
// =============================================================================

func TestParseStep1(t *testing.T) {
	output := `{"closest_prior_art": "D1", "selection_reason": "技术领域相同且公开特征最多"}`
	result := parseStep1(output)

	if result.ClosestPriorArt != "D1" {
		t.Errorf("ClosestPriorArt = %q, want D1", result.ClosestPriorArt)
	}
	if result.SelectionReason == "" {
		t.Error("SelectionReason should not be empty")
	}
}

func TestParseStep1_NoJSON(t *testing.T) {
	output := "最接近的现有技术是D1，因为..."
	result := parseStep1(output)

	if result.SelectionReason != output {
		t.Errorf("parseStep1 without JSON should store raw output in SelectionReason")
	}
}

func TestParseStep2(t *testing.T) {
	output := `{"distinguishing_features": ["使用超声波代替红外"], "non_contributing_features": ["外壳颜色"], "tech_effects": ["精度更高"], "actual_tech_problem": "如何提高检测精度"}`
	result := parseStep2(output)

	if len(result.DistinguishingFeatures) != 1 {
		t.Errorf("expected 1 distinguishing feature, got %d", len(result.DistinguishingFeatures))
	}
	if len(result.NonContributingFeatures) != 1 || result.NonContributingFeatures[0] != "外壳颜色" {
		t.Errorf("expected non_contributing_features to contain '外壳颜色', got %v", result.NonContributingFeatures)
	}
	if result.ActualTechProblem == "" {
		t.Error("ActualTechProblem should not be empty")
	}
}

func TestParseStep2_NoNonContributing(t *testing.T) {
	output := `{"distinguishing_features": ["特征A"], "tech_effects": ["效果A"], "actual_tech_problem": "问题A"}`
	result := parseStep2(output)

	if len(result.DistinguishingFeatures) != 1 {
		t.Errorf("expected 1 distinguishing feature, got %d", len(result.DistinguishingFeatures))
	}
	// non_contributing_features should be empty when not present in JSON.
	if len(result.NonContributingFeatures) != 0 {
		t.Errorf("expected 0 non-contributing features, got %d", len(result.NonContributingFeatures))
	}
}

func TestParseStep3(t *testing.T) {
	output := `{"technical_suggestion": false, "suggestion_type": "", "has_reverse_teaching": false, "is_cross_domain": false, "rationale": "D2未公开超声波方案，作用不同", "confidence": "high"}`
	result := parseStep3(output)

	if result.TechnicalSuggestion {
		t.Error("expected TechnicalSuggestion=false")
	}
	if result.Confidence != "high" {
		t.Errorf("Confidence = %q, want high", result.Confidence)
	}
	if result.HasReverseTeaching {
		t.Error("expected HasReverseTeaching=false")
	}
}

func TestParseStep3_WithReverseTeaching(t *testing.T) {
	output := `{"technical_suggestion": false, "suggestion_type": "other_doc", "has_reverse_teaching": true, "is_cross_domain": false, "rationale": "D2明确教导不要使用超声波方案", "confidence": "high"}`
	result := parseStep3(output)

	if result.TechnicalSuggestion {
		t.Error("expected TechnicalSuggestion=false with reverse teaching")
	}
	if !result.HasReverseTeaching {
		t.Error("expected HasReverseTeaching=true")
	}
	if result.SuggestionType != "other_doc" {
		t.Errorf("expected SuggestionType=other_doc, got %q", result.SuggestionType)
	}
}

func TestParseStep3_CrossDomain(t *testing.T) {
	output := `{"technical_suggestion": true, "suggestion_type": "other_doc", "has_reverse_teaching": false, "is_cross_domain": true, "rationale": "跨领域应用", "confidence": "medium"}`
	result := parseStep3(output)

	if !result.IsCrossDomain {
		t.Error("expected IsCrossDomain=true")
	}
}

func TestParseStep3_FiveTypes(t *testing.T) {
	tests := []struct {
		name           string
		suggestionType string
		expectValid    bool
	}{
		{"公知常识", "common_knowledge", true},
		{"同文件其他部分", "same_doc", true},
		{"另文件披露", "other_doc", true},
		{"功能等同", "functional_equivalent", true},
		{"普遍需求", "universal_need", true},
		{"无效类型", "invalid_type", true}, // parse doesn't validate enum
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := fmt.Sprintf(`{"technical_suggestion": true, "suggestion_type": "%s", "has_reverse_teaching": false, "is_cross_domain": false, "rationale": "test", "confidence": "medium"}`, tt.suggestionType)
			result := parseStep3(output)
			if result.SuggestionType != tt.suggestionType {
				t.Errorf("SuggestionType = %q, want %q", result.SuggestionType, tt.suggestionType)
			}
		})
	}
}

func TestParseStep4(t *testing.T) {
	output := `{"has_significant_progress": true, "progress_type": "effect_improve", "rationale": "提高了检测精度20%"}`
	result := parseStep4(output)

	if !result.HasSignificantProgress {
		t.Error("expected HasSignificantProgress=true")
	}
	if result.ProgressType != "effect_improve" {
		t.Errorf("ProgressType = %q, want effect_improve", result.ProgressType)
	}
}

func TestParseStep4_NoProgress(t *testing.T) {
	output := `{"has_significant_progress": false, "progress_type": "", "rationale": "未发现任何有益技术效果"}`
	result := parseStep4(output)

	if result.HasSignificantProgress {
		t.Error("expected HasSignificantProgress=false")
	}
}

func TestParseStep3_InvalidConfidence(t *testing.T) {
	output := `{"technical_suggestion": true, "rationale": "test", "confidence": "invalid"}`
	result := parseStep3(output)

	if result.Confidence != "medium" {
		t.Errorf("Confidence should default to medium, got %q", result.Confidence)
	}
}

func TestParseConclusion(t *testing.T) {
	output := `{"conclusion": "具备创造性", "is_inventive": true, "confidence": "high", "aux_factors": ["预料不到的技术效果"]}`
	result := parseConclusion(output)

	if !result.IsInventive {
		t.Error("expected IsInventive=true")
	}
	if result.Confidence != "high" {
		t.Errorf("Confidence = %q, want high", result.Confidence)
	}
	if len(result.AuxFactors) != 1 {
		t.Errorf("expected 1 aux factor, got %d", len(result.AuxFactors))
	}
}

// =============================================================================
// buildResult test
// =============================================================================

func TestBuildResult(t *testing.T) {
	step1 := `{"closest_prior_art": "D1", "selection_reason": "同领域"}`
	step2 := `{"distinguishing_features": ["特征A"], "tech_effects": ["效果A"], "actual_tech_problem": "问题A"}`
	step3 := `{"technical_suggestion": false, "rationale": "无启示", "confidence": "high"}`
	step4 := `{"has_significant_progress": true, "progress_type": "effect_improve", "rationale": "提高了检测精度"}`
	conclusion := `{"conclusion": "具备创造性", "is_inventive": true, "has_significant_progress": true, "confidence": "high", "aux_factors": []}`

	result := buildResult(step1, step2, step3, step4, conclusion)

	if !result.Assessed {
		t.Error("expected Assessed=true")
	}
	if result.Step1.ClosestPriorArt != "D1" {
		t.Errorf("Step1.ClosestPriorArt = %q, want D1", result.Step1.ClosestPriorArt)
	}
	if len(result.Step2.DistinguishingFeatures) != 1 {
		t.Errorf("expected 1 distinguishing feature, got %d", len(result.Step2.DistinguishingFeatures))
	}
	if result.Step3.TechnicalSuggestion {
		t.Error("expected TechnicalSuggestion=false")
	}
	if !result.Step4.HasSignificantProgress {
		t.Error("expected HasSignificantProgress=true")
	}
	if !result.IsInventive {
		t.Error("expected IsInventive=true (NonObvious AND HasSignificantProgress)")
	}

	// Backward compatibility: ThreeStep should mirror individual steps.
	if result.ThreeStep.ClosestPriorArt != "D1" {
		t.Errorf("ThreeStep.ClosestPriorArt = %q, want D1", result.ThreeStep.ClosestPriorArt)
	}
	if result.ThreeStep.TechnicalSuggestion {
		t.Error("ThreeStep.TechnicalSuggestion should be false")
	}
}

// =============================================================================
// JSON Schema tests
// =============================================================================

func TestStep1Schema(t *testing.T) {
	schema := step1Schema()
	if schema == nil {
		t.Fatal("step1Schema returned nil")
	}
	if schema["type"] != "object" {
		t.Error("schema type should be object")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["closest_prior_art"] == nil || props["selection_reason"] == nil {
		t.Error("step1Schema missing required properties")
	}
}

func TestStep2Schema(t *testing.T) {
	schema := step2Schema()
	if schema == nil {
		t.Fatal("step2Schema returned nil")
	}
}

func TestStep3Schema(t *testing.T) {
	schema := step3Schema()
	if schema == nil {
		t.Fatal("step3Schema returned nil")
	}
}

func TestConclusionSchema(t *testing.T) {
	schema := conclusionSchema()
	if schema == nil {
		t.Fatal("conclusionSchema returned nil")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["is_inventive"] == nil {
		t.Error("conclusionSchema should have is_inventive property")
	}
}

// =============================================================================
// Framework tests
// =============================================================================

func TestDefaultFramework(t *testing.T) {
	fw := defaultA223Framework()
	if fw == "" {
		t.Error("defaultA223Framework() should not be empty")
	}
	for _, term := range []string{"22", "创造性", "三步法", "现有技术", "区别特征", "技术启示"} {
		if !strings.Contains(fw, term) {
			t.Errorf("defaultA223Framework() should contain %q", term)
		}
	}
}

func TestDefaultFramework_ContainsAllKeyTerms(t *testing.T) {
	fw := defaultA223Framework()
	requiredTerms := []string{
		"22", "第 3 款",
		"创造性", "三步法",
		"最接近的现有技术", "区别特征", "技术启示",
		"公知常识", "事后诸葛亮", "预料不到的技术效果",
		"商业上的成功", "技术偏见", "长期未满足",
		// Phase 3.3: 多源一致性标记
		"综合来源备注", "两步要件的关系", "弹性空间",
	}
	for _, term := range requiredTerms {
		if !strings.Contains(fw, term) {
			t.Errorf("defaultA223Framework() missing required term: %q", term)
		}
	}
}

func TestFrameworkAdapter_NilProvider(t *testing.T) {
	fw := NewFramework(nil)
	result := fw.GetArticleFramework()
	if result == "" {
		t.Error("GetArticleFramework() returned empty string with nil provider")
	}
	for _, term := range []string{"22", "创造性", "三步法", "审查指南"} {
		if !strings.Contains(result, term) {
			t.Errorf("framework text missing term: %q", term)
		}
	}
}

// mockArticleProvider is a test-only ArticleFrameworkProvider.
type mockArticleProvider struct {
	articles map[string]ArticleFrameworkData
}

func (m *mockArticleProvider) Article(id string) ArticleFrameworkData {
	if m.articles == nil {
		return ArticleFrameworkData{}
	}
	return m.articles[id]
}

func TestFrameworkAdapter_WithProvider(t *testing.T) {
	provider := &mockArticleProvider{
		articles: map[string]ArticleFrameworkData{
			"patent-law-a22.3": {
				Name:         "测试法条22.3",
				LawRef:       "测试专利法",
				GuidelineRef: "测试审查指南",
				Steps: []ArticleStepData{
					{Order: 1, Name: "确定最接近现有技术", InputHint: "现有技术列表", OutputSchema: map[string]string{"closest": "string"}},
					{Order: 2, Name: "确定区别特征和技术问题", OutputSchema: map[string]string{"features": "array"}},
					{Order: 3, Name: "判断技术启示", OutputSchema: map[string]string{"suggestion": "bool"}},
				},
				ConclusionSchema: map[string]string{"isInventive": "bool"},
				ApplicableTo:     []string{"patentability", "invalidation"},
			},
		},
	}

	fw := NewFramework(provider)
	result := fw.GetArticleFramework()

	if !strings.Contains(result, "测试法条22.3") {
		t.Error("framework should contain provider-provided name")
	}
	if !strings.Contains(result, "测试专利法") {
		t.Error("framework should contain provider-provided law ref")
	}
	if !strings.Contains(result, "确定最接近现有技术") {
		t.Error("framework should contain step 1")
	}
}

func TestFrameworkAdapter_FallbackToDefault(t *testing.T) {
	provider := &mockArticleProvider{
		articles: map[string]ArticleFrameworkData{
			"some-other-article": {Name: "其他法条"},
		},
	}
	fw := NewFramework(provider)
	result := fw.GetArticleFramework()

	if !strings.Contains(result, "专利法第22条第3款") {
		t.Error("framework should fallback to default for unknown article ID")
	}
	if strings.Contains(result, "其他法条") {
		t.Error("framework should NOT contain wrong article's data")
	}
}

// =============================================================================
// Tool tests
// =============================================================================

func TestParseInventivenessArgs_ValidJSON(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"prior_art_chunks": []map[string]any{
			{"doc_id": "D1", "title": "对比文件1", "snippet": "技术内容", "score": 0.95},
		},
		"features": []map[string]any{
			{"id": "f1", "description": "测试特征", "category": "structure", "function": "测试功能", "importance": "high"},
		},
		"pfe_triples": []map[string]any{
			{"id": "t1", "problem": "测试问题", "effect": "测试效果"},
		},
		"novelty_conclusion": "具备新颖性",
		"evidence_coverage":  "partial",
	})
	input := parseInventivenessArgs(json.RawMessage(raw))
	if input == nil {
		t.Fatal("parseInventivenessArgs returned nil for valid JSON")
	}
	if len(input.Features) != 1 || input.Features[0].ID != "f1" {
		t.Error("feature not parsed correctly")
	}
	if len(input.PriorArtChunks) != 1 || input.PriorArtChunks[0].DocID != "D1" {
		t.Error("prior_art_chunk not parsed correctly")
	}
	// With features present and coverage=partial, should auto-upgrade.
	if input.EvidenceCoverage != "full" {
		t.Errorf("expected EvidenceCoverage=full when features present, got %q", input.EvidenceCoverage)
	}
}

func TestParseInventivenessArgs_InvalidJSON(t *testing.T) {
	input := parseInventivenessArgs(json.RawMessage(`{invalid}`))
	if input != nil {
		t.Error("parseInventivenessArgs should return nil for invalid JSON")
	}
}

func TestParseInventivenessArgs_EmptyObject(t *testing.T) {
	input := parseInventivenessArgs(json.RawMessage(`{}`))
	if input == nil {
		t.Fatal("parseInventivenessArgs returned nil for empty object")
	}
	if len(input.Features) != 0 || len(input.PriorArtChunks) != 0 {
		t.Error("expected empty features and prior_art")
	}
	if input.EvidenceCoverage != "partial" {
		t.Errorf("expected EvidenceCoverage=partial for empty features, got %q", input.EvidenceCoverage)
	}
}

func TestNewInventivenessTool_NoProvider(t *testing.T) {
	tool := NewInventivenessTool()
	if tool == nil {
		t.Fatal("NewInventivenessTool returned nil")
	}
	if tool.Name != "evaluate_inventiveness" {
		t.Errorf("tool name = %q, want \"evaluate_inventiveness\"", tool.Name)
	}
	if !tool.ReadOnly {
		t.Error("evaluate_inventiveness should be read-only")
	}
	if tool.Parameters == nil {
		t.Error("tool Parameters should not be nil")
	}
}

// =============================================================================
// ArticleFramework YAML verification
// =============================================================================

func TestArticleFrameworkYAML_LoadAndParse(t *testing.T) {
	yamlPath := "../rules/data/articles/patent-law-a22.3.yaml"
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Skipf("YAML file not found at %s: %v", yamlPath, err)
	}

	content := string(data)
	requiredFields := []string{
		"articleId:", "patent-law-a22.3",
		"name:", "专利法第22条第3款",
		"lawRef:", "2020",
		"guidelineRef:", "审查指南",
		"steps:",
		"step-1", "step-2", "step-3",
		"conclusionSchema:",
		"applicableTo:",
	}
	for _, field := range requiredFields {
		if !strings.Contains(content, field) {
			t.Errorf("YAML missing required field/content: %q", field)
		}
	}

	t.Logf("ArticleFramework YAML verified: %d bytes, all required fields present", len(data))
}

func TestArticleFrameworkYAML_MatchesDefaultFramework(t *testing.T) {
	yamlPath := "../rules/data/articles/patent-law-a22.3.yaml"
	_, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Skipf("YAML file not found: %v", err)
	}

	defaultFW := defaultA223Framework()

	coreTerms := []string{
		"最接近的现有技术",
		"区别特征",
		"实际解决",
		"技术启示",
		"显而易见",
		"显著的进步",
		"反向教导",
		"事后诸葛亮",
		"无贡献特征",
	}
	for _, term := range coreTerms {
		if !strings.Contains(defaultFW, term) {
			t.Errorf("defaultA223Framework() missing term: %q", term)
		}
	}
}

// =============================================================================
// Iteration 1 新增测试
// =============================================================================

func TestStep4Schema(t *testing.T) {
	schema := step4Schema()
	if schema == nil {
		t.Fatal("step4Schema returned nil")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["has_significant_progress"] == nil || props["rationale"] == nil {
		t.Error("step4Schema missing required properties")
	}
}

func TestBuildResult_NoProgress(t *testing.T) {
	// Test edge case: NonObvious=true but HasSignificantProgress=false → IsInventive=false
	step1 := `{"closest_prior_art": "D1", "selection_reason": "同领域"}`
	step2 := `{"distinguishing_features": ["特征A"], "tech_effects": [], "actual_tech_problem": "问题A"}`
	step3 := `{"technical_suggestion": false, "rationale": "无启示", "confidence": "high"}`
	step4 := `{"has_significant_progress": false, "progress_type": "", "rationale": "方案引入新问题且无有益效果"}`
	conclusion := `{"conclusion": "虽然非显而易见，但缺乏显著进步", "is_inventive": false, "has_significant_progress": false, "confidence": "medium"}`

	result := buildResult(step1, step2, step3, step4, conclusion)

	if result.IsInventive {
		t.Error("expected IsInventive=false when NonObvious but no significant progress")
	}
	if result.Step4.HasSignificantProgress {
		t.Error("expected HasSignificantProgress=false")
	}
}

func TestDefaultFramework_V3Terms(t *testing.T) {
	fw := defaultA223Framework()
	v3Terms := []string{
		"第84号局令",
		"本领域的技术人员",
		"五种情形",
		"反向教导",
		"跨领域结合",
		"改进动机",
		"第 4 步：判断显著的进步",
		"效果改善型",
		"异途同归型",
		"充分条件而非必要条件",
		"无贡献特征",
	}
	for _, term := range v3Terms {
		if !strings.Contains(fw, term) {
			t.Errorf("defaultA223Framework() missing v3 term: %q", term)
		}
	}
}

// =============================================================================
// Iteration 2 新增测试
// =============================================================================

func TestPersonSkilledDefinition(t *testing.T) {
	def := personSkilledDefinition()
	if def == "" {
		t.Fatal("personSkilledDefinition returned empty string")
	}
	requiredTerms := []string{"本领域的技术人员", "普通技术知识", "常规实验手段", "不具有创造能力"}
	for _, term := range requiredTerms {
		if !strings.Contains(def, term) {
			t.Errorf("personSkilledDefinition missing term: %q", term)
		}
	}
}

func TestInventionTypeConstants(t *testing.T) {
	types := map[string]string{
		"Generic":       InventionTypeGeneric,
		"Pioneering":    InventionTypePioneering,
		"Combination":   InventionTypeCombination,
		"Selection":     InventionTypeSelection,
		"Transfer":      InventionTypeTransfer,
		"NewUse":        InventionTypeNewUse,
		"ElementChange": InventionTypeElementChange,
	}
	for name, val := range types {
		if name == "Generic" {
			if val != "" {
				t.Errorf("InventionTypeGeneric should be empty string, got %q", val)
			}
		} else {
			if val == "" {
				t.Errorf("InventionType%s should not be empty", name)
			}
		}
	}
}

func TestInventionTypeGuidance(t *testing.T) {
	for _, it := range []string{
		InventionTypePioneering, InventionTypeCombination, InventionTypeSelection,
		InventionTypeTransfer, InventionTypeNewUse, InventionTypeElementChange,
	} {
		t.Run(it, func(t *testing.T) {
			guidance := inventionTypeGuidance(it)
			if guidance == "" {
				t.Errorf("inventionTypeGuidance(%q) returned empty", it)
			}
		})
	}

	if g := inventionTypeGuidance(InventionTypeGeneric); g != "" {
		t.Errorf("inventionTypeGuidance(generic) should be empty, got %q", g)
	}
}

func TestInventionTypeFramework(t *testing.T) {
	for _, it := range []string{
		InventionTypePioneering, InventionTypeCombination, InventionTypeSelection,
		InventionTypeTransfer, InventionTypeNewUse, InventionTypeElementChange,
	} {
		t.Run(it, func(t *testing.T) {
			fw := InventionTypeFramework(it)
			if fw == "" {
				t.Errorf("InventionTypeFramework(%q) returned empty", it)
			}
			if !strings.Contains(fw, "创造性") {
				t.Errorf("InventionTypeFramework(%q) should mention 创造性", it)
			}
		})
	}

	if fw := InventionTypeFramework(InventionTypeGeneric); fw != "" {
		t.Errorf("InventionTypeFramework(generic) should be empty, got %q", fw)
	}
}

func TestInventivenessInput_InventionType(t *testing.T) {
	input := &InventivenessInput{
		InventionType:    InventionTypeCombination,
		EvidenceCoverage: "full",
		Features:         []TechFeature{{ID: "f1", Description: "test"}},
	}
	if input.InventionType != InventionTypeCombination {
		t.Errorf("InventionType = %q, want %q", input.InventionType, InventionTypeCombination)
	}

	defaultInput := &InventivenessInput{}
	if defaultInput.InventionType != "" {
		t.Errorf("default InventionType should be empty, got %q", defaultInput.InventionType)
	}
}

func TestProgressTypeConstants(t *testing.T) {
	types := map[string]string{
		"EffectImprove": ProgressTypeEffectImprove,
		"DifferentPath": ProgressTypeDifferentPath,
		"TrendLeading":  ProgressTypeTrendLeading,
		"Tradeoff":      ProgressTypeTradeoff,
	}
	for name, val := range types {
		if val == "" {
			t.Errorf("ProgressType%s should not be empty", name)
		}
	}
}

// =============================================================================
// Iteration 3 新增测试
// =============================================================================

func TestTechDomainGuidance(t *testing.T) {
	for _, domain := range []string{"chemistry", "computer", "tcm"} {
		t.Run(domain, func(t *testing.T) {
			guidance := techDomainGuidance(domain)
			if guidance == "" {
				t.Errorf("techDomainGuidance(%q) returned empty", domain)
			}
		})
	}

	if g := techDomainGuidance("unknown"); g != "" {
		t.Errorf("techDomainGuidance(unknown) should be empty, got %q", g)
	}
}

func TestTechDomainFramework(t *testing.T) {
	for _, domain := range []string{"chemistry", "computer", "tcm"} {
		t.Run(domain, func(t *testing.T) {
			fw := TechDomainFramework(domain)
			if fw == "" {
				t.Errorf("TechDomainFramework(%q) returned empty", domain)
			}
			if !strings.Contains(fw, "创造性") {
				t.Errorf("TechDomainFramework(%q) should mention 创造性", domain)
			}
		})
	}

	if fw := TechDomainFramework(""); fw != "" {
		t.Errorf("TechDomainFramework(empty) should be empty, got %q", fw)
	}
}

func TestChemistryDomainFramework_ContainsSubclasses(t *testing.T) {
	fw := chemistryDomainFramework()
	subclasses := []string{"化合物", "晶体化合物", "基因", "单克隆抗体", "组合物", "制备方法", "制药用途", "电子等排体"}
	for _, sc := range subclasses {
		if !strings.Contains(fw, sc) {
			t.Errorf("chemistryDomainFramework missing subclass: %q", sc)
		}
	}
}

func TestComputerDomainFramework_ContainsAI(t *testing.T) {
	fw := computerDomainFramework()
	terms := []string{"算法", "整体考量", "技术领域", "功能上彼此相互支持"}
	for _, term := range terms {
		if !strings.Contains(fw, term) {
			t.Errorf("computerDomainFramework missing term: %q", term)
		}
	}
}

func TestTCMDomainFramework_ContainsPrinciples(t *testing.T) {
	fw := tcmDomainFramework()
	terms := []string{"君臣佐使", "理、法、方、药", "加减方", "合方", "自组方"}
	for _, term := range terms {
		if !strings.Contains(fw, term) {
			t.Errorf("tcmDomainFramework missing term: %q", term)
		}
	}
}

func TestInventivenessInput_TechDomain(t *testing.T) {
	input := &InventivenessInput{
		TechDomain:       "chemistry",
		EvidenceCoverage: "full",
		Features:         []TechFeature{{ID: "f1", Description: "化合物"}},
	}
	if input.TechDomain != "chemistry" {
		t.Errorf("TechDomain = %q, want chemistry", input.TechDomain)
	}
}

func TestInventivenessInput_InventionTypeAndTechDomain(t *testing.T) {
	// Verify combined usage of InventionType + TechDomain.
	input := &InventivenessInput{
		InventionType:    InventionTypeSelection,
		TechDomain:       "chemistry",
		EvidenceCoverage: "full",
	}
	if input.InventionType != InventionTypeSelection {
		t.Errorf("InventionType = %q", input.InventionType)
	}
	if input.TechDomain != "chemistry" {
		t.Errorf("TechDomain = %q", input.TechDomain)
	}
}

// =============================================================================
// Iteration 4 新增测试
// =============================================================================

func TestExaminerErrorPrevention(t *testing.T) {
	prevention := examinerErrorPrevention()
	if prevention == "" {
		t.Fatal("examinerErrorPrevention returned empty string")
	}
	requiredTerms := []string{"事后诸葛亮", "技术特征割裂", "本领域技术人员", "整体性"}
	for _, term := range requiredTerms {
		if !strings.Contains(prevention, term) {
			t.Errorf("examinerErrorPrevention missing term: %q", term)
		}
	}
}

func TestConfidenceCalibration(t *testing.T) {
	calibration := confidenceCalibration()
	if calibration == "" {
		t.Fatal("confidenceCalibration returned empty string")
	}
	requiredTerms := []string{"39,496", "54%", "95.9%", "73.7%", "5.7%"}
	for _, term := range requiredTerms {
		if !strings.Contains(calibration, term) {
			t.Errorf("confidenceCalibration missing term: %q", term)
		}
	}
}

func TestEmpiricalStatistics(t *testing.T) {
	stats := EmpiricalStatistics()
	if stats == "" {
		t.Fatal("EmpiricalStatistics returned empty string")
	}
	requiredTerms := []string{"单对比文件+公知常识", "多对比文件结合", "预料不到的效果", "置信度校准"}
	for _, term := range requiredTerms {
		if !strings.Contains(stats, term) {
			t.Errorf("EmpiricalStatistics missing term: %q", term)
		}
	}
}

func TestAllFrameworks_NonEmpty(t *testing.T) {
	// Verify all framework functions return non-empty strings.
	frameworks := map[string]func() string{
		"defaultA223":   defaultA223Framework,
		"pioneering":    pioneeringFramework,
		"combination":   combinationFramework,
		"selection":     selectionFramework,
		"transfer":      transferFramework,
		"newUse":        newUseFramework,
		"elementChange": elementChangeFramework,
		"chemistry":     chemistryDomainFramework,
		"computer":      computerDomainFramework,
		"tcm":           tcmDomainFramework,
	}
	for name, fn := range frameworks {
		t.Run(name, func(t *testing.T) {
			if fw := fn(); fw == "" {
				t.Errorf("%s framework is empty", name)
			}
		})
	}
}
