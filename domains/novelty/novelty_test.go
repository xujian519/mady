package novelty

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// mockProvider — 用于测试的 LLM Provider 模拟
// =============================================================================

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
// Section 1: Graph compilation tests
// =============================================================================

func TestBuildNoveltyGraph(t *testing.T) {
	compiled, err := BuildNoveltyGraph(mockProvider{})
	if err != nil {
		t.Fatalf("BuildNoveltyGraph failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("BuildNoveltyGraph returned nil")
	}
}

func TestBuildNoveltyGraph_Valid(t *testing.T) {
	compiled, err := BuildNoveltyGraph(mockProvider{})
	if err != nil {
		t.Fatalf("BuildNoveltyGraph failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("expected non-nil compiled graph")
	}
}

// =============================================================================
// Section 2: loadInputNode tests
// =============================================================================

func TestLoadInputNode_SkipEmpty(t *testing.T) {
	node := loadInputNode()
	state := graph.PregelState{}
	state, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("loadInputNode error: %v", err)
	}
	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	r := raw.(*NoveltyResult)
	if !r.Skipped {
		t.Error("expected Skipped=true")
	}
	if r.Assessed {
		t.Error("expected Assessed=false")
	}
}

func TestLoadInputNode_SkipNilInput(t *testing.T) {
	node := loadInputNode()
	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = (*NoveltyInput)(nil)
	state, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("loadInputNode error: %v", err)
	}
	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	r := raw.(*NoveltyResult)
	if !r.Skipped {
		t.Error("expected Skipped=true")
	}
}

func TestLoadInputNode_SkipNoEvidence(t *testing.T) {
	node := loadInputNode()
	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = &NoveltyInput{
		EvidenceCoverage: "none",
	}
	state, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("loadInputNode error: %v", err)
	}
	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	r := raw.(*NoveltyResult)
	if !r.Skipped {
		t.Error("expected Skipped=true")
	}
}

func TestLoadInputNode_SkipNoDocs(t *testing.T) {
	node := loadInputNode()
	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = &NoveltyInput{
		EvidenceCoverage: "full",
		PriorArtDocs:     nil,
		ConflictApps:     nil,
	}
	state, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("loadInputNode error: %v", err)
	}
	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		t.Fatal("expected result in state")
	}
	r := raw.(*NoveltyResult)
	if !r.Skipped {
		t.Error("expected Skipped=true for no prior art docs")
	}
}

func TestLoadInputNode_Valid(t *testing.T) {
	node := loadInputNode()
	state := graph.PregelState{}
	input := &NoveltyInput{
		Claims:           []ClaimText{{ID: "1", Text: "一种装置", Type: "independent"}},
		PriorArtDocs:     []PriorArtDoc{{DocID: "D1", Title: "对比文件1"}},
		FilingDate:       "2024-01-01",
		TechDomain:       "mechanical",
		EvidenceCoverage: "full",
	}
	state[StateKeyNoveltyInput] = input
	state, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("loadInputNode error: %v", err)
	}

	// Should NOT set result (conclusion node is responsible)
	if _, ok := state[StateKeyNoveltyResult]; ok {
		t.Error("expected no result for valid input")
	}

	// Input should still be in state
	stored := extractInput(state)
	if stored == nil {
		t.Fatal("expected stored input")
	}
	if stored.FilingDate != "2024-01-01" {
		t.Errorf("expected FilingDate=2024-01-01, got %s", stored.FilingDate)
	}
}

// =============================================================================
// Section 3: State helper tests
// =============================================================================

