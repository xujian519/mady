package guardrails

import (
	"strings"
)

// Severity indicates how critical a rule violation is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Action specifies what happens when a rule check fails.
type Action string

const (
	// ActionBlock replaces the entire output with a rejection message.
	ActionBlock Action = "block"

	// ActionInject appends a disclaimer or warning to the output.
	ActionInject Action = "inject"

	// ActionAlert passes the content through but logs a warning.
	ActionAlert Action = "alert"

	// ActionLog records the violation but takes no visible action.
	ActionLog Action = "log"
)

// RuleResult is the outcome of a single rule check.
type RuleResult struct {
	// Passed indicates whether the check passed.
	Passed bool `json:"passed"`

	// Severity of the violation (error/warning/info).
	Severity Severity `json:"severity,omitempty"`

	// Action to take when the check fails.
	Action Action `json:"action,omitempty"`

	// Message describes the violation for logging or display.
	Message string `json:"message,omitempty"`

	// RuleName identifies which rule produced this result.
	RuleName string `json:"rule_name,omitempty"`
}

// Rule is the interface for a single content check rule.
// Implementations should be stateless and safe for concurrent use.
type Rule interface {
	// Name returns a short identifier for this rule (e.g. "fact-check").
	Name() string

	// Check evaluates the content and returns a result.
	// content is the model output text to check.
	// metadata provides optional context (e.g. domain, expected sections).
	Check(content string, metadata map[string]any) RuleResult
}

// RulePipeline chains multiple rules and executes them in registration order.
// All rules run regardless of earlier failures (fail-open for visibility).
// The pipeline aggregates results and can modify content progressively.
type RulePipeline struct {
	rules []Rule
}

// NewRulePipeline creates a pipeline with the given rules.
func NewRulePipeline(rules ...Rule) *RulePipeline {
	return &RulePipeline{rules: rules}
}

// Add appends a rule to the pipeline.
func (p *RulePipeline) Add(r Rule) {
	p.rules = append(p.rules, r)
}

// CheckAll runs all rules against the content and returns aggregated results.
// All rules execute regardless of earlier failures.
func (p *RulePipeline) CheckAll(content string, metadata map[string]any) []RuleResult {
	var results []RuleResult
	for _, r := range p.rules {
		result := r.Check(content, metadata)
		result.RuleName = r.Name()
		results = append(results, result)
	}
	return results
}

// Apply runs all rules and applies remedial actions to the content.
// Returns the (possibly modified) content and all rule results.
//
// Actions are applied in priority order:
//  1. Block — if any rule returns ActionBlock, the content is replaced.
//  2. Inject — disclaimers from ActionInject rules are appended.
//  3. Alert/Log — no content modification.
func (p *RulePipeline) Apply(content string, metadata map[string]any) (string, []RuleResult) {
	results := p.CheckAll(content, metadata)

	// Check for blocking rules first (highest priority).
	for _, r := range results {
		if !r.Passed && r.Action == ActionBlock {
			return r.Message, results
		}
	}

	// Collect inject disclaimers.
	var disclaimers []string
	for _, r := range results {
		if !r.Passed && r.Action == ActionInject && r.Message != "" {
			// Avoid duplicate disclaimer injection.
			if !strings.Contains(content, r.Message) {
				disclaimers = append(disclaimers, r.Message)
			}
		}
	}

	if len(disclaimers) > 0 {
		content += "\n\n---\n" + strings.Join(disclaimers, "\n")
	}

	return content, results
}

// HasBlocking returns true if any result has ActionBlock and did not pass.
func HasBlocking(results []RuleResult) bool {
	for _, r := range results {
		if !r.Passed && r.Action == ActionBlock {
			return true
		}
	}
	return false
}

// FailedRules returns the names of rules that did not pass.
func FailedRules(results []RuleResult) []string {
	var names []string
	for _, r := range results {
		if !r.Passed {
			names = append(names, r.RuleName)
		}
	}
	return names
}
