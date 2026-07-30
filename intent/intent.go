// Package intent provides unified intent classification for Mady.
//
// It aggregates three existing classification systems into a single pipeline:
//   - Domain routing (chat/patent/legal/assistant)
//   - Case type detection (invalidity/infringement/novelty/drafting/OA/etc.)
//   - Complexity classification (Low/Medium/High)
//
// The UnifiedIntentRouter chains four classifiers in priority order:
//  1. Preference (learned user corrections, confidence ≥0.9)
//  2. Keyword (deterministic string matching, confidence ≥0.8)
//  3. LLM (structured-output classification)
//  4. Semantic (vector similarity, as LLM augmentation)
//
// All classifiers fall back to chat/Low on failure.
package intent

// Domain represents the target domain for a user request.
type Domain string

const (
	DomainChat      Domain = "chat"
	DomainAssistant Domain = "assistant"
	DomainPatent    Domain = "patent"
	DomainLegal     Domain = "legal"
)

// SubIntent represents a fine-grained intent within a domain.
// For patent: invalidity, infringement, novelty, inventiveness, drafting, OA, etc.
// For legal: contract, litigation, regulation, etc.
type SubIntent string

const (
	SubIntentInvalidation  SubIntent = "invalidation"
	SubIntentInfringement  SubIntent = "infringement"
	SubIntentNovelty       SubIntent = "novelty"
	SubIntentInventiveness SubIntent = "inventiveness"
	SubIntentDrafting      SubIntent = "drafting"
	SubIntentOAResponse    SubIntent = "oa_response"
	SubIntentReexamination SubIntent = "reexamination"
	SubIntentFTO           SubIntent = "fto"
	SubIntentEnablement    SubIntent = "enablement"
	SubIntentGeneral       SubIntent = "general"
)

// RunMode selects how a task should be processed.
type RunMode string

const (
	ModeDirect       RunMode = "direct"
	ModeJudgment     RunMode = "judgment"
	ModeFlexiblePlan RunMode = "flexible_plan"
)

// Complexity represents the reasoning complexity of a request.
type Complexity int

const (
	ComplexityLow Complexity = iota
	ComplexityMedium
	ComplexityHigh
)

// IntentResult is the unified output of intent classification.
type IntentResult struct {
	// Domain is the target domain (chat/patent/legal/assistant).
	Domain Domain `json:"domain"`

	// SubIntent is the fine-grained intent within the domain.
	// Empty for chat/assistant domains.
	SubIntent SubIntent `json:"sub_intent,omitempty"`

	// RunMode suggests how to process: direct, judgment, or flexible_plan.
	RunMode RunMode `json:"run_mode"`

	// Complexity indicates the reasoning complexity (Low/Medium/High).
	Complexity Complexity `json:"complexity"`

	// Confidence is the aggregated confidence score (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Sources lists which classifiers contributed to this result.
	Sources []string `json:"sources,omitempty"`

	// MatchedKeywords are the keywords that triggered keyword classification.
	MatchedKeywords []string `json:"matched_keywords,omitempty"`

	// Suggestion is a human-readable recommendation based on the intent.
	Suggestion string `json:"suggestion,omitempty"`

	// ExplicitTrigger indicates the user explicitly requested this domain
	// (e.g., via "@legal" prefix).
	ExplicitTrigger bool `json:"explicit_trigger,omitempty"`
}

// Classifier is the interface for individual intent classifiers.
// Each classifier returns a partial IntentResult.
type Classifier interface {
	// Classify analyzes the user input and returns an intent.
	// Confidence should be 0.0-1.0. Return zero-value IntentResult
	// with 0 confidence if the classifier cannot determine intent.
	Classify(input string) IntentResult

	// Name returns a human-readable identifier for this classifier.
	Name() string
}

// UnifiedRouter orchestrates multiple classifiers into a priority chain.
//
// Routing priority (first non-zero confidence wins):
//  1. Preference (learned corrections)
//  2. Keyword (deterministic matching)
//  3. LLM (structured output)
//  4. Semantic (vector similarity)
//
// If all classifiers fail, returns chat/Low as safe default.
type UnifiedRouter struct {
	classifiers []Classifier
}

// NewUnifiedRouter creates a router with the given classifiers in priority order.
func NewUnifiedRouter(classifiers ...Classifier) *UnifiedRouter {
	return &UnifiedRouter{classifiers: classifiers}
}

// Classify runs all classifiers in priority order and returns the first
// result with confidence above the threshold for that classifier type.
// Falls back to the classifier with highest confidence, or chat/Low.
func (r *UnifiedRouter) Classify(input string) IntentResult {
	var best IntentResult
	bestConfidence := 0.0

	for _, c := range r.classifiers {
		result := c.Classify(input)
		if result.Confidence > bestConfidence {
			best = result
			bestConfidence = result.Confidence
		}

		// Different classifiers have different confidence thresholds for
		// "good enough to stop". Preference needs 0.9, keyword 0.8.
		threshold := r.thresholdFor(c.Name())
		if result.Confidence >= threshold {
			return result
		}
	}

	// Fallback: return best result found, or safe default.
	if bestConfidence > 0 {
		return best
	}
	return IntentResult{
		Domain:     DomainChat,
		RunMode:    ModeDirect,
		Complexity: ComplexityLow,
		Confidence: 0.5,
		Sources:    []string{"fallback"},
	}
}

// thresholdFor returns the confidence threshold for a given classifier type.
func (r *UnifiedRouter) thresholdFor(name string) float64 {
	switch name {
	case "preference":
		return 0.9
	case "keyword":
		return 0.8
	default:
		return 0.7
	}
}
