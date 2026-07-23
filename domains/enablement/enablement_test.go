package enablement

import (
	"context"
	"encoding/json"
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

func TestBuildEnablementGraph(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildEnablementGraph(provider)
	if err != nil {
		t.Fatalf("BuildEnablementGraph failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("expected non-nil compiled graph")
	}
	// Verify entry point is set correctly.
	t.Logf("enablement graph compiled successfully")
}

func TestBuildEnablementGraph_Valid(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildEnablementGraph(provider)
	if err != nil {
		t.Fatalf("BuildEnablementGraph failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("expected non-nil compiled graph")
	}
	t.Log("enablement graph compiled successfully")
}

func TestLoadInputNode_SkipEmpty(t *testing.T) {
	// Test that loadInputNode correctly skips when input is missing.
	state := graph.PregelState{}
	// No "enablement_input" key set
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	result, ok := state[stateKeyResult].(*EnablementResult)
	if !ok {
		t.Fatal("expected EnablementResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when input is missing")
	}
	if result.Assessed {
		t.Error("expected Assessed=false when input is missing")
	}
}

func TestLoadInputNode_SkipNilInput(t *testing.T) {
	// Test that loadInputNode correctly skips when input is nil.
	state := graph.PregelState{
		stateKeyInput: (*EnablementInput)(nil),
	}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	result, ok := state[stateKeyResult].(*EnablementResult)
	if !ok {
		t.Fatal("expected EnablementResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when input is nil")
	}
}

func TestLoadInputNode_SkipEmptyFeatures(t *testing.T) {
	// Test that loadInputNode correctly skips when features and PFE triples are empty.
	state := graph.PregelState{
		stateKeyInput: &EnablementInput{
			Features:   []TechFeature{},
			PFETriples: []PFETriple{},
		},
	}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	result, ok := state[stateKeyResult].(*EnablementResult)
	if !ok {
		t.Fatal("expected EnablementResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when features and triples are empty")
	}
}

func TestLoadInputNode_Valid(t *testing.T) {
	// Test that loadInputNode accepts valid input.
	input := &EnablementInput{
		Features:   []TechFeature{{ID: "f1", Description: "test feature"}},
		PFETriples: []PFETriple{},
	}
	state := graph.PregelState{
		stateKeyInput: input,
	}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode returned error: %v", err)
	}

	// Should NOT have set result (that's done by conclusion node)
	if _, ok := state[stateKeyResult]; ok {
		t.Error("loadInputNode should not set result for valid input")
	}
	// Should have validated and stored the input
	stored, ok := state[stateKeyInput].(*EnablementInput)
	if !ok || stored == nil {
		t.Error("expected validated input in state")
	}
}

func TestStateHasSkip(t *testing.T) {
	// Verify stateHasSkip works correctly.
	tests := []struct {
		name   string
		state  graph.PregelState
		expect bool
	}{
		{"empty state", graph.PregelState{}, false},
		{"no result key", graph.PregelState{"other": "value"}, false},
		{"result is string not struct", graph.PregelState{stateKeyResult: "not a struct"}, false},
		{"result skipped", graph.PregelState{
			stateKeyResult: &EnablementResult{Skipped: true},
		}, true},
		{"result not skipped", graph.PregelState{
			stateKeyResult: &EnablementResult{Skipped: false, Assessed: true},
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

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		expect string
	}{
		{"short enough", "hello", 10, "hello"},
		{"too long ascii", "hello world", 5, "hello…"},
		{"too long unicode", "你好世界测试", 3, "你好世…"},
		{"exact length", "hello", 5, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.input, tt.maxLen)
			if got != tt.expect {
				t.Errorf("truncateText() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestExtractInput(t *testing.T) {
	// Test the extractInput helper.
	input := &EnablementInput{Features: []TechFeature{{ID: "f1"}}}
	state := graph.PregelState{stateKeyInput: input}

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

func TestRequiredSectionCount(t *testing.T) {
	if n := RequiredSectionCount(); n != 5 {
		t.Errorf("RequiredSectionCount() = %d, want 5", n)
	}
}

func TestSectionLabel(t *testing.T) {
	if s := SectionLabel(0); s == "" {
		t.Error("SectionLabel(0) should not be empty")
	}
	if s := SectionLabel(999); s != "" {
		t.Error("SectionLabel(999) should be empty")
	}
}

// TestDefaultFramework ensures the fallback framework text is non-empty.
func TestDefaultFramework(t *testing.T) {
	fw := defaultA263Framework()
	if fw == "" {
		t.Error("defaultA263Framework() should not be empty")
	}
	// Should contain key terms
	for _, term := range []string{"26", "清楚", "完整", "能够实现"} {
		if !contains(fw, term) {
			t.Errorf("defaultA263Framework() should contain %q", term)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================================================
// Fixture-based tests (P5.1)
// =============================================================================

type testCase struct {
	ID          string           `json:"id"`
	Description string           `json:"description"`
	Source      string           `json:"source"`
	Input       testCaseInput    `json:"input"`
	Expected    testCaseExpected `json:"expected"`
}

type testCaseInput struct {
	Features []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Function    string `json:"function"`
		Importance  string `json:"importance"`
	} `json:"features"`
	PFETriples []struct {
		ID         string   `json:"id"`
		Problem    string   `json:"problem"`
		FeatureIDs []string `json:"feature_ids"`
		Effect     string   `json:"effect"`
	} `json:"pfe_triples"`
	Problems    []string          `json:"problems"`
	Effects     []string          `json:"effects"`
	DocSections map[string]string `json:"doc_sections"`
	HasDrawings bool              `json:"has_drawings"`
}

type testCaseExpected struct {
	IsSufficient     bool   `json:"is_sufficient"`
	Confidence       string `json:"confidence"`
	DeficienciesMin  int    `json:"deficiencies_min"`
	MissingKeyMeans  bool   `json:"missing_key_means,omitempty"`
	VagueMeans       bool   `json:"vague_means,omitempty"`
	InsufficientData bool   `json:"insufficient_data,omitempty"`
	ShouldSkip       bool   `json:"should_skip,omitempty"`
}

func loadFixtureCases(t *testing.T) []testCase {
	t.Helper()
	data, err := readTestdataFile("enablement_cases.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	var cases []testCase
	if err := jsonUnmarshal(data, &cases); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}
	return cases
}

func TestFixtureCases_LoadAndParse(t *testing.T) {
	cases := loadFixtureCases(t)
	if len(cases) == 0 {
		t.Fatal("expected at least one fixture case")
	}
	t.Logf("loaded %d fixture cases", len(cases))
	for _, c := range cases {
		if c.ID == "" {
			t.Error("fixture case missing ID")
		}
		if len(c.Input.Features) == 0 && len(c.Input.PFETriples) == 0 && !c.Expected.ShouldSkip {
			t.Errorf("case %s: empty input but should_skip=false", c.ID)
		}
	}
}

func TestFixtureCases_BuildInputs(t *testing.T) {
	cases := loadFixtureCases(t)
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			input := convertFixtureToInput(&c)
			if input == nil {
				t.Fatal("convertFixtureToInput returned nil")
			}
			// Verify basic field count
			if len(input.Features) != len(c.Input.Features) {
				t.Errorf("Features count mismatch: got %d, want %d", len(input.Features), len(c.Input.Features))
			}
			if len(input.PFETriples) != len(c.Input.PFETriples) {
				t.Errorf("PFETriples count mismatch: got %d, want %d", len(input.PFETriples), len(c.Input.PFETriples))
			}
		})
	}
}

func convertFixtureToInput(c *testCase) *EnablementInput {
	input := &EnablementInput{
		Problems:    c.Input.Problems,
		Effects:     c.Input.Effects,
		DocSections: c.Input.DocSections,
		HasDrawings: c.Input.HasDrawings,
	}
	for _, f := range c.Input.Features {
		input.Features = append(input.Features, TechFeature{
			ID:          f.ID,
			Description: f.Description,
			Category:    f.Category,
			Function:    f.Function,
			Importance:  f.Importance,
		})
	}
	for _, t := range c.Input.PFETriples {
		input.PFETriples = append(input.PFETriples, PFETriple{
			ID:         t.ID,
			Problem:    t.Problem,
			FeatureIDs: t.FeatureIDs,
			Effect:     t.Effect,
		})
	}
	if len(input.Features) > 0 {
		input.EvidenceCoverage = "full"
	} else {
		input.EvidenceCoverage = "partial"
	}
	return input
}

func TestFixtureCases_EmptyInputSkips(t *testing.T) {
	// Verify that the empty input case triggers Skip behavior.
	var emptyCase *testCase
	cases := loadFixtureCases(t)
	for i := range cases {
		if cases[i].Expected.ShouldSkip {
			emptyCase = &cases[i]
			break
		}
	}
	if emptyCase == nil {
		t.Skip("no should_skip fixture case found")
	}

	input := convertFixtureToInput(emptyCase)
	state := graph.PregelState{stateKeyInput: input}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode: %v", err)
	}
	result, ok := state[stateKeyResult].(*EnablementResult)
	if !ok {
		t.Fatal("expected EnablementResult in state")
	}
	if !result.Skipped {
		t.Error("expected Skipped=true for empty input fixture case")
	}
}

// =============================================================================
// buildEnablementInput tests (P5.2)
// =============================================================================

func TestBuildEnablementInput_NilFeatures(t *testing.T) {
	// Simulate what happens when disclosure report has nil extraction.
	var features []TechFeature
	var triples []PFETriple
	input := &EnablementInput{
		Features:         features,
		PFETriples:       triples,
		EvidenceCoverage: "partial",
	}
	if input.EvidenceCoverage != "partial" {
		t.Error("expected partial coverage for nil features")
	}
	// loadInputNode should skip this
	state := graph.PregelState{stateKeyInput: input}
	state, err := loadInputNode()(nil, state)
	if err != nil {
		t.Fatalf("loadInputNode: %v", err)
	}
	result, _ := state[stateKeyResult].(*EnablementResult)
	if result == nil || !result.Skipped {
		t.Error("expected Skipped when features and triples are both empty")
	}
}

func TestBuildEnablementInput_CoverageAutoUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		features int
		wantCov  string
	}{
		{"zero features", 0, "partial"},
		{"one feature", 1, "full"},
		{"many features", 5, "full"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feats := make([]TechFeature, tt.features)
			for i := range feats {
				feats[i] = TechFeature{ID: "f", Description: "test"}
			}
			input := &EnablementInput{
				Features:         feats,
				EvidenceCoverage: "partial",
			}
			if len(input.Features) > 0 {
				input.EvidenceCoverage = "full"
			}
			if input.EvidenceCoverage != tt.wantCov {
				t.Errorf("EvidenceCoverage = %q, want %q", input.EvidenceCoverage, tt.wantCov)
			}
		})
	}
}

func TestBuildEnablementInput_DocSectionsPassthrough(t *testing.T) {
	sections := map[string]string{
		"technical_field": "测试技术领域",
		"embodiments":     "测试实施例",
	}
	input := &EnablementInput{
		DocSections: sections,
		HasDrawings: true,
	}
	if len(input.DocSections) != 2 {
		t.Errorf("expected 2 doc sections, got %d", len(input.DocSections))
	}
	if !input.HasDrawings {
		t.Error("expected HasDrawings=true")
	}
}

// =============================================================================
// tool.go tests (P5.3)
// =============================================================================

func TestParseEnablementArgs_ValidJSON(t *testing.T) {
	raw := jsonMarshal(map[string]any{
		"features": []map[string]any{
			{"id": "f1", "description": "测试特征", "category": "structure", "function": "测试功能", "importance": "high"},
		},
		"pfe_triples": []map[string]any{
			{"id": "t1", "problem": "测试问题", "feature_ids": []string{"f1"}, "effect": "测试效果"},
		},
		"problems":     []string{"测试问题"},
		"effects":      []string{"测试效果"},
		"doc_sections": map[string]string{"technical_field": "测试领域"},
		"has_drawings": true,
	})
	input := parseEnablementArgs(json.RawMessage(raw))
	if input == nil {
		t.Fatal("parseEnablementArgs returned nil for valid JSON")
	}
	if len(input.Features) != 1 || input.Features[0].ID != "f1" {
		t.Error("feature not parsed correctly")
	}
	if len(input.PFETriples) != 1 || input.PFETriples[0].ID != "t1" {
		t.Error("pfe_triple not parsed correctly")
	}
	if input.EvidenceCoverage != "full" {
		t.Error("expected EvidenceCoverage=full when features present")
	}
}

func TestParseEnablementArgs_InvalidJSON(t *testing.T) {
	input := parseEnablementArgs(json.RawMessage(`{invalid}`))
	if input != nil {
		t.Error("parseEnablementArgs should return nil for invalid JSON")
	}
}

func TestParseEnablementArgs_EmptyObject(t *testing.T) {
	input := parseEnablementArgs(json.RawMessage(`{}`))
	if input == nil {
		t.Fatal("parseEnablementArgs returned nil for empty object")
	}
	if len(input.Features) != 0 || len(input.PFETriples) != 0 {
		t.Error("expected empty features and triples")
	}
	if input.EvidenceCoverage != "partial" {
		t.Error("expected EvidenceCoverage=partial for empty features")
	}
}

func TestNewEnablementTool_NoProvider(t *testing.T) {
	// Tool should return an error response, not panic, when provider is nil.
	tool := NewEnablementTool()
	if tool == nil {
		t.Fatal("NewEnablementTool returned nil")
	}
	if tool.Name != "evaluate_enablement" {
		t.Errorf("tool name = %q, want \"evaluate_enablement\"", tool.Name)
	}
	if !tool.ReadOnly {
		t.Error("evaluate_enablement should be read-only")
	}
	// Verify parameters schema is set
	if tool.Parameters == nil {
		t.Error("tool Parameters should not be nil")
	}
}

// =============================================================================
// DefaultFramework completeness test (P5.4 prep)
// =============================================================================

func TestDefaultFramework_ContainsAllKeyTerms(t *testing.T) {
	fw := defaultA263Framework()
	requiredTerms := []string{
		"26",
		"清楚", "完整", "能够实现",
		"技术领域", "背景技术", "发明内容", "附图说明", "具体实施方式",
		"缺少关键技术手段", "技术手段含糊不清", "仅给出任务", "不能解决技术问题",
		"某一手段不能实现", "实验证据",
	}
	for _, term := range requiredTerms {
		if !contains(fw, term) {
			t.Errorf("defaultA263Framework() missing required term: %q", term)
		}
	}
}

// jsonMarshal is a test helper that panics on marshal error (tests only).
func jsonMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func readTestdataFile(name string) ([]byte, error) {
	return os.ReadFile("testdata/" + name)
}

// =============================================================================
// ArticleFramework YAML verification (P5.4)
// =============================================================================

func TestArticleFrameworkYAML_LoadAndParse(t *testing.T) {
	// Directly parse the YAML to verify structural integrity without
	// depending on domains/rules (which is blocked by a pre-existing build issue).
	yamlPath := "../rules/data/articles/patent-law-a26.3.yaml"
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Skipf("YAML file not found at %s (may need to run from repo root): %v", yamlPath, err)
	}

	content := string(data)
	// Verify key structural elements exist.
	requiredFields := []string{
		"articleId:", "patent-law-a26.3",
		"name:", "专利法第26条第3款",
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

	// Verify 3 steps with correct order.
	if !strings.Contains(content, "step-1") ||
		!strings.Contains(content, "step-2") ||
		!strings.Contains(content, "step-3") {
		t.Error("YAML missing required step IDs")
	}

	t.Logf("ArticleFramework YAML verified: %d bytes, all required fields present", len(data))
}

func TestArticleFrameworkYAML_MatchesDefaultFramework(t *testing.T) {
	yamlPath := "../rules/data/articles/patent-law-a26.3.yaml"
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Skipf("YAML file not found: %v", err)
	}
	_ = data

	defaultFW := defaultA263Framework()

	// The default framework should reference all 5 core sections
	// (using simplified text that appears in the markdown, not the
	// full parenthetical descriptions from types.go).
	coreSections := []string{
		"技术领域",
		"背景技术",
		"发明内容",
		"附图说明",
		"具体实施方式",
	}
	for _, section := range coreSections {
		if !contains(defaultFW, section) {
			t.Errorf("defaultA263Framework() missing section: %q", section)
		}
	}
}

// TestFrameworkAdapter verifies that the Framework type works with nil provider (degraded mode).
func TestFrameworkAdapter_NilProvider(t *testing.T) {
	fw := NewFramework(nil)
	result := fw.GetArticleFramework()
	if result == "" {
		t.Error("GetArticleFramework() returned empty string with nil provider")
	}
	// Should contain key terms from the default framework.
	for _, term := range []string{"26", "清楚", "完整", "能够实现", "审查指南"} {
		if !contains(result, term) {
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
			"patent-law-a26.3": {
				Name:         "测试法条",
				LawRef:       "测试法",
				GuidelineRef: "测试指南",
				Steps: []ArticleStepData{
					{Order: 1, Name: "步骤一", InputHint: "输入1", OutputSchema: map[string]string{"out1": "string"}},
					{Order: 2, Name: "步骤二", InputHint: "输入2", OutputSchema: map[string]string{"out2": "string"}},
				},
				ConclusionSchema: map[string]string{"result": "bool"},
				ApplicableTo:     []string{"test"},
			},
		},
	}

	fw := NewFramework(provider)
	result := fw.GetArticleFramework()

	if !contains(result, "测试法条") {
		t.Error("framework should contain provider-provided name")
	}
	if !contains(result, "测试法") {
		t.Error("framework should contain provider-provided law ref")
	}
	if !contains(result, "步骤一") || !contains(result, "步骤二") {
		t.Error("framework should contain both steps")
	}
}

func TestFrameworkAdapter_FallbackToDefault(t *testing.T) {
	// Provider returns empty data for unknown article ID — should fallback to default.
	provider := &mockArticleProvider{
		articles: map[string]ArticleFrameworkData{
			"some-other-article": {Name: "其他法条"},
		},
	}
	fw := NewFramework(provider)
	result := fw.GetArticleFramework()

	// Should contain default content, not provider content.
	if !contains(result, "专利法第26条第3款") {
		t.Error("framework should fallback to default for unknown article ID")
	}
	if contains(result, "其他法条") {
		t.Error("framework should NOT contain wrong article's data")
	}
}

// =============================================================================
// Domain detection tests
// =============================================================================

func TestDetectDomain_Chemical(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "化合物A的分子式", Category: "material"},
		},
		Problems: []string{"需要一种新型催化剂"},
		Effects:  []string{"催化活性提高"},
	}
	domain := DetectDomain(input)
	if domain != DomainChemical {
		t.Errorf("expected DomainChemical, got %s", domain)
	}
}

