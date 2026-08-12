// Package patent — rule engine for patent analysis, deterministic track of the dual-track checker.
//
// This engine performs keyword/synonym-based checks against Chinese patent law
// requirements (novelty, inventiveness three-step method, disclosure sufficiency,
// claim analysis, infringement). It is the deterministic counterpart to the
// semantic LLM-judge track in checker.go.
//
// Design (ported from @nuo/legal-bus patent-checks.ts + keyword-utils.ts):
//
//	RuleEngine.Evaluate(rules, text) → []RuleCheckResult
//	Aggregate(results) → Verdict (pass / needs_revision / blocked)
//
// Verdict aggregation: a single Level-0 or Level-1 failure → blocked;
// three or more Level-2 failures → needs_revision; otherwise pass.
package patent

import (
	"github.com/xujian519/mady/domains/rules"
)

// Severity describes how serious a rule violation is for reporting purposes.
type Severity = rules.Severity

const (
	// SeverityCritical indicates a critical rule violation.
	SeverityCritical Severity = "critical"
	// SeverityMajor indicates a major rule violation.
	SeverityMajor Severity = "major"
	// SeverityMinor indicates a minor rule violation.
	SeverityMinor Severity = "minor"
)

// RuleLevel controls verdict aggregation severity. Level 0 is the strictest.
type RuleLevel int

const (
	// LevelMust indicates a blocking failure that hard-blocks the verdict.
	LevelMust RuleLevel = 0
	// LevelShould indicates an important failure that also blocks the verdict.
	LevelShould RuleLevel = 1
	// LevelQuality indicates a quality concern; 3+ failures needed to block.
	LevelQuality RuleLevel = 2
)

// Verdict is the aggregate pass/fail decision of a rule check.
type Verdict string

const (
	// VerdictPass indicates the check passed all rules.
	VerdictPass Verdict = "pass"
	// VerdictNeedsRevision indicates the check passed but with suggestions.
	VerdictNeedsRevision Verdict = "needs_revision"
	// VerdictBlocked indicates the check failed and blocked the action.
	VerdictBlocked Verdict = "blocked"
)

// CheckType identifies the concrete checking strategy a rule uses.
type CheckType string

const (
	// CheckNovelty indicates a novelty check (Article 22.2).
	CheckNovelty CheckType = "patent_novelty"
	// CheckInventiveness indicates an inventiveness check (Article 22.3).
	CheckInventiveness CheckType = "patent_inventiveness"
	// CheckInfringement indicates an infringement check.
	CheckInfringement CheckType = "patent_infringement"
	// CheckDisclosure indicates a disclosure sufficiency check (Article 26.3).
	CheckDisclosure CheckType = "patent_disclosure"
	// CheckClaimAnalysis indicates a claim analysis check.
	CheckClaimAnalysis CheckType = "patent_claim_analysis"
	// CheckDesignComparison indicates a design patent comparison check.
	CheckDesignComparison CheckType = "patent_design_comparison"
	// CheckPublicAccess indicates a public access/prior art check.
	CheckPublicAccess CheckType = "patent_public_access"
	// CheckAmendmentScope indicates an amendment scope check (Article 33).
	CheckAmendmentScope CheckType = "patent_amendment_scope"
	// CheckSubjectMatter indicates a subject matter eligibility check.
	CheckSubjectMatter CheckType = "patent_subject_matter"
)

// Domain string constants (plain string, for Domain field).
const (
	domainInventiveness = "patent_inventiveness"
	domainNovelty       = "patent_novelty"
	domainInfringement  = "patent_infringement"
	domainDisclosure    = "patent_disclosure"
	domainClaims        = "patent_claims"
	domainExamination   = "patent_examination"
	domainDesign        = "patent_design"
	domainInvalidation  = "patent_invalidation"
	domainAmendment     = "patent_amendment"
	domainReexamination = "patent_reexamination"
)

// Category string constants (for ReasoningPattern.Category).
const (
	categoryCreativity = "creativity"
	categoryNovelty    = "novelty"
	categoryClaims     = "claims"
	categoryOther      = "other"
)

// Claim analysis dimension names (used for rule routing by the drafting engine).
const (
	dimClarity     = "clarity"
	dimSupport     = "support"
	dimEssential   = "essential_features"
	dimConsistency = "consistency"
)

// Claim analysis dimension names (used for rule routing by the drafting engine).
// The actual dimension values come from the claimdrafting package constants.
// These local aliases are kept for documentation clarity in the rule files.

