package smartrouter

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// mockProvider is a minimal agentcore.Provider for testing.
type mockProvider struct {
	name     string
	response *agentcore.ProviderResponse
	err      error
	gotModel []string // models received across calls
}

func (m *mockProvider) Complete(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	m.gotModel = append(m.gotModel, req.Model)
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockProvider) Stream(_ context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	m.gotModel = append(m.gotModel, req.Model)
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Content: "delta", Done: true}
	close(ch)
	return ch, nil
}

func simpleReq(text string) *agentcore.ProviderRequest {
	return &agentcore.ProviderRequest{
		Model: "default",
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: text},
		},
	}
}

func profile(name string, strengths []TaskType, quality, cost, latency float64, p agentcore.Provider) ModelProfile {
	return ModelProfile{
		Name: name, Provider: p, Strengths: strengths,
		QualityScore: quality, CostPerMTokens: cost, LatencyMs: latency,
	}
}

// ---------------------------------------------------------------------------
// DefaultClassifier
// ---------------------------------------------------------------------------

func TestDefaultClassifier_Classify(t *testing.T) {
	c := &DefaultClassifier{}
	cases := []struct {
		name string
		text string
		want TaskType
	}{
		{"patent", "请分析这件专利的权利要求和新颖性", TaskPatent},
		{"legal", "这是一个合同纠纷案件，涉及违约", TaskLegal},
		{"coding", "帮我重构这个函数，修复bug", TaskCoding},
		{"general", "你好，今天天气怎么样", TaskGeneral},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Classify(simpleReq(tc.text))
			if got != tc.want {
				t.Errorf("Classify(%q) = %s, want %s", tc.text, got, tc.want)
			}
		})
	}
}

func TestDefaultClassifier_NilRequest(t *testing.T) {
	c := &DefaultClassifier{}
	if got := c.Classify(nil); got != TaskGeneral {
		t.Errorf("Classify(nil) = %s, want %s", got, TaskGeneral)
	}
}

// ---------------------------------------------------------------------------
// Route
// ---------------------------------------------------------------------------

func TestSmartRouter_Route_SelectsStrength(t *testing.T) {
	legalP := &mockProvider{name: "legal-model"}
	codeP := &mockProvider{name: "code-model"}
	profiles := []ModelProfile{
		profile("code-model", []TaskType{TaskCoding}, 0.9, 1.0, 100, codeP),
		profile("legal-model", []TaskType{TaskLegal}, 0.9, 10.0, 500, legalP),
	}
	s := New(profiles)
	dec := s.Route(simpleReq("合同纠纷违约诉讼"))
	if dec.Profile.Name != "legal-model" {
		t.Errorf("Route selected %s, want legal-model", dec.Profile.Name)
	}
	if dec.TaskType != TaskLegal {
		t.Errorf("TaskType = %s, want %s", dec.TaskType, TaskLegal)
	}
}

func TestSmartRouter_Route_FallbackToGeneral(t *testing.T) {
	genP := &mockProvider{name: "gen-model"}
	profiles := []ModelProfile{
		profile("gen-model", []TaskType{TaskGeneral}, 0.5, 2.0, 200, genP),
	}
	s := New(profiles)
	// Patent request but only a general profile exists → should still route.
	dec := s.Route(simpleReq("专利权利要求新颖性"))
	if dec.Profile.Name != "gen-model" {
		t.Errorf("Route selected %s, want gen-model (general fallback)", dec.Profile.Name)
	}
}

func TestSmartRouter_PriorityQuality(t *testing.T) {
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	profiles := []ModelProfile{
		profile("p1", []TaskType{TaskGeneral}, 0.8, 5.0, 300, p1),
		profile("p2", []TaskType{TaskGeneral}, 0.9, 8.0, 400, p2),
	}
	s := New(profiles, WithPriority(PriorityQuality))
	dec := s.Route(simpleReq("hello"))
	if dec.Profile.Name != "p2" {
		t.Errorf("quality priority selected %s, want p2 (higher QualityScore)", dec.Profile.Name)
	}
}

func TestSmartRouter_PriorityCost(t *testing.T) {
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	profiles := []ModelProfile{
		profile("p1", []TaskType{TaskGeneral}, 0.8, 5.0, 300, p1),
		profile("p2", []TaskType{TaskGeneral}, 0.9, 2.0, 400, p2),
	}
	s := New(profiles, WithPriority(PriorityCost))
	dec := s.Route(simpleReq("hello"))
	if dec.Profile.Name != "p2" {
		t.Errorf("cost priority selected %s, want p2 (lower cost)", dec.Profile.Name)
	}
}

// ---------------------------------------------------------------------------
// Complete (execution)
// ---------------------------------------------------------------------------

func TestSmartRouter_Complete_SetsModel(t *testing.T) {
	p := &mockProvider{name: "legal-model", response: &agentcore.ProviderResponse{Content: "ok"}}
	profiles := []ModelProfile{
		profile("legal-model", []TaskType{TaskLegal}, 0.9, 10.0, 500, p),
	}
	s := New(profiles)
	resp, err := s.Complete(context.Background(), simpleReq("合同诉讼"))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Content)
	}
	if len(p.gotModel) != 1 || p.gotModel[0] != "legal-model" {
		t.Errorf("backend got model %v, want [legal-model]", p.gotModel)
	}
}