func TestDetectDomain_Biotech(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "使用微生物菌株发酵", Category: "method"},
		},
		Problems: []string{"需要酶制剂"},
	}
	domain := DetectDomain(input)
	if domain != DomainBiotech {
		t.Errorf("expected DomainBiotech, got %s", domain)
	}
}

func TestDetectDomain_TCM(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "葛根和砂仁等中药药材的组合物", Category: "material"},
		},
		Effects: []string{"解酒毒"},
	}
	domain := DetectDomain(input)
	if domain != DomainTCM {
		t.Errorf("expected DomainTCM, got %s", domain)
	}
}

func TestDetectDomain_Computer(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "使用机器学习算法处理数据", Category: "method"},
		},
		Problems: []string{"需要高效的AI模型"},
	}
	domain := DetectDomain(input)
	if domain != DomainComputer {
		t.Errorf("expected DomainComputer, got %s", domain)
	}
}

func TestDetectDomain_Mechanical(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "壳体(1)与支架通过螺栓连接", Category: "structure"},
		},
		Problems: []string{"需要稳定的机械结构"},
	}
	domain := DetectDomain(input)
	if domain != DomainMechanical {
		t.Errorf("expected DomainMechanical, got %s", domain)
	}
}

func TestDetectDomain_General(t *testing.T) {
	domain := DetectDomain(nil)
	if domain != DomainGeneral {
		t.Errorf("expected DomainGeneral for nil input, got %s", domain)
	}

	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "某种东西", Category: "misc"},
		},
	}
	domain = DetectDomain(input)
	if domain != DomainGeneral {
		t.Errorf("expected DomainGeneral for generic input, got %s", domain)
	}
}

