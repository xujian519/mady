package wiring

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/reasoning"
)

// RuleAssertionHook is an AfterModelCall lifecycle hook that checks whether
// the model's output references all Must-tier (ReqMust) confirmed rules.
//
// It implements the "soft constraint" variant of design-rule-acquisition-
// stage.md §5 Execute: rather than blocking execution (AfterModelCall cannot
// return an error), it records violations for later consumption by Stage ⑤
// Check and the RuleComplianceCompleteness metric. A violation means a
// confirmed mandatory rule was not referenced in the output — flagging it for
// human review without halting the workflow.
//
// The hook is safe for concurrent use: multiple agent turns may fire
// AfterModelCall in sequence, and Violations() accumulates across them.
type RuleAssertionHook struct {
	agentcore.BaseLifecycleHook

	// mustRules is the set of mandatory rule IDs that every conclusion must
	// reference. Typically extracted from ConfirmedRuleSet.ActiveConstraints()
	// filtered by Requirement == ReqMust.
	mustRules []string

	mu         sync.Mutex
	violations []RuleViolation
}

// RuleViolation records one instance of a mandatory rule going unreferenced.
type RuleViolation struct {
	Turn     int64  `json:"turn"`
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name,omitempty"`
	Output   string `json:"output_snippet,omitempty"` // first 200 chars of the offending output
}

// NewRuleAssertionHook creates a hook that checks the given rule constraints.
// Only ReqMust rules are enforced; Should/Note rules are advisory and skipped.
// Returns nil if no Must rules are present (hook would be a no-op).
func NewRuleAssertionHook(rules []reasoning.RuleConstraint) *RuleAssertionHook {
	var must []string
	names := make(map[string]string)
	for _, r := range rules {
		if r.Requirement == reasoning.ReqMust {
			must = append(must, r.ArticleID)
			names[r.ArticleID] = r.ArticleName
		}
	}
	if len(must) == 0 {
		return nil
	}
	return &RuleAssertionHook{mustRules: must}
}

// AfterModelCall checks the model output for references to each Must rule.
// Missing references are recorded as violations (non-blocking).
func (h *RuleAssertionHook) AfterModelCall(_ context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if h == nil || mcc == nil || mcc.Response == nil {
		return
	}
	content := strings.ToLower(mcc.Response.Content)
	if content == "" {
		return
	}

	var turn int64
	if arc != nil {
		turn = arc.Turn
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ruleID := range h.mustRules {
		if !strings.Contains(content, strings.ToLower(ruleID)) {
			h.violations = append(h.violations, RuleViolation{
				Turn:   turn,
				RuleID: ruleID,
				Output: truncate(mcc.Response.Content, 200),
			})
		}
	}
}

// Violations returns a copy of all recorded rule-violations accumulated across
// model calls. Callers (e.g. Stage ⑤ Check, report generation) consume this to
// surface "confirmed but ignored" rules in the final output.
func (h *RuleAssertionHook) Violations() []RuleViolation {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RuleViolation, len(h.violations))
	copy(out, h.violations)
	return out
}

// HasViolations reports whether any Must rule went unreferenced.
func (h *RuleAssertionHook) HasViolations() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.violations) > 0
}

// ComplianceScore returns the fraction of Must rules that were referenced
// across all model calls (1.0 = all referenced, 0 = none). This mirrors the
// RuleComplianceCompleteness metric but is computed live during execution.
func (h *RuleAssertionHook) ComplianceScore() float64 {
	if h == nil || len(h.mustRules) == 0 {
		return 1
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	violated := make(map[string]bool, len(h.violations))
	for _, v := range h.violations {
		violated[v.RuleID] = true
	}
	compliant := len(h.mustRules) - len(violated)
	if compliant < 0 {
		compliant = 0
	}
	return float64(compliant) / float64(len(h.mustRules))
}

// String returns a human-readable summary of violations for logging/debugging.
func (h *RuleAssertionHook) String() string {
	if h == nil {
		return "RuleAssertionHook(nil)"
	}
	v := h.Violations()
	if len(v) == 0 {
		return fmt.Sprintf("RuleAssertionHook: %d Must rules, all referenced ✓", len(h.mustRules))
	}
	return fmt.Sprintf("RuleAssertionHook: %d Must rules, %d violations (score %.0f%%)",
		len(h.mustRules), len(v), h.ComplianceScore()*100)
}
