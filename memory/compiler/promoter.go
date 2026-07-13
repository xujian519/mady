package compiler

import (
	"fmt"
	"time"
)

// RuleRegistrar is the callback that registers a promoted rule candidate
// into the target rule system (e.g., workflows/patent.RuleEngine).
// The caller is responsible for converting RuleCandidate fields into
// the target rule format — this bridge is NOT automated because
// Strategy.Guidance (prompt text) and CheckRule (structured legal rule)
// have no direct mapping.
type RuleRegistrar func(candidate RuleCandidate) error

// PromotionLog records a single promotion event for audit trail.
type PromotionLog struct {
	CandidateID string    `json:"candidate_id"`
	StrategyID  string    `json:"strategy_id"`
	SuccessRate float64   `json:"success_rate"`
	Samples     int       `json:"samples"`
	PromotedAt  time.Time `json:"promoted_at"`
	Note        string    `json:"note,omitempty"`
}

// RulePromoter orchestrates the final promotion of approved candidates
// into the live rule system. It enforces the promotion gate one last
// time before calling the registrar.
type RulePromoter struct {
	gate      *RulePromotionGate
	registrar RuleRegistrar
	logs      []PromotionLog
}

// NewRulePromoter creates a promoter with the given gate and registrar.
// The registrar must not be nil — promotion without a registration
// callback is meaningless.
func NewRulePromoter(gate *RulePromotionGate, registrar RuleRegistrar) *RulePromoter {
	if gate == nil {
		gate = NewRulePromotionGate(DefaultPromotionGateConfig())
	}
	if registrar == nil {
		registrar = func(_ RuleCandidate) error {
			return fmt.Errorf("未配置规则注册器")
		}
	}
	return &RulePromoter{
		gate:      gate,
		registrar: registrar,
	}
}

// Promote checks the promotion gate and, if all requirements are met,
// calls the registrar to register the rule. Returns an error if the
// gate rejects the candidate or the registrar fails.
func (p *RulePromoter) Promote(c RuleCandidate) error {
	result := p.gate.Evaluate(c)
	if !result.Ready {
		return fmt.Errorf("晋升门控未通过: %v", result.Reasons)
	}

	if err := p.registrar(c); err != nil {
		return fmt.Errorf("规则注册失败: %w", err)
	}

	p.logs = append(p.logs, PromotionLog{
		CandidateID: c.ID,
		StrategyID:  c.StrategyID,
		SuccessRate: c.SuccessRate,
		Samples:     c.Samples,
		PromotedAt:  time.Now(),
		Note:        c.ReviewerNote,
	})

	return nil
}

// PromoteBatch promotes all approved candidates from a ReviewQueue.
// Returns the number of successfully promoted rules and any errors
// encountered. A candidate that fails promotion does not block
// subsequent candidates.
func (p *RulePromoter) PromoteBatch(queue *ReviewQueue) (int, []error) {
	approved := queue.DrainApproved()
	promoted := 0
	var errs []error

	for _, c := range approved {
		if err := p.Promote(c); err != nil {
			errs = append(errs, fmt.Errorf("候选 %s: %w", c.ID, err))
			continue
		}
		promoted++
	}

	return promoted, errs
}

// Logs returns a snapshot of all promotion events.
func (p *RulePromoter) Logs() []PromotionLog {
	out := make([]PromotionLog, len(p.logs))
	copy(out, p.logs)
	return out
}

// PromoteFromCompiler is a convenience pipeline that extracts candidates
// from a Compiler, enqueues them into a ReviewQueue, and returns the
// queue for human review. The caller is expected to run ReviewSession
// on each candidate before calling PromoteBatch.
func PromoteFromCompiler(c *Compiler, queue *ReviewQueue, minSamples int, minSuccessRate float64) int {
	extractor := NewRuleCandidateExtractor(minSamples, minSuccessRate)
	candidates := extractor.ExtractFromCompiler(c)
	return queue.Enqueue(candidates...)
}