func TestSmartRouter_Complete_Fallback(t *testing.T) {
	failP := &mockProvider{name: "fail-model", err: errors.New("backend down")}
	okP := &mockProvider{name: "ok-model", response: &agentcore.ProviderResponse{Content: "recovered"}}
	profiles := []ModelProfile{
		profile("fail-model", []TaskType{TaskGeneral}, 0.9, 5.0, 300, failP),
		profile("ok-model", []TaskType{TaskGeneral}, 0.7, 2.0, 200, okP),
	}
	// PriorityQuality so fail-model (0.9) ranks first, triggering fallback to ok-model.
	s := New(profiles, EnableFallback(), WithPriority(PriorityQuality))
	resp, err := s.Complete(context.Background(), simpleReq("hello"))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Content != "recovered" {
		t.Errorf("Content = %q, want recovered (from fallback)", resp.Content)
	}
	if len(failP.gotModel) != 1 {
		t.Errorf("fail provider should be called once, got %d", len(failP.gotModel))
	}
	if len(okP.gotModel) != 1 {
		t.Errorf("ok provider should be called once (fallback), got %d", len(okP.gotModel))
	}
}

func TestSmartRouter_Complete_NoFallback(t *testing.T) {
	failP := &mockProvider{name: "fail-model", err: errors.New("backend down")}
	okP := &mockProvider{name: "ok-model", response: &agentcore.ProviderResponse{Content: "ok"}}
	profiles := []ModelProfile{
		profile("fail-model", []TaskType{TaskGeneral}, 0.9, 5.0, 300, failP),
		profile("ok-model", []TaskType{TaskGeneral}, 0.7, 2.0, 200, okP),
	}
	// PriorityQuality ranks fail-model first; without fallback, its error propagates.
	s := New(profiles, WithPriority(PriorityQuality))
	_, err := s.Complete(context.Background(), simpleReq("hello"))
	if err == nil {
		t.Error("expected error when fallback disabled and first backend fails")
	}
	if len(okP.gotModel) != 0 {
		t.Errorf("ok provider should NOT be called without fallback, got %d calls", len(okP.gotModel))
	}
}

// ---------------------------------------------------------------------------
// Stream
// ---------------------------------------------------------------------------

func TestSmartRouter_Stream(t *testing.T) {
	p := &mockProvider{name: "gen-model"}
	profiles := []ModelProfile{
		profile("gen-model", []TaskType{TaskGeneral}, 0.5, 2.0, 200, p),
	}
	s := New(profiles)
	ch, err := s.Stream(context.Background(), simpleReq("hello"))
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var got string
	for delta := range ch {
		got += delta.Content
	}
	if got != "delta" {
		t.Errorf("stream content = %q, want delta", got)
	}
	if len(p.gotModel) != 1 || p.gotModel[0] != "gen-model" {
		t.Errorf("backend got model %v, want [gen-model]", p.gotModel)
	}
}

// ---------------------------------------------------------------------------
// History + LastDecision
// ---------------------------------------------------------------------------

func TestSmartRouter_History(t *testing.T) {
	p := &mockProvider{name: "legal-model", response: &agentcore.ProviderResponse{Content: "ok"}}
	hist := NewRouteHistory()
	profiles := []ModelProfile{
		profile("legal-model", []TaskType{TaskLegal}, 0.9, 10.0, 500, p),
	}
	s := New(profiles, WithHistory(hist))
	_, _ = s.Complete(context.Background(), simpleReq("合同诉讼"))

	records := hist.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(records))
	}
	if !records[0].Success {
		t.Errorf("record Success = false, want true")
	}
	if records[0].Profile != "legal-model" {
		t.Errorf("record Profile = %s, want legal-model", records[0].Profile)
	}
	if records[0].TaskType != TaskLegal {
		t.Errorf("record TaskType = %s, want %s", records[0].TaskType, TaskLegal)
	}

	stats := hist.Stats()
	if stats[TaskLegal]["legal-model:success"] != 1 {
		t.Errorf("stats = %v, want legal-model:success=1", stats)
	}
}

func TestSmartRouter_LastDecision(t *testing.T) {
	profiles := []ModelProfile{
		profile("gen-model", []TaskType{TaskGeneral}, 0.5, 2.0, 200, &mockProvider{name: "gen-model"}),
	}
	s := New(profiles)
	if d := s.LastDecision(); d != nil {
		t.Error("LastDecision should be nil before any route")
	}
	s.Route(simpleReq("hello"))
	d := s.LastDecision()
	if d == nil {
		t.Fatal("LastDecision should be non-nil after Route")
	}
	if d.Profile.Name != "gen-model" {
		t.Errorf("LastDecision Profile = %s, want gen-model", d.Profile.Name)
	}
	if d.Priority != PriorityBalanced {
		t.Errorf("LastDecision Priority = %s, want %s", d.Priority, PriorityBalanced)
	}
}

// ---------------------------------------------------------------------------
// hasStrength
// ---------------------------------------------------------------------------

func TestModelProfile_HasStrength(t *testing.T) {
	p := ModelProfile{Strengths: []TaskType{TaskLegal}}
	if !p.hasStrength(TaskLegal) {
		t.Error("legal strength should match")
	}
	if p.hasStrength(TaskCoding) {
		t.Error("coding should not match legal-only profile")
	}
	gen := ModelProfile{Strengths: []TaskType{TaskGeneral}}
	if !gen.hasStrength(TaskPatent) {
		t.Error("general profile should match any task type")
	}
}