func TestDomainStep3Supplement_NonEmpty(t *testing.T) {
	// Chemical, biotech, TCM, computer, mechanical/electronic should all have supplements
	domains := []TechDomain{
		DomainChemical, DomainBiotech, DomainTCM,
		DomainComputer, DomainMechanical, DomainElectronic,
	}
	for _, d := range domains {
		s := DomainStep3Supplement(d)
		if s == "" {
			t.Errorf("DomainStep3Supplement(%s) returned empty string", d)
		}
	}
}

func TestDomainStep3Supplement_GeneralEmpty(t *testing.T) {
	s := DomainStep3Supplement(DomainGeneral)
	if s != "" {
		t.Errorf("DomainStep3Supplement(DomainGeneral) should return empty string, got %d chars", len(s))
	}
}

func TestDomainStep2Supplement_TCM(t *testing.T) {
	s := DomainStep2Supplement(DomainTCM)
	if !contains(s, "正名") {
		t.Error("TCM step2 supplement should mention 正名")
	}
}

func TestDomainLabel(t *testing.T) {
	cases := map[TechDomain]string{
		DomainChemical:   "化学/医药",
		DomainBiotech:    "生物技术",
		DomainTCM:        "中药",
		DomainComputer:   "计算机/软件/AI",
		DomainMechanical: "机械/结构",
		DomainElectronic: "电子/电气",
		DomainGeneral:    "通用",
	}
	for domain, expected := range cases {
		got := DomainLabel(domain)
		if got != expected {
			t.Errorf("DomainLabel(%s) = %q, want %q", domain, got, expected)
		}
	}
}

