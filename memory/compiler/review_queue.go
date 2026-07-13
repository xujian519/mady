package compiler

import (
	"fmt"
	"sync"
	"time"
)

// ShadowEvalResult records the outcome of a shadow evaluation run.
type ShadowEvalResult struct {
	Passed bool      `json:"passed"`
	Score  float64   `json:"score"`
	Detail string    `json:"detail,omitempty"`
	RunAt  time.Time `json:"run_at"`
}

// ShadowEvalFunc is the evaluation function injected by the caller.
// It receives the candidate and returns a pass/fail verdict with a score.
// The caller is responsible for wiring this to the evaluation suite
// (e.g., agentcore/evaluate/benchmark.RunStatic).
type ShadowEvalFunc func(candidate RuleCandidate) (ShadowEvalResult, error)

// ReviewQueue manages the lifecycle of rule candidates awaiting human review.
// It is thread-safe and uses an in-memory slice; persistence is the
// caller's responsibility (e.g., via SQLiteMemoryStore).
type ReviewQueue struct {
	mu         sync.Mutex
	candidates []RuleCandidate
	shadowFn   ShadowEvalFunc
}

// NewReviewQueue creates an empty review queue with an optional shadow
// evaluation function. If shadowFn is nil, shadow evaluation is skipped
// and candidates must be marked manually via MarkShadowResult.
func NewReviewQueue(shadowFn ShadowEvalFunc) *ReviewQueue {
	return &ReviewQueue{
		shadowFn: shadowFn,
	}
}

// Enqueue adds candidates to the review queue.
func (q *ReviewQueue) Enqueue(candidates ...RuleCandidate) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	added := 0
	for _, c := range candidates {
		if c.Status == CandidateDraft {
			q.candidates = append(q.candidates, c)
			added++
		}
	}
	return added
}

// Dequeue removes and returns the next candidate awaiting review.
// Returns false if the queue is empty.
func (q *ReviewQueue) Dequeue() (RuleCandidate, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.candidates) == 0 {
		return RuleCandidate{}, false
	}
	c := q.candidates[0]
	q.candidates = q.candidates[1:]
	return c, true
}

// Pending returns the number of candidates awaiting review.
func (q *ReviewQueue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.candidates)
}

// List returns a snapshot of all candidates currently in the queue.
func (q *ReviewQueue) List() []RuleCandidate {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]RuleCandidate, len(q.candidates))
	copy(out, q.candidates)
	return out
}

// RunShadowEval runs the shadow evaluation function on a candidate
// and marks the result. Returns an error if no shadow function is
// configured.
func (q *ReviewQueue) RunShadowEval(c *RuleCandidate) error {
	q.mu.Lock()
	fn := q.shadowFn
	q.mu.Unlock()

	if fn == nil {
		return fmt.Errorf("影子评估函数未配置")
	}

	result, err := fn(*c)
	if err != nil {
		return fmt.Errorf("影子评估失败: %w", err)
	}

	c.MarkShadowResult(result.Passed)
	return nil
}

// ReviewSession orchestrates the review of a single candidate:
// shadow eval (if configured) → human review → promotion gate check.
// The humanReview parameter controls the approval decision.
// Returns the promotion result and any error from shadow evaluation.
func (q *ReviewQueue) ReviewSession(c *RuleCandidate, approved bool, note string) (PromotionResult, error) {
	if q.shadowFn != nil {
		if err := q.RunShadowEval(c); err != nil {
			return PromotionResult{}, err
		}
	}

	c.MarkHumanApproval(approved, note)

	gate := NewRulePromotionGate(DefaultPromotionGateConfig())
	return gate.Evaluate(*c), nil
}

// DrainApproved returns all candidates that have been approved and
// removes them from the queue. Candidates still in draft status
// (not yet reviewed) remain in the queue.
func (q *ReviewQueue) DrainApproved() []RuleCandidate {
	q.mu.Lock()
	defer q.mu.Unlock()

	var approved []RuleCandidate
	var remaining []RuleCandidate
	for _, c := range q.candidates {
		if c.Status == CandidateApproved || c.HumanApproved {
			approved = append(approved, c)
		} else {
			remaining = append(remaining, c)
		}
	}
	q.candidates = remaining
	return approved
}
