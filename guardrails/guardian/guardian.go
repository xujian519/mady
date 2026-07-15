package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// Session is a long-lived Guardian reviewer that uses a dedicated Provider
// to evaluate tool calls. It maintains a single conversation prefix for
// cache efficiency and tracks denial patterns via a circuit breaker.
type Session struct {
	provider agentcore.Provider
	model    string
	policy   string
	level    ReviewLevel
	breaker  CircuitBreaker

	mu           sync.Mutex
	reviewCount  int
	lastReviewAt time.Time
}

// Config configures a Guardian Session.
type Config struct {
	Provider agentcore.Provider
	Model    string // model name for review calls (empty = provider default)
	Policy   string // safety policy prompt (empty = PatentLegalPolicy)
	Level    ReviewLevel
}

// NewSession creates a Guardian session with the given configuration.
func NewSession(cfg Config) *Session {
	if cfg.Policy == "" {
		cfg.Policy = PatentLegalPolicy
	}
	return &Session{
		provider: cfg.Provider,
		model:    cfg.Model,
		policy:   cfg.Policy,
		level:    cfg.Level,
	}
}

// shouldReview determines whether a tool call needs Guardian review.
func (s *Session) shouldReview(toolName string, readOnly bool) bool {
	if s.level == ReviewOff {
		return false
	}
	if readOnly {
		return false
	}
	if s.level == ReviewHighRisk {
		return HighRiskTools[toolName]
	}
	return true // ReviewAllWriters
}

// Review evaluates a pending tool call and returns the assessment.
// On error, the Guardian fails closed (deny).
func (s *Session) Review(
	ctx context.Context,
	toolName string,
	args json.RawMessage,
	transcript string,
) (Assessment, error) {
	s.mu.Lock()
	s.reviewCount++
	s.lastReviewAt = time.Now()

	// Circuit breaker check
	if !s.breaker.Allow() {
		s.mu.Unlock()
		return Assessment{
			Outcome:   OutcomeDeny,
			RiskLevel: RiskHigh,
			Rationale: "熔断器已触发：连续多次拒绝，自动阻断",
		}, nil
	}
	s.mu.Unlock()

	if s.provider == nil {
		return Assessment{Outcome: OutcomeAllow, RiskLevel: RiskLow,
			Rationale: "no provider configured"}, nil
	}

	// Build the review prompt
	prompt := s.buildReviewPrompt(toolName, args, transcript)

	reviewCtx, cancel := context.WithTimeout(ctx, reviewTimeout)
	defer cancel()

	req := &agentcore.ProviderRequest{
		Model: s.model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: s.policy},
			{Role: agentcore.RoleUser, Content: prompt},
		},
		Temperature: 0,
	}

	resp, err := s.provider.Complete(reviewCtx, req)
	if err != nil {
		s.breaker.RecordDenial()
		return Assessment{}, fmt.Errorf("guardian review failed: %w", err)
	}

	assessment := parseAssessment(resp.Content)

	s.mu.Lock()
	if assessment.IsDenied() {
		s.breaker.RecordDenial()
	} else {
		s.breaker.RecordAllow()
	}
	s.mu.Unlock()

	return assessment, nil
}

// buildReviewPrompt constructs the user message for the review LLM call.
func (s *Session) buildReviewPrompt(toolName string, args json.RawMessage, transcript string) string {
	var sb strings.Builder
	sb.WriteString("请评估以下工具调用是否安全：\n\n")
	fmt.Fprintf(&sb, "工具: %s\n", toolName)

	if len(args) > 0 {
		var pretty map[string]any
		if err := json.Unmarshal(args, &pretty); err == nil {
			formatted, jsonErr := json.MarshalIndent(pretty, "", "  ")
			if jsonErr != nil {
				slog.Default().Warn("guardian: failed to marshal args for review prompt", "err", jsonErr)
				formatted = []byte(string(args))
			}
			fmt.Fprintf(&sb, "参数: %s\n", string(formatted))
		} else {
			fmt.Fprintf(&sb, "参数: %s\n", string(args))
		}
	}

	if transcript != "" {
		maxLen := 2000
		t := transcript
		if len(t) > maxLen {
			t = t[:maxLen] + "\n[...已截断...]"
		}
		fmt.Fprintf(&sb, "\n近期对话上下文:\n%s\n", t)
	}

	sb.WriteString("\n请输出 JSON 评估结果。")
	return sb.String()
}

// BreakerStats returns current circuit breaker statistics.
func (s *Session) BreakerStats() BreakerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.breaker.Stats()
}

// ReviewCount returns the total number of reviews performed.
func (s *Session) ReviewCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reviewCount
}

// ResetBreaker manually resets the circuit breaker.
func (s *Session) ResetBreaker() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breaker.Reset()
}