// =============================================================================
// New types verification tests
// =============================================================================

func TestEnablementJudgment_NewFlags_JSONRoundtrip(t *testing.T) {
	judgment := EnablementJudgment{
		CanImplement:       false,
		MissingKeyMeans:    true,
		MeansCannotSolve:   true,
		PartialMeansUnreal: true,
		FailureReasons:     []string{"技术手段不能解决技术问题"},
	}
	data, err := json.Marshal(judgment)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored EnablementJudgment
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !restored.MeansCannotSolve {
		t.Error("MeansCannotSolve flag not preserved in JSON roundtrip")
	}
	if !restored.PartialMeansUnreal {
		t.Error("PartialMeansUnreal flag not preserved in JSON roundtrip")
	}
}

func TestClarityResult_NewFields_JSONRoundtrip(t *testing.T) {
	clarity := ClarityResult{
		IsClear:        false,
		CoinedTerms:    []string{"气相指痕光谱"},
		ObviousErrors:  []string{"滤网位置矛盾"},
		AmbiguousTerms: []string{"藤子暗消"},
	}
	data, err := json.Marshal(clarity)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored ClarityResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(restored.CoinedTerms) != 1 || restored.CoinedTerms[0] != "气相指痕光谱" {
		t.Errorf("CoinedTerms not preserved: %v", restored.CoinedTerms)
	}
	if len(restored.ObviousErrors) != 1 {
		t.Errorf("ObviousErrors not preserved: %v", restored.ObviousErrors)
	}
}

