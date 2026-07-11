package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// mockProvider returns a predetermined response.
type mockProvider struct {
	content string
	err     error
	calls   int64
}

func (m *mockProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	atomic.AddInt64(&m.calls, 1)
	if m.err != nil {
		return nil, m.err
	}
	return &agentcore.ProviderResponse{Content: m.content}, nil
}

func (m *mockProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, nil
}

func TestParseAssessment_JSON(t *testing.T) {
	tests := []struct {
		input   string
		outcome Outcome
		risk    RiskLevel
	}{
		{`{"risk_level":"high","outcome":"deny","rationale":"dangerous"}`, OutcomeDeny, RiskHigh},
		{`{"risk_level":"low","outcome":"allow","rationale":"safe"}`, OutcomeAllow, RiskLow},
		{`{"risk_level":"medium","outcome":"allow","rationale":"ok"}`, OutcomeAllow, RiskMedium},
	}
	for _, tt := range tests {
		a := parseAssessment(tt.input)
		if a.Outcome != tt.outcome {
			t.Errorf("outcome=%s want %s (input=%s)", a.Outcome, tt.outcome, tt.input)
		}
		if a.RiskLevel != tt.risk {
			t.Errorf("risk=%s want %s (input=%s)", a.RiskLevel, tt.risk, tt.input)
		}
	}
}

func TestParseAssessment_MarkdownBlock(t *testing.T) {
	input := "Here is the assessment:\n```json\n{\"outcome\":\"deny\",\"risk_level\":\"high\",\"rationale\":\"test\"}\n```\n"
	a := parseAssessment(input)
	if a.Outcome != OutcomeDeny {
		t.Errorf("outcome=%s want deny", a.Outcome)
	}
}

func TestParseAssessment_Fallback(t *testing.T) {
	a := parseAssessment("I recommend to deny this action")
	if a.Outcome != OutcomeDeny {
		t.Errorf("expected deny from keyword fallback, got %s", a.Outcome)
	}

	a = parseAssessment("This looks safe to proceed")
	if a.Outcome != OutcomeAllow {
		t.Errorf("expected allow from keyword fallback, got %s", a.Outcome)
	}
}

func TestCircuitBreaker(t *testing.T) {
	var cb CircuitBreaker

	for i := 0; i < maxConsecutiveDenials; i++ {
		if !cb.Allow() {
			t.Fatalf("breaker tripped too early at iteration %d", i)
		}
		cb.RecordDenial()
	}

	if !cb.IsTripped() {
		t.Error("breaker should be tripped after max consecutive denials")
	}
	if cb.Allow() {
		t.Error("breaker should not allow when tripped")
	}

	cb.Reset()
	if !cb.Allow() {
		t.Error("breaker should allow after reset")
	}
}

func TestCircuitBreaker_AllowResets(t *testing.T) {
	var cb CircuitBreaker
	cb.RecordDenial()
	cb.RecordDenial()
	cb.RecordAllow() // resets consecutive count
	if cb.IsTripped() {
		t.Error("breaker should not trip after an allow resets consecutive count")
	}
}

func TestSession_ShouldReview(t *testing.T) {
	s := NewSession(Config{Level: ReviewAllWriters})
	if s.shouldReview("read", true) {
		t.Error("read-only should not be reviewed")
	}
	if !s.shouldReview("edit", false) {
		t.Error("writer should be reviewed in ReviewAllWriters mode")
	}

	s.level = ReviewHighRisk
	if s.shouldReview("edit", false) {
		t.Error("edit should not be reviewed in ReviewHighRisk mode")
	}
	if !s.shouldReview("delete", false) {
		t.Error("delete should be reviewed in ReviewHighRisk mode")
	}

	s.level = ReviewOff
	if s.shouldReview("delete", false) {
		t.Error("nothing should be reviewed when off")
	}
}

func TestSession_Review_Allow(t *testing.T) {
	mp := &mockProvider{
		content: `{"outcome":"allow","risk_level":"low","rationale":"safe operation"}`,
	}
	s := NewSession(Config{Provider: mp, Model: "test-model"})

	assessment, err := s.Review(context.Background(), "edit", json.RawMessage(`{"path":"/tmp/test.go"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if assessment.IsDenied() {
		t.Error("expected allow")
	}
}

func TestSession_Review_Deny(t *testing.T) {
	mp := &mockProvider{
		content: `{"outcome":"deny","risk_level":"high","rationale":"destructive operation"}`,
	}
	s := NewSession(Config{Provider: mp})

	assessment, err := s.Review(context.Background(), "delete", json.RawMessage(`{"path":"/important.go"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.IsDenied() {
		t.Error("expected deny")
	}
	if assessment.Rationale != "destructive operation" {
		t.Errorf("rationale=%q want 'destructive operation'", assessment.Rationale)
	}
}

func TestSession_Review_FailClosed(t *testing.T) {
	mp := &mockProvider{err: fmt.Errorf("network error")}
	s := NewSession(Config{Provider: mp})

	_, err := s.Review(context.Background(), "delete", json.RawMessage(`{}`), "")
	if err == nil {
		t.Error("expected error on review failure")
	}
}

func TestSession_CircuitBreakerTrips(t *testing.T) {
	mp := &mockProvider{
		content: `{"outcome":"deny","risk_level":"high","rationale":"no"}`,
	}
	s := NewSession(Config{Provider: mp})

	for i := 0; i < maxConsecutiveDenials; i++ {
		_, _ = s.Review(context.Background(), "delete", json.RawMessage(`{}`), "")
	}

	stats := s.BreakerStats()
	if !stats.Tripped {
		t.Error("breaker should be tripped")
	}

	// Next review should auto-deny without calling provider
	callsBefore := atomic.LoadInt64(&mp.calls)
	assessment, _ := s.Review(context.Background(), "delete", json.RawMessage(`{}`), "")
	if !assessment.IsDenied() {
		t.Error("expected auto-deny when breaker tripped")
	}
	callsAfter := atomic.LoadInt64(&mp.calls)
	if callsAfter != callsBefore {
		t.Error("provider should not be called when breaker is tripped")
	}
}

func TestSession_NoProvider(t *testing.T) {
	s := NewSession(Config{})
	assessment, err := s.Review(context.Background(), "edit", json.RawMessage(`{}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if assessment.IsDenied() {
		t.Error("no provider should default to allow")
	}
}

func TestFormatTranscript(t *testing.T) {
	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "delete this file"},
		{Role: agentcore.RoleAssistant, Content: "I'll help you with that"},
	}
	result := FormatTranscript(msgs, 5)
	if result == "" {
		t.Error("expected non-empty transcript")
	}
	if !contains(result, "delete this file") {
		t.Error("transcript should contain user message")
	}
}

func TestFormatTranscript_Truncation(t *testing.T) {
	msgs := make([]agentcore.Message, 10)
	for i := range msgs {
		msgs[i] = agentcore.Message{Role: agentcore.RoleUser, Content: fmt.Sprintf("msg %d", i)}
	}
	result := FormatTranscript(msgs, 3)
	if !contains(result, "msg 9") {
		t.Error("should contain last message")
	}
	if contains(result, "msg 0") {
		t.Error("should not contain first message when truncated")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
