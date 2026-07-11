// Dual-track checker — combines the deterministic rule engine with an optional
// semantic LLM-judge track.
//
// Flow (ported from @nuo/legal-bus CheckerAgent.ts + CheckRuleEngine.ts):
//
//  1. Deterministic track: RuleEngine.Evaluate → rule results + Aggregate verdict.
//  2. Decide whether the LLM semantic track is needed (shouldTriggerLlm).
//  3. If not needed → return rule-only result.
//  4. If needed → LLM judges → merge rule + LLM verdicts.
//
// Verdict merge: if the rule track blocked, the final verdict stays blocked
// (rules are authoritative on hard constraints). Otherwise the LLM verdict
// takes over. The CheckMethod field records which tracks contributed.
package patent

import (
	"context"
	"strconv"
	"strings"
)

// CheckMethod records which tracks contributed to a verdict.
type CheckMethod string

const (
	CheckMethodRules    CheckMethod = "rules"     // deterministic only
	CheckMethodLLMJudge CheckMethod = "llm_judge" // semantic only (no rule issues)
	CheckMethodHybrid   CheckMethod = "hybrid"    // both tracks contributed
)

// CheckIssue is a single problem found during checking.
type CheckIssue struct {
	Severity    Severity
	Description string
	RuleID      string
}

// CheckerInput is the payload for a dual-track check.
type CheckerInput struct {
	StepID string
	Text   string // the text to check (e.g., analysis output)
	Domain string // analysis domain (e.g., patent_novelty)
	Role   string // producing role: researcher / executor (affects LLM trigger)
}

// CheckerResult is the outcome of a dual-track check.
type CheckerResult struct {
	StepID      string
	Verdict     Verdict
	CheckMethod CheckMethod
	Issues      []CheckIssue
	Summary     string
	LegalBasis  []string
	// Conflict records any disagreement between the two tracks (zero value
	// when no conflict or when only the rule track ran). Populated when the
	// semantic track contributes.
	Conflict TrackConflict
}

// LlmJudgeClient is the semantic-judge interface. It receives the text and
// applicable rules, and returns a verdict with reasons and suggestions.
// Implementations wrap a provider LLM call. A nil client disables the
// semantic track (rule-only mode).
type LlmJudgeClient interface {
	Judge(ctx context.Context, text string, rules []CheckRule) (
		verdict Verdict, reasons []string, suggestions []string, err error)
}

// Checker orchestrates the two tracks.
type Checker struct {
	ruleEngine *RuleEngine
	llm        LlmJudgeClient // may be nil for rule-only mode
	policy     MergePolicy
}

// NewChecker creates a dual-track checker with the default (conservative)
// merge policy. If llm is nil, only the deterministic track runs.
func NewChecker(engine *RuleEngine, llm LlmJudgeClient) *Checker {
	return &Checker{ruleEngine: engine, llm: llm, policy: DefaultMergePolicy()}
}

// NewCheckerWithPolicy creates a dual-track checker with a custom merge policy.
func NewCheckerWithPolicy(engine *RuleEngine, llm LlmJudgeClient, policy MergePolicy) *Checker {
	return &Checker{ruleEngine: engine, llm: llm, policy: policy}
}

// SetMergePolicy updates the conflict-resolution policy at runtime.
func (c *Checker) SetMergePolicy(policy MergePolicy) {
	c.policy = policy
}

// Check runs the dual-track check. The rule track always runs; the LLM track
// runs only when shouldTriggerLlm returns true.
func (c *Checker) Check(ctx context.Context, input CheckerInput) (*CheckerResult, error) {
	rules := c.ruleEngine.Rules()
	ruleResults := c.ruleEngine.Evaluate(rules, input.Text, input.Domain)
	ruleVerdict := Aggregate(ruleResults)

	if !c.shouldTriggerLlm(ruleVerdict, ruleResults, input.Role) {
		return c.buildRuleOnlyResult(ruleResults, ruleVerdict, input.StepID), nil
	}

	// Semantic track.
	llmVerdict, llmReasons, llmSuggestions, err := c.llm.Judge(ctx, input.Text, rules)
	if err != nil {
		// LLM failure degrades gracefully to rule-only result.
		return c.buildRuleOnlyResult(ruleResults, ruleVerdict, input.StepID), nil
	}

	return c.mergeVerdicts(ruleResults, ruleVerdict, llmVerdict, llmReasons, llmSuggestions, input.StepID), nil
}