func TestStateHasSkip(t *testing.T) {
	tests := []struct {
		name   string
		state  graph.PregelState
		expect bool
	}{
		{"no result key", graph.PregelState{}, false},
		{"wrong type", graph.PregelState{StateKeyNoveltyResult: "string"}, false},
		{"not skipped", graph.PregelState{StateKeyNoveltyResult: &NoveltyResult{Skipped: false}}, false},
		{"skipped", graph.PregelState{StateKeyNoveltyResult: &NoveltyResult{Skipped: true}}, true},
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
	// Empty state
	if got := extractInput(graph.PregelState{}); got != nil {
		t.Error("expected nil for empty state")
	}

	// Wrong type
	state := graph.PregelState{StateKeyNoveltyInput: "string"}
	if got := extractInput(state); got != nil {
		t.Error("expected nil for wrong type")
	}

	// Valid input
	input := &NoveltyInput{FilingDate: "2024-06-01"}
	state = graph.PregelState{StateKeyNoveltyInput: input}
	if got := extractInput(state); got == nil {
		t.Error("expected non-nil input")
	} else if got.FilingDate != "2024-06-01" {
		t.Errorf("expected FilingDate=2024-06-01, got %s", got.FilingDate)
	}
}

// =============================================================================
// Section 4: JSON extraction tests
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
// Section 5: buildInputText tests
// =============================================================================

func TestBuildInputText(t *testing.T) {
	input := &NoveltyInput{
		FilingDate: "2024-01-01",
		TechDomain: "mechanical",
		Claims:     []ClaimText{{ID: "1", Text: "一种装置", Type: "independent"}},
		PriorArtDocs: []PriorArtDoc{
			{DocID: "D1", Title: "对比文件1", PubDate: "2023-06-01", PubType: "written", Snippet: "公开了一种装置"},
		},
	}
	text := buildInputText(input)
	required := []string{"2024-01-01", "一种装置", "对比文件1", "mechanical"}
	for _, term := range required {
		if !strings.Contains(text, term) {
			t.Errorf("buildInputText missing required term: %q", term)
		}
	}
}

func TestBuildInputText_Nil(t *testing.T) {
	if got := buildInputText(nil); got != "" {
		t.Errorf("expected empty string for nil input, got %q", got)
	}
}

// =============================================================================
// Section 6: Step parsing tests
// =============================================================================

func TestParsePriorArt(t *testing.T) {
	output := `{"is_publicly_known": true, "prior_art_type": "written", "disclosure_reason": "对比文件在申请日前公开出版"}`
	result := parsePriorArt(output)
	if !result.IsPubliclyKnown {
		t.Error("expected IsPubliclyKnown=true")
	}
	if result.PriorArtType != "written" {
		t.Errorf("expected PriorArtType=written, got %s", result.PriorArtType)
	}
}

func TestParsePriorArt_NoJSON(t *testing.T) {
	output := "该对比文件在申请日前已公开出版"
	result := parsePriorArt(output)
	if result.DisclosureReason != output {
		t.Error("expected raw output as DisclosureReason when no JSON")
	}
}

func TestParseCompare(t *testing.T) {
	output := `{"claim_features": ["A", "B", "C"], "disclosed_features": ["A", "B"], "missing_features": ["C"], "full_feature_coverage": false}`
	result := parseCompare(output)
	if len(result.ClaimFeatures) != 3 {
		t.Errorf("expected 3 claim features, got %d", len(result.ClaimFeatures))
	}
	if result.FullFeatureCoverage {
		t.Error("expected FullFeatureCoverage=false")
	}
}

func TestParseCompare_MissingFeatures(t *testing.T) {
	output := `{"claim_features": ["A"], "disclosed_features": ["A"], "missing_features": [], "full_feature_coverage": true}`
	result := parseCompare(output)
	if !result.FullFeatureCoverage {
		t.Error("expected FullFeatureCoverage=true")
	}
	if len(result.MissingFeatures) != 0 {
		t.Errorf("expected empty missing features, got %d", len(result.MissingFeatures))
	}
}

func TestParseConflict(t *testing.T) {
	output := `{"is_conflict_app": true, "conflict_reasons": ["在先申请日早于在后申请日", "在先申请公开日晚于在后申请日"], "full_content_compare": true, "conflict_doc_id": "CN2023100001"}`
	result := parseConflict(output)
	if !result.IsConflictApp {
		t.Error("expected IsConflictApp=true")
	}
	if len(result.ConflictReasons) != 2 {
		t.Errorf("expected 2 conflict reasons, got %d", len(result.ConflictReasons))
	}
}

func TestParseConflict_NotConflict(t *testing.T) {
	output := `{"is_conflict_app": false, "conflict_reasons": [], "full_content_compare": false}`
	result := parseConflict(output)
	if result.IsConflictApp {
		t.Error("expected IsConflictApp=false")
	}
}

func TestParseGracePriority(t *testing.T) {
	output := `{"has_grace_period": true, "grace_type": "exhibition", "grace_within_6m": true, "has_priority": false, "priority_valid": false}`
	result := parseGracePriority(output)
	if !result.HasGracePeriod {
		t.Error("expected HasGracePeriod=true")
	}
	if result.GraceType != "exhibition" {
		t.Errorf("expected GraceType=exhibition, got %s", result.GraceType)
	}
}

func TestParseGracePriority_NoGrace(t *testing.T) {
	output := `{"has_grace_period": false, "grace_type": "", "grace_within_6m": false, "has_priority": true, "priority_valid": true, "same_subject": "相同主题"}`
	result := parseGracePriority(output)
	if result.HasGracePeriod {
		t.Error("expected HasGracePeriod=false")
	}
	if !result.HasPriority {
		t.Error("expected HasPriority=true")
	}
}

func TestParseConclusion(t *testing.T) {
	output := `{"conclusion": "权利要求1-3具备新颖性", "has_novelty": true, "confidence": "high", "failed_claims": []}`
	result := parseConclusion(output)
	if !result.HasNovelty {
		t.Error("expected HasNovelty=true")
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence=high, got %s", result.Confidence)
	}
}

func TestParseConclusion_InvalidConfidence(t *testing.T) {
	output := `{"conclusion": "具备新颖性", "has_novelty": true, "confidence": "very_high", "failed_claims": []}`
	result := parseConclusion(output)
	if !result.HasNovelty {
		t.Error("expected HasNovelty=true")
	}
	if result.Confidence != "medium" {
		t.Errorf("expected confidence=medium (fallback), got %s", result.Confidence)
	}
}

func TestParseConclusion_NoJSON(t *testing.T) {
	output := "整体来看具备新颖性"
	result := parseConclusion(output)
	if result.Conclusion != output {
		t.Error("expected raw output as conclusion when no JSON")
	}
	if result.Confidence != "medium" {
		t.Errorf("expected confidence=medium (fallback), got %s", result.Confidence)
	}
}

// =============================================================================
// Section 7: buildResult tests
// =============================================================================

func TestBuildResult(t *testing.T) {
	priorArt := `{"is_publicly_known": true, "prior_art_type": "written", "disclosure_reason": "公开出版"}`
	compare := `{"claim_features": ["A", "B"], "disclosed_features": ["A"], "missing_features": ["B"], "full_feature_coverage": false}`
	conflict := `{"is_conflict_app": false, "full_content_compare": false}`
	gracePriority := `{"has_grace_period": false, "has_priority": false}`
	conclusion := `{"conclusion": "权利要求1具备新颖性", "has_novelty": true, "confidence": "medium", "failed_claims": []}`

	result := buildResult(priorArt, compare, conflict, gracePriority, conclusion)

	if !result.Assessed {
		t.Error("expected Assessed=true")
	}
	if !result.HasNovelty {
		t.Error("expected HasNovelty=true (from LLM conclusion)")
	}
	if result.Confidence != "medium" {
		t.Errorf("expected confidence=medium, got %s", result.Confidence)
	}
	if !result.PriorArtCheck.IsPubliclyKnown {
		t.Error("expected PriorArtCheck.IsPubliclyKnown=true")
	}
	if result.SingleCompare.FullFeatureCoverage {
		t.Error("expected SingleCompare.FullFeatureCoverage=false")
	}
}

func TestBuildResult_NoFeatureCoverage(t *testing.T) {
	// LLM 没有显式 hasNovelty → 自动计算
	priorArt := `{"is_publicly_known": true, "prior_art_type": "written", "disclosure_reason": ""}`
	compare := `{"claim_features": ["A", "B"], "disclosed_features": ["A"], "missing_features": ["B"], "full_feature_coverage": false}`
	conflict := `{"is_conflict_app": false, "full_content_compare": false}`
	gracePriority := `{"has_grace_period": false, "has_priority": false}`
	conclusion := `{"conclusion": "", "has_novelty": false, "confidence": "medium", "failed_claims": []}`

	result := buildResult(priorArt, compare, conflict, gracePriority, conclusion)

	if !result.HasNovelty {
		t.Error("expected HasNovelty=true (not full coverage and no conflict)")
	}
}

func TestBuildResult_FullCoverage(t *testing.T) {
	// 特征全部被公开 → HasNovelty=false
	priorArt := `{"is_publicly_known": true, "prior_art_type": "written", "disclosure_reason": ""}`
	compare := `{"claim_features": ["A", "B"], "disclosed_features": ["A", "B"], "missing_features": [], "full_feature_coverage": true}`
	conflict := `{"is_conflict_app": false, "full_content_compare": false}`
	gracePriority := `{"has_grace_period": false, "has_priority": false}`
	conclusion := `{"conclusion": "", "has_novelty": false, "confidence": "low", "failed_claims": ["1"]}`

	result := buildResult(priorArt, compare, conflict, gracePriority, conclusion)

	if result.HasNovelty {
		t.Error("expected HasNovelty=false (full coverage)")
	}
}

// =============================================================================
// Section 8: JSON Schema tests
// =============================================================================

func TestPriorArtSchema(t *testing.T) {
	schema := priorArtSchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if props["is_publicly_known"] == nil {
		t.Error("missing property: is_publicly_known")
	}
	if props["prior_art_type"] == nil {
		t.Error("missing property: prior_art_type")
	}
}

func TestCompareSchema(t *testing.T) {
	schema := compareSchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["claim_features"] == nil {
		t.Error("missing property: claim_features")
	}
	if props["full_feature_coverage"] == nil {
		t.Error("missing property: full_feature_coverage")
	}
}

func TestConflictSchema(t *testing.T) {
	schema := conflictSchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["is_conflict_app"] == nil {
		t.Error("missing property: is_conflict_app")
	}
}

func TestGracePrioritySchema(t *testing.T) {
	schema := gracePrioritySchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["has_grace_period"] == nil {
		t.Error("missing property: has_grace_period")
	}
	if props["has_priority"] == nil {
		t.Error("missing property: has_priority")
	}
}

func TestSpecialDomainSchema(t *testing.T) {
	schema := specialDomainSchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["affects_novelty"] == nil {
		t.Error("missing property: affects_novelty")
	}
}

func TestConclusionSchema(t *testing.T) {
	schema := conclusionSchema()
	if schema == nil {
		t.Fatal("nil schema")
	}
	props, _ := schema["properties"].(map[string]any)
	if props["has_novelty"] == nil {
		t.Error("missing property: has_novelty")
	}
	if props["failed_claims"] == nil {
		t.Error("missing property: failed_claims")
	}
}

// =============================================================================
// Section 9: Framework tests
// =============================================================================

func TestDefaultFramework(t *testing.T) {
	framework := NewFramework(nil)
	result := framework.GetArticleFramework()
	if result == "" {
		t.Fatal("default framework should not be empty")
	}
	if !strings.Contains(result, "A22.2") && !strings.Contains(result, "22条第2款") {
		t.Error("default framework should mention A22.2")
	}
}

func TestDefaultFramework_ContainsAllKeyTerms(t *testing.T) {
	fw := defaultA222Framework()
	requiredTerms := []string{
		"22条第2款", "新颖性", "现有技术", "为公众所知",
		"单独对比", "上位概念", "下位概念", "惯用手段",
		"数值范围", "抵触申请", "宽限期", "优先权",
		"本领域的技术人员", "hasNovelty", "confidence",
	}
	for _, term := range requiredTerms {
		if !strings.Contains(fw, term) {
			t.Errorf("default framework missing required term: %q", term)
		}
	}
}

func TestFrameworkAdapter_NilProvider(t *testing.T) {
	f := NewFramework(nil)
	result := f.GetArticleFramework()
	if !strings.Contains(result, "新颖性") {
		t.Error("nil provider should return default framework containing 新颖性")
	}
}

type mockArticleProvider struct {
	data map[string]ArticleFrameworkData
}

func (m mockArticleProvider) Article(id string) ArticleFrameworkData {
	if m.data != nil {
		return m.data[id]
	}
	return ArticleFrameworkData{}
}

func TestFrameworkAdapter_WithProvider(t *testing.T) {
	provider := mockArticleProvider{
		data: map[string]ArticleFrameworkData{
			"patent-law-a22.2": {
				Name:         "专利法第22条第2款——新颖性",
				LawRef:       "专利法第22条第2款",
				GuidelineRef: "审查指南第二部分第三章",
				Steps: []ArticleStepData{
					{Order: 1, Name: "确定申请日", InputHint: "申请日"},
				},
				ConclusionSchema: map[string]string{"hasNovelty": "bool"},
				ApplicableTo:     []string{"novelty"},
			},
		},
	}
	f := NewFramework(provider)
	result := f.GetArticleFramework()
	if !strings.Contains(result, "专利法第22条第2款——新颖性") {
		t.Error("expected provider data")
	}
	if !strings.Contains(result, "确定申请日") {
		t.Error("expected steps from provider")
	}
}

func TestFrameworkAdapter_FallbackToDefault(t *testing.T) {
	provider := mockArticleProvider{
		data: map[string]ArticleFrameworkData{},
	}
	f := NewFramework(provider)
	result := f.GetArticleFramework()
	if !strings.Contains(result, "新颖性") {
		t.Error("unknown article ID should fallback to default")
	}
}

func TestFormatArticleData(t *testing.T) {
	af := ArticleFrameworkData{
		Name:         "测试框架",
		LawRef:       "测试法条",
		Steps:        []ArticleStepData{{Order: 1, Name: "测试步骤", InputHint: "测试输入"}},
		ApplicableTo: []string{"test"},
	}
	result := formatArticleData(af)
	if !strings.Contains(result, "测试框架") {
		t.Error("expected formatted name")
	}
	if !strings.Contains(result, "第 1 步") {
		t.Error("expected step numbering")
	}
	if !strings.Contains(result, "test") {
		t.Error("expected applicableTo")
	}
}

// =============================================================================
// Section 10: Tool tests
// =============================================================================

func TestNewNoveltyTool_NoProvider(t *testing.T) {
	tool := NewNoveltyTool()
	if tool.Name != "evaluate_novelty" {
		t.Errorf("expected tool name evaluate_novelty, got %s", tool.Name)
	}
	if !tool.ReadOnly {
		t.Error("expected ReadOnly=true")
	}
}

func TestParseNoveltyArgs_ValidJSON(t *testing.T) {
	args := json.RawMessage(`{
		"claims": [{"id": "1", "text": "一种装置", "type": "independent"}],
		"prior_art_docs": [{"doc_id": "D1", "title": "对比文件"}],
		"filing_date": "2024-01-01",
		"tech_domain": "mechanical"
	}`)
	input := parseNoveltyArgs(args)
	if input == nil {
		t.Fatal("expected non-nil input")
	}
	if len(input.Claims) != 1 {
		t.Errorf("expected 1 claim, got %d", len(input.Claims))
	}
	if input.FilingDate != "2024-01-01" {
		t.Errorf("expected FilingDate=2024-01-01, got %s", input.FilingDate)
	}
}

func TestParseNoveltyArgs_InvalidJSON(t *testing.T) {
	args := json.RawMessage(`invalid json`)
	input := parseNoveltyArgs(args)
	if input != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestParseNoveltyArgs_EmptyObject(t *testing.T) {
	args := json.RawMessage(`{}`)
	input := parseNoveltyArgs(args)
	if input == nil {
		t.Fatal("expected non-nil input for empty object")
	}
	if len(input.Claims) != 0 {
		t.Errorf("expected 0 claims, got %d", len(input.Claims))
	}
}

// =============================================================================
// Section 11: Constants and person-skilled definition tests
// =============================================================================

func TestStateKeyConstants(t *testing.T) {
	if StateKeyNoveltyInput != "novelty_input" {
		t.Errorf("unexpected StateKeyNoveltyInput: %s", StateKeyNoveltyInput)
	}
	if StateKeyNoveltyResult != "novelty_result" {
		t.Errorf("unexpected StateKeyNoveltyResult: %s", StateKeyNoveltyResult)
	}
}

func TestPersonSkilledDefinition(t *testing.T) {
	def := personSkilledDefinition()
	requiredTerms := []string{"本领域的技术人员", "普通技术知识", "创造能力", "常规实验"}
	for _, term := range requiredTerms {
		if !strings.Contains(def, term) {
			t.Errorf("personSkilledDefinition missing term: %q", term)
		}
	}
}

// =============================================================================
// Section 12: Type JSON tag validation
// =============================================================================

func TestNoveltyInput_JSONTags(t *testing.T) {
	input := NoveltyInput{
		Claims:           []ClaimText{{ID: "1", Text: "装置", Type: "independent"}},
		PriorArtDocs:     []PriorArtDoc{{DocID: "D1"}},
		FilingDate:       "2024-01-01",
		TechDomain:       "mechanical",
		EvidenceCoverage: "full",
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["filing_date"] == nil {
		t.Error("expected snake_case JSON tag 'filing_date'")
	}
	if decoded["prior_art_docs"] == nil {
		t.Error("expected snake_case JSON tag 'prior_art_docs'")
	}
	if decoded["evidence_coverage"] == nil {
		t.Error("expected snake_case JSON tag 'evidence_coverage'")
	}
}

func TestNoveltyResult_JSONTags(t *testing.T) {
	result := NoveltyResult{
		Assessed:   true,
		HasNovelty: true,
		Conclusion: "具备新颖性",
		Confidence: "high",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["has_novelty"] == nil {
		t.Error("expected snake_case 'has_novelty'")
	}
	if decoded["prior_art_check"] == nil {
		t.Error("expected snake_case 'prior_art_check'")
	}
	if decoded["single_compare"] == nil {
		t.Error("expected snake_case 'single_compare'")
	}
}