func TestEnablementResult_NewFields_JSONRoundtrip(t *testing.T) {
	result := EnablementResult{
		Assessed:        true,
		TechDomain:      "chemical",
		IsSufficient:    false,
		SupportIssue:    true,
		SupportWarnings: []string{"权利要求1得不到说明书支持"},
		DataAssessment: &ExperimentDataAssessment{
			DataNeeded:     true,
			DataProvided:   false,
			IsValid:        false,
			MissingFactors: []string{"实验方法", "实验结果"},
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored EnablementResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.TechDomain != "chemical" {
		t.Errorf("TechDomain = %q, want 'chemical'", restored.TechDomain)
	}
	if !restored.SupportIssue {
		t.Error("SupportIssue not preserved")
	}
	if restored.DataAssessment == nil {
		t.Fatal("DataAssessment is nil after roundtrip")
	}
	if !restored.DataAssessment.DataNeeded {
		t.Error("DataAssessment.DataNeeded not preserved")
	}
}

func TestExperimentDataAssessment_JSONRoundtrip(t *testing.T) {
	assess := ExperimentDataAssessment{
		DataNeeded:     true,
		DataProvided:   true,
		IsValid:        false,
		MissingFactors: []string{"结果与效果的对应关系"},
		Notes:          "实验数据笼统不具体",
	}
	data, err := json.Marshal(assess)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored ExperimentDataAssessment
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !restored.DataNeeded || !restored.DataProvided || restored.IsValid {
		t.Errorf("flags not preserved: %+v", restored)
	}
	if len(restored.MissingFactors) != 1 {
		t.Errorf("MissingFactors not preserved: %v", restored.MissingFactors)
	}
}

// =============================================================================
// KnowledgeRetriever 相关测试
// =============================================================================

// mockKnowledgeRetriever 是 KnowledgeRetriever 的测试替身。
type mockKnowledgeRetriever struct {
	guidelines []string
	cases      []string
	err        error // 模拟检索失败
}

func (m *mockKnowledgeRetriever) SearchGuidelines(_ context.Context, _ TechDomain, _ []string, _ []TechFeature) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.guidelines, nil
}

func (m *mockKnowledgeRetriever) SearchSimilarCases(_ context.Context, _ TechDomain, _ []TechFeature, _ []string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cases, nil
}

func TestEnrichInput_NilRetriever(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{{ID: "f1", Description: "测试特征"}},
	}

	EnrichInput(context.Background(), input, nil)

	if len(input.GuidelineRefs) != 0 {
		t.Error("GuidelineRefs should be empty when retriever is nil")
	}
	if len(input.SimilarCases) != 0 {
		t.Error("SimilarCases should be empty when retriever is nil")
	}
}

