package guardian

import (
	"encoding/json"
	"strings"
)

// RiskLevel classifies the assessed risk of a tool call.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Outcome is the Guardian's verdict on a tool call.
type Outcome string

const (
	OutcomeAllow Outcome = "allow"
	OutcomeDeny  Outcome = "deny"
)

// Assessment is the structured result of a Guardian review.
type Assessment struct {
	RiskLevel RiskLevel `json:"risk_level"`
	Outcome   Outcome   `json:"outcome"`
	Rationale string    `json:"rationale"`
}

// IsDenied returns true when the outcome is deny.
func (a Assessment) IsDenied() bool {
	return a.Outcome == OutcomeDeny
}

// parseAssessment extracts an Assessment from the LLM response content.
// It tries JSON parsing first, then falls back to keyword detection.
func parseAssessment(content string) Assessment {
	content = strings.TrimSpace(content)

	// Try JSON decode first
	var a Assessment
	if err := json.Unmarshal([]byte(content), &a); err == nil && a.Outcome != "" {
		return normalizeAssessment(a)
	}

	// Try extracting JSON from markdown code block
	if idx := strings.Index(content, "{"); idx >= 0 {
		end := strings.LastIndex(content, "}")
		if end > idx {
			if err := json.Unmarshal([]byte(content[idx:end+1]), &a); err == nil && a.Outcome != "" {
				return normalizeAssessment(a)
			}
		}
	}

	// Fallback: keyword detection
	lower := strings.ToLower(content)
	if strings.Contains(lower, "deny") || strings.Contains(lower, "拒绝") {
		return Assessment{Outcome: OutcomeDeny, RiskLevel: RiskHigh,
			Rationale: "fallback keyword detection"}
	}
	return Assessment{Outcome: OutcomeAllow, RiskLevel: RiskLow,
		Rationale: "fallback keyword detection"}
}

func normalizeAssessment(a Assessment) Assessment {
	if a.Outcome != OutcomeDeny && a.Outcome != OutcomeAllow {
		a.Outcome = OutcomeAllow
	}
	switch a.RiskLevel {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		a.RiskLevel = RiskLow
	}
	return a
}
