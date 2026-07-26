package agentcore

// This file implements a proactive token-budget manager inspired by
// PilotDeck's context/budget/TokenBudgetManager.
//
// Mady already has a domain-tuned token estimator (chars/4 + CJK correction
// in token.go). What it lacked was (1) a drift-tolerance padding factor so
// the heuristic survives gaps against real provider tokenizers, and (2) a
// structured ok/warning/blocking state machine that the agent loop and
// context engines can consult. TokenBudgetManager adds exactly those two
// pieces on top of the existing estimator — it does NOT reinvent estimation.

import "fmt"

// BudgetState is the coarse consumption state of a context window.
type BudgetState string

const (
	// BudgetOK means consumption is comfortably below the warning threshold.
	BudgetOK BudgetState = "ok"
	// BudgetWarning means consumption has crossed the warning threshold;
	// callers may surface a hint to the user or pre-emptively compress.
	BudgetWarning BudgetState = "warning"
	// BudgetBlocking means consumption is near the hard limit; further input
	// risks a provider context-overflow error and compaction should fire.
	BudgetBlocking BudgetState = "blocking"
)

// BudgetConfig configures a TokenBudgetManager. Fields left at their zero
// value fall back to the documented defaults (see DefaultBudgetConfig).
type BudgetConfig struct {
	// WarningRatio is the padded-ratio at which the state becomes BudgetWarning.
	// Default 0.8.
	WarningRatio float64
	// BlockingRatio is the padded-ratio at which the state becomes BudgetBlocking.
	// Must exceed WarningRatio. Default 0.95.
	BlockingRatio float64
	// PaddingNumerator / PaddingDenominator form the drift-tolerance factor
	// applied to the raw estimate (raw * num/den, ceiled). Default 4/3, which
	// reserves ~33% headroom for estimator-vs-provider drift.
	PaddingNumerator   int
	PaddingDenominator int
}

// DefaultBudgetConfig returns the recommended defaults.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		WarningRatio:       0.8,
		BlockingRatio:      0.95,
		PaddingNumerator:   4,
		PaddingDenominator: 3,
	}
}

// BudgetSnapshot is a point-in-time evaluation of context budget consumption.
// All ratio fields are 0 when MaxContextTokens <= 0 (safe default, no
// spurious blocking on unknown window sizes).
type BudgetSnapshot struct {
	Tokens           int64       // raw estimated tokens (messages + tool defs)
	PaddedTokens     int64       // drift-tolerant upper bound (Tokens * padding)
	MaxContextTokens int64       // the window the snapshot was evaluated against
	Ratio            float64     // Tokens / MaxContextTokens
	PaddedRatio      float64     // PaddedTokens / MaxContextTokens
	WarningRatio     float64     // configured warning threshold
	BlockingRatio    float64     // configured blocking threshold
	State            BudgetState // derived from PaddedRatio (conservative)
}

// String returns a human-readable summary of the budget snapshot.
// Example: "context: 63/100 tokens (padded 84/100, 84.0%), state: warning"
func (s BudgetSnapshot) String() string {
	return fmt.Sprintf("context: %d/%d tokens (padded %d/%d, %.1f%%), state: %s",
		s.Tokens, s.MaxContextTokens,
		s.PaddedTokens, s.MaxContextTokens,
		s.PaddedRatio*100,
		s.State,
	)
}

// TokenBudgetManager wraps Mady's token estimator with drift tolerance and a
// structured budget state machine. It is safe for concurrent use: it holds
// only immutable configuration after construction.
type TokenBudgetManager struct {
	warningRatio       float64
	blockingRatio      float64
	paddingNumerator   int
	paddingDenominator int
}

// NewTokenBudgetManager constructs a manager from cfg, substituting defaults
// for any zero-value field so callers may safely pass BudgetConfig{}.
func NewTokenBudgetManager(cfg BudgetConfig) *TokenBudgetManager {
	d := DefaultBudgetConfig()
	m := &TokenBudgetManager{
		warningRatio:       cfg.WarningRatio,
		blockingRatio:      cfg.BlockingRatio,
		paddingNumerator:   cfg.PaddingNumerator,
		paddingDenominator: cfg.PaddingDenominator,
	}
	if m.warningRatio <= 0 {
		m.warningRatio = d.WarningRatio
	}
	if m.blockingRatio <= 0 {
		m.blockingRatio = d.BlockingRatio
	}
	if m.paddingNumerator <= 0 {
		m.paddingNumerator = d.PaddingNumerator
	}
	if m.paddingDenominator <= 0 {
		m.paddingDenominator = d.PaddingDenominator
	}
	return m
}

// Pad applies the configured drift-tolerance factor to a raw token count,
// returning ceil(raw * numerator/denominator). A raw count of 0 stays 0.
// If the factor is misconfigured (denominator <= 0) it returns raw unchanged
// rather than panicking — a degraded estimate is always preferable to a crash.
func (m *TokenBudgetManager) Pad(raw int64) int64 {
	if raw <= 0 {
		return 0
	}
	if m.paddingDenominator <= 0 || m.paddingNumerator <= 0 {
		return raw
	}
	// Integer ceil(raw * num / den): (raw*num + den - 1) / den.
	scaled := raw * int64(m.paddingNumerator)
	return (scaled + int64(m.paddingDenominator) - 1) / int64(m.paddingDenominator)
}

// Evaluate measures the budget consumption of a conversation (messages plus
// tool definitions) against maxContextTokens and returns a structured
// snapshot. The State field keys off the PADDED ratio so callers act before
// real provider overflow, not after.
func (m *TokenBudgetManager) Evaluate(msgs []Message, toolDefs []ToolDefinition, maxContextTokens int64) BudgetSnapshot {
	tokens := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)
	padded := m.Pad(tokens)

	var ratio, paddedRatio float64
	if maxContextTokens > 0 {
		ratio = float64(tokens) / float64(maxContextTokens)
		paddedRatio = float64(padded) / float64(maxContextTokens)
	}

	state := BudgetOK
	// paddedRatio = padded / max 是精确的有理数除法（两 int64 相除），
	// 在 float64 中多数场景可精确表示（如 84/100 = 0.84），因此 epsilon
	// 1e-12 在正常路径上从不触发；它仅为防御 1/3 类循环小数的极端浮点
	// 舍入场景而设，确保边界语义 (>=) 的稳定性。
	if paddedRatio+1e-12 >= m.blockingRatio {
		state = BudgetBlocking
	} else if paddedRatio+1e-12 >= m.warningRatio {
		state = BudgetWarning
	}

	return BudgetSnapshot{
		Tokens:           tokens,
		PaddedTokens:     padded,
		MaxContextTokens: maxContextTokens,
		Ratio:            ratio,
		PaddedRatio:      paddedRatio,
		WarningRatio:     m.warningRatio,
		BlockingRatio:    m.blockingRatio,
		State:            state,
	}
}