// shouldTriggerLlm decides whether the semantic track should run:
//   - blocked → false (rule track is authoritative on hard constraints)
//   - researcher role → true (always cross-check research output)
//   - needs_revision → true (LLM may find or resolve nuanced issues)
//   - executor + any rule issues → true
func (c *Checker) shouldTriggerLlm(ruleVerdict Verdict, ruleResults []RuleCheckResult, role string) bool {
	if c.llm == nil {
		return false
	}
	if ruleVerdict == VerdictBlocked {
		return false
	}
	if role == "researcher" {
		return true
	}
	if ruleVerdict == VerdictNeedsRevision {
		return true
	}
	if role == "executor" && len(ruleResults) > 0 {
		return true
	}
	return false
}

func (c *Checker) buildRuleOnlyResult(results []RuleCheckResult, verdict Verdict, stepID string) *CheckerResult {
	var issues []CheckIssue
	for _, r := range results {
		issues = append(issues, CheckIssue{
			Severity:    r.Severity,
			Description: r.Message,
			RuleID:      r.RuleID,
		})
	}
	return &CheckerResult{
		StepID:      stepID,
		Verdict:     verdict,
		CheckMethod: CheckMethodRules,
		Issues:      issues,
		Summary:     summarizeIssues(issues),
	}
}

func (c *Checker) mergeVerdicts(
	ruleResults []RuleCheckResult,
	ruleVerdict Verdict,
	llmVerdict Verdict,
	llmReasons []string,
	llmSuggestions []string,
	stepID string,
) *CheckerResult {
	// Merge issues from both tracks.
	var merged []CheckIssue
	for _, r := range ruleResults {
		merged = append(merged, CheckIssue{
			Severity:    r.Severity,
			Description: r.Message,
			RuleID:      r.RuleID,
		})
	}
	for i, reason := range llmReasons {
		merged = append(merged, CheckIssue{
			Severity:    SeverityMinor,
			Description: reason,
			RuleID:      "llm_" + strconv.Itoa(i),
		})
	}

	// Resolve track conflict via the configured merge policy.
	finalVerdict, conflict := ResolveTrackVerdict(ruleVerdict, llmVerdict, c.policy)

	var method CheckMethod
	if ruleVerdict == VerdictBlocked || len(ruleResults) > 0 {
		method = CheckMethodHybrid
	} else {
		method = CheckMethodLLMJudge
	}

	summary := strings.Join(llmSuggestions, "; ")
	if summary == "" {
		summary = summarizeIssues(merged)
	}

	return &CheckerResult{
		StepID:      stepID,
		Verdict:     finalVerdict,
		CheckMethod: method,
		Issues:      merged,
		Summary:     summary,
		Conflict:    conflict,
	}
}

func summarizeIssues(issues []CheckIssue) string {
	if len(issues) == 0 {
		return "所有检查通过"
	}
	critical, major, minor := 0, 0, 0
	for _, iss := range issues {
		switch iss.Severity {
		case SeverityCritical:
			critical++
		case SeverityMajor:
			major++
		case SeverityMinor:
			minor++
		}
	}
	var b strings.Builder
	b.WriteString("发现 ")
	b.WriteString(strconv.Itoa(critical))
	b.WriteString(" 个严重, ")
	b.WriteString(strconv.Itoa(major))
	b.WriteString(" 个主要, ")
	b.WriteString(strconv.Itoa(minor))
	b.WriteString(" 个次要问题")
	return b.String()
}

// FormatCheckerResult renders a CheckerResult as a Markdown section.
func FormatCheckerResult(r *CheckerResult) string {
	var b strings.Builder
	b.WriteString("## 双轨检查结果\n\n")
	b.WriteString("- 结论: ")
	b.WriteString(string(r.Verdict))
	b.WriteString("\n- 检查方式: ")
	b.WriteString(string(r.CheckMethod))
	b.WriteString("\n- 摘要: ")
	b.WriteString(r.Summary)
	b.WriteString("\n\n")

	if len(r.Issues) == 0 {
		b.WriteString("无问题。\n")
		return b.String()
	}

	b.WriteString("| 严重度 | 描述 | 规则 |\n")
	b.WriteString("|--------|------|------|\n")
	for _, iss := range r.Issues {
		b.WriteString("| ")
		b.WriteString(string(iss.Severity))
		b.WriteString(" | ")
		b.WriteString(iss.Description)
		b.WriteString(" | ")
		b.WriteString(iss.RuleID)
		b.WriteString(" |\n")
	}

	// Append conflict section if present.
	conflictSection := FormatConflict(&r.Conflict)
	if conflictSection != "" {
		b.WriteString("\n")
		b.WriteString(conflictSection)
	}
	return b.String()
}
