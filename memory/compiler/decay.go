package compiler

import "time"

// Quality classifies the signal strength of a memory or trace.
type Quality string

const (
	QualityHigh   Quality = "HIGH_SIGNAL"
	QualityMedium Quality = "MEDIUM_SIGNAL"
	QualityNoise  Quality = "NOISE"
)

// ClassifyQuality determines the quality of an execution outcome based on
// its signals. High-signal traces (clear success/failure with low errors)
// are valuable for learning; noise traces (aborted or high error rate) are not.
func ClassifyQuality(trace ExecutionTrace) Quality {
	switch trace.Outcome {
	case OutcomeAborted:
		return QualityNoise
	case OutcomeSuccess:
		if trace.ToolErrors == 0 {
			return QualityHigh
		}
		return QualityMedium
	case OutcomeFailure:
		if trace.ToolErrors > trace.ToolCalls/2 {
			return QualityMedium // failure due to too many tool errors
		}
		return QualityHigh // clean failure is still informative
	case OutcomePartial:
		return QualityMedium
	default:
		return QualityNoise
	}
}

// DecayConfig controls confidence decay over time.
type DecayConfig struct {
	WeeklyDecayRate float64 // confidence lost per week (default: 0.05 = 5%)
	MinConfidence   float64 // floor confidence (default: 0.1)
}

// DefaultDecayConfig returns standard decay parameters.
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		WeeklyDecayRate: 0.05,
		MinConfidence:   0.1,
	}
}

// DecayedConfidence applies time-based decay to a base confidence.
// Confidence decreases by WeeklyDecayRate per week since the last use.
func DecayedConfidence(base float64, lastUsed time.Time, cfg DecayConfig) float64 {
	if lastUsed.IsZero() {
		return base
	}
	weeks := time.Since(lastUsed).Hours() / 24 / 7
	decay := 1.0 - cfg.WeeklyDecayRate*weeks
	if decay < cfg.MinConfidence {
		decay = cfg.MinConfidence
	}
	return base * decay
}

// StrategyConfidence computes the current confidence in a strategy,
// accounting for success rate and time-based decay.
func StrategyConfidence(s Strategy, cfg DecayConfig) float64 {
	base := s.SuccessRate()
	return DecayedConfidence(base, s.LastUsedAt, cfg)
}