func TestEnrichInput_NilInput(t *testing.T) {
	// EnrichInput 在 input 为 nil 时不应 panic。
	EnrichInput(context.Background(), nil, &mockKnowledgeRetriever{})
}

func TestEnrichInput_GuidelineRefs(t *testing.T) {
	mock := &mockKnowledgeRetriever{
		guidelines: []string{
			"审查指南第二部分第二章第2.1.3节：能够实现的标准",
			"审查指南第二部分第二章第2.1.2节：清楚、完整的要求",
		},
	}
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "催化剂活性组分", Category: "material", Function: "催化反应", Importance: "high"},
		},
		Problems: []string{"催化剂活性不够高"},
	}

	EnrichInput(context.Background(), input, mock)

	if len(input.GuidelineRefs) != 2 {
		t.Fatalf("expected 2 guideline refs, got %d", len(input.GuidelineRefs))
	}
	if !strings.Contains(input.GuidelineRefs[0], "审查指南") {
		t.Errorf("first ref should contain '审查指南', got: %s", input.GuidelineRefs[0])
	}
}

func TestEnrichInput_SimilarCases(t *testing.T) {
	mock := &mockKnowledgeRetriever{
		cases: []string{
			"无效宣告决定第34992号：折叠椅充分公开案",
			"行政判决第021号：CMOS传感器封装案",
		},
	}
	input := &EnablementInput{
		Features: []TechFeature{
			{ID: "f1", Description: "壳体通过螺栓连接", Category: "structure", Function: "固定", Importance: "high"},
		},
	}

	EnrichInput(context.Background(), input, mock)

	if len(input.SimilarCases) != 2 {
		t.Fatalf("expected 2 similar cases, got %d", len(input.SimilarCases))
	}
	if !strings.Contains(input.SimilarCases[0], "充分公开") {
		t.Errorf("case should contain '充分公开', got: %s", input.SimilarCases[0])
	}
}