// Frequently used patent term constants (Chinese).
const (
	termDistinguishingFeatures = "区别技术特征"
	termClosestPriorArt        = "最接近对比文件"
	termCommonKnowledge        = "公知常识"
	termObvious                = "显而易见"
	termTechHint               = "技术启示"
	termPriorArtDoc            = "对比文件"
	termPriority               = "优先权"
	termProtectionScope        = "保护范围"
	termAmendmentExceed        = "修改超范围"
	termTechSolution           = "技术方案"
	termTechEffect             = "技术效果"
	termNaturalLaw             = "自然规律"
	termSciDiscovery           = "科学发现"
	termMentalActivity         = "智力活动规则"
	termDesignPatent           = "外观设计"
	termOverallVisual          = "整体视觉效果"
	termProductCategory        = "产品种类"
	termFunctionalLimit        = "功能性限定"
	termExpData                = "实验数据"
	termCanUse                 = "能够使用"
	termPersonSkilled          = "本领域技术人员"
	termUseLimit               = "用途限定"
	termInternetDisclosure     = "互联网公开"
	termDiffFeatures           = "区别特征"
	termFilingDate             = "filing date"
	termEnablement             = "enablement"
	// additional frequently used Chinese terms
	termNovelty               = "新颖性"
	termInventiveness         = "创造性"
	termPriorArt              = "现有技术"
	termPriorityDate          = "优先权日"
	termSufficientDisclosure  = "充分公开"
	termConventionalDesign    = "常规设计"
	termTechFeature           = "技术特征"
	termCombinationMotivation = "组合动机"
	termBeneficialEffect      = "有益效果"
	termCanMake               = "能够制造"
	termFilingDateLit         = "申请日"
	// additional constants for frequently repeated strings
	termNonObvious              = "非显而易见"
	termSufficientDisclosurePub = "公开充分"
	termCommonTechnicalMeans    = "惯用技术手段"
	termCombinationHint         = "结合启示"
	termEnable                  = "能够实现"
	termClaimsLocal             = "权利要求"
	termClosestPriorArtFull     = "最接近的现有技术"
)

// Rule ID constants.
const (
	ruleInventivenessThreeStep  = "INVENTIVENESS-THREE-STEP"
	ruleNoveltySingleComparison = "NOVELTY-SINGLE-COMPARISON"
)

// CheckRule is a single deterministic rule in the patent rule engine.
// The CheckType field determines which check parameters are consulted
// (RequiredElements, StepElements, RequiredAspects, Dimensions, etc.).
type CheckRule struct {
	ID          string
	Name        string
	Description string
	Level       RuleLevel
	Severity    Severity
	Message     string // failure message (Chinese, user-visible)
	CheckType   CheckType
	Domain      string // applicable domain filter ("" = all domains)

	// Check parameters — meaning depends on CheckType.
	RequiredElements []string   // CheckNovelty / CheckInfringement: all must match
	StepElements     [][]string // CheckInventiveness: 3 steps, any match per step
	RequiredAspects  []string   // CheckDisclosure: all must match
	Dimensions       []string   // CheckClaimAnalysis: dimensions to verify
	PathElements     [][]string // reasoning path step completeness (any CheckType)
	SingleComparison bool       // CheckNovelty: enforce single-comparison principle
	DependsOn        []string   // rule IDs that must also be checked first
	FixSuggestion    string
}

// RuleCheckResult is the outcome of evaluating one rule against a text.
type RuleCheckResult struct {
	RuleID        string
	RuleName      string
	Passed        bool
	Level         RuleLevel
	Severity      Severity
	Message       string
	FixSuggestion string
}

// RuleEngine holds a registered set of deterministic rules.
type RuleEngine struct {
	rules map[string]CheckRule
}

// NewRuleEngine creates an empty rule engine.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{rules: make(map[string]CheckRule)}
}

// RegisterRule adds (or replaces) a rule by its ID.
func (e *RuleEngine) RegisterRule(rule CheckRule) {
	e.rules[rule.ID] = rule
}

// RegisterRules adds multiple rules.
func (e *RuleEngine) RegisterRules(rules []CheckRule) {
	for _, r := range rules {
		e.rules[r.ID] = r
	}
}

// RemoveRule deletes a rule by ID.
func (e *RuleEngine) RemoveRule(id string) {
	delete(e.rules, id)
}

// GetRule returns a rule by ID.
func (e *RuleEngine) GetRule(id string) (CheckRule, bool) {
	r, ok := e.rules[id]
	return r, ok
}

// Rules returns all registered rules.
func (e *RuleEngine) Rules() []CheckRule {
	out := make([]CheckRule, 0, len(e.rules))
	for _, r := range e.rules {
		out = append(out, r)
	}
	return out
}

// Evaluate runs the given rules against text. Only rules whose Domain is empty
// or matches the domain argument are evaluated. Each rule is dispatched to its
// CheckType-specific checker.
func (e *RuleEngine) Evaluate(rules []CheckRule, text string, domain string) []RuleCheckResult {
	var results []RuleCheckResult
	for _, rule := range rules {
		if domain != "" && rule.Domain != "" && rule.Domain != domain {
			continue
		}
		passed, detail := evaluateRule(rule, text)
		if !passed {
			msg := rule.Message
			if detail != "" {
				msg = detail
			}
			results = append(results, RuleCheckResult{
				RuleID:        rule.ID,
				RuleName:      rule.Name,
				Passed:        false,
				Level:         rule.Level,
				Severity:      rule.Severity,
				Message:       msg,
				FixSuggestion: rule.FixSuggestion,
			})
		}
	}
	return results
}

// Aggregate computes the verdict from a set of rule results.
//
//   - Any Level-0 (Must) or Level-1 (Should) failure → blocked.
//   - 3+ Level-2 (Quality) failures → needs_revision.
//   - Otherwise → pass.
func Aggregate(results []RuleCheckResult) Verdict {
	level2Failures := 0
	for _, r := range results {
		if r.Passed {
			continue
		}
		if r.Level <= LevelShould {
			return VerdictBlocked
		}
		if r.Level == LevelQuality {
			level2Failures++
		}
	}
	if level2Failures >= 3 {
		return VerdictNeedsRevision
	}
	return VerdictPass
}