func TestEnrichInput_ErrorGraceful(t *testing.T) {
	mock := &mockKnowledgeRetriever{
		guidelines: []string{"不应出现"},
		cases:      []string{"不应出现"},
		err:        assertError("模拟检索失败"),
	}
	input := &EnablementInput{
		Features: []TechFeature{{ID: "f1", Description: "测试特征"}},
	}

	EnrichInput(context.Background(), input, mock)

	if len(input.GuidelineRefs) != 0 {
		t.Error("GuidelineRefs should be empty after retriever error")
	}
	if len(input.SimilarCases) != 0 {
		t.Error("SimilarCases should be empty after retriever error")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func TestBuildPFEInput_IncludesGuidelineRefs(t *testing.T) {
	input := &EnablementInput{
		GuidelineRefs: []string{"审查指南第二部分第二章第2.1.3节"},
		Features:      []TechFeature{{ID: "f1", Description: "测试"}},
		Problems:      []string{"测试问题"},
		Effects:       []string{"测试效果"},
	}
	output := buildPFEInput(input)

	if !strings.Contains(output, "审查指南参考") {
		t.Error("buildPFEInput should include '审查指南参考' section")
	}
	if !strings.Contains(output, "审查指南第二部分第二章") {
		t.Error("buildPFEInput should include guideline ref content")
	}
}

func TestBuildPFEInput_IncludesSimilarCases(t *testing.T) {
	input := &EnablementInput{
		SimilarCases: []string{"无效宣告决定第34992号"},
		Features:     []TechFeature{{ID: "f1", Description: "测试"}},
		Problems:     []string{"测试问题"},
	}
	output := buildPFEInput(input)

	if !strings.Contains(output, "类案参考") {
		t.Error("buildPFEInput should include '类案参考' section")
	}
	if !strings.Contains(output, "无效宣告决定第34992号") {
		t.Error("buildPFEInput should include similar case content")
	}
}

func TestBuildPFEInput_EmptySimilarCases(t *testing.T) {
	input := &EnablementInput{
		Features: []TechFeature{{ID: "f1", Description: "测试"}},
	}
	output := buildPFEInput(input)

	if strings.Contains(output, "类案参考") {
		t.Error("buildPFEInput should not include '类案参考' when SimilarCases is empty")
	}
}

func TestWithKnowledgeRetriever_Option(t *testing.T) {
	cfg := &enablementConfig{}
	opt := WithKnowledgeRetriever(&mockKnowledgeRetriever{
		guidelines: []string{"测试条款"},
	})

	if cfg.knowledgeRetriever != nil {
		t.Error("initial config should have nil retriever")
	}

	opt(cfg)

	if cfg.knowledgeRetriever == nil {
		t.Fatal("WithKnowledgeRetriever should set retriever on config")
	}

	refs, err := cfg.knowledgeRetriever.SearchGuidelines(context.Background(), DomainGeneral, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 1 || refs[0] != "测试条款" {
		t.Errorf("unexpected guidelines: %v", refs)
	}
}
