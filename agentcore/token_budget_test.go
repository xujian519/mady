package agentcore

import (
	"strings"
	"testing"
)

// TestDefaultBudgetConfig verifies the documented defaults are applied when
// the caller leaves fields at their zero value.
func TestDefaultBudgetConfig(t *testing.T) {
	cfg := DefaultBudgetConfig()
	if cfg.WarningRatio != 0.8 {
		t.Fatalf("WarningRatio default = %v, want 0.8", cfg.WarningRatio)
	}
	if cfg.BlockingRatio != 0.95 {
		t.Fatalf("BlockingRatio default = %v, want 0.95", cfg.BlockingRatio)
	}
	if cfg.PaddingNumerator != 4 || cfg.PaddingDenominator != 3 {
		t.Fatalf("padding default = %d/%d, want 4/3", cfg.PaddingNumerator, cfg.PaddingDenominator)
	}
}

// TestNewTokenBudgetManager_Defaults confirms a zero-value config yields the
// documented defaults (so callers can pass BudgetConfig{} safely).
func TestNewTokenBudgetManager_Defaults(t *testing.T) {
	m := NewTokenBudgetManager(BudgetConfig{})
	snap := m.Evaluate(nil, nil, 1000)
	if snap.WarningRatio != 0.8 {
		t.Fatalf("WarningRatio = %v, want 0.8", snap.WarningRatio)
	}
	if snap.BlockingRatio != 0.95 {
		t.Fatalf("BlockingRatio = %v, want 0.95", snap.BlockingRatio)
	}
}

// TestPad applies the drift-tolerance padding factor (ceil(raw * num/den)).
// This is the core mechanism that lets the heuristic estimator survive
// tokenizer drift against real provider tokenizers.
func TestPad(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	cases := []struct {
		raw  int64
		want int64
	}{
		{0, 0},   // zero stays zero (no padding on empty)
		{3, 4},   // ceil(3 * 4/3) = ceil(4) = 4
		{7, 10},  // ceil(7 * 4/3) = ceil(9.33) = 10
		{9, 12},  // ceil(9 * 4/3) = ceil(12) = 12
		{10, 14}, // ceil(10 * 4/3) = ceil(13.33) = 14
		{100, 134},
	}
	for _, c := range cases {
		if got := m.Pad(c.raw); got != c.want {
			t.Fatalf("Pad(%d) = %d, want %d", c.raw, got, c.want)
		}
	}
}

// TestPad_CustomFactor confirms a non-default padding factor is honored.
func TestPad_CustomFactor(t *testing.T) {
	m := NewTokenBudgetManager(BudgetConfig{
		PaddingNumerator:   3,
		PaddingDenominator: 2,
	})
	// ceil(10 * 3/2) = ceil(15) = 15
	if got := m.Pad(10); got != 15 {
		t.Fatalf("custom Pad(10) = %d, want 15", got)
	}
}

// TestPad_InvalidFactor guards against a misconfigured denominator: when the
// factor is unusable (denominator <= 0) Pad must return the raw value rather
// than panic on divide-by-zero. NB: NewTokenBudgetManager auto-fills invalid
// fields with defaults, so this path is exercised by constructing the manager
// directly (same package can access unexported fields).
func TestPad_InvalidFactor(t *testing.T) {
	m := &TokenBudgetManager{
		paddingNumerator:   4,
		paddingDenominator: 0, // invalid
	}
	if got := m.Pad(100); got != 100 {
		t.Fatalf("Pad with invalid factor = %d, want raw 100", got)
	}
	m2 := &TokenBudgetManager{
		paddingNumerator:   0, // also invalid
		paddingDenominator: 3,
	}
	if got := m2.Pad(100); got != 100 {
		t.Fatalf("Pad with zero numerator = %d, want raw 100", got)
	}
}

// TestEvaluate_OK exercises the healthy band: padded ratio well below the
// warning threshold → BudgetOK. Also asserts exact token bookkeeping.
func TestEvaluate_OK(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	msgs := []Message{{Role: RoleUser, Content: "hello world"}}
	// EstimateMessagesTokens([msg]) = 3 (framing) + 4 (overhead) + 3 (content) = 10
	snap := m.Evaluate(msgs, nil, 10000)
	if snap.Tokens != 10 {
		t.Fatalf("Tokens = %d, want 10", snap.Tokens)
	}
	if snap.PaddedTokens != 14 { // ceil(10 * 4/3)
		t.Fatalf("PaddedTokens = %d, want 14", snap.PaddedTokens)
	}
	if snap.Ratio < 0 || snap.Ratio > 0.01 {
		t.Fatalf("Ratio = %v, want ~0.001", snap.Ratio)
	}
	if snap.State != BudgetOK {
		t.Fatalf("State = %v, want BudgetOK", snap.State)
	}
	if snap.MaxContextTokens != 10000 {
		t.Fatalf("MaxContextTokens = %d, want 10000", snap.MaxContextTokens)
	}
}

// TestEvaluate_Warning lands the padded ratio in [0.8, 0.95) → BudgetWarning.
func TestEvaluate_Warning(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	// Build ASCII content so EstimateTokens is deterministic:
	//   content of 221 bytes → (221+3)/4 = 56 tokens
	//   msg tokens = 4 + 56 = 60
	//   EstimateMessagesTokens = 3 + 60 = 63
	//   padded = ceil(63 * 4/3) = 84
	//   paddedRatio against window 100 = 0.84 ∈ [0.8, 0.95) → warning
	content := strings.Repeat("a", 221)
	msgs := []Message{{Role: RoleUser, Content: content}}
	snap := m.Evaluate(msgs, nil, 100)
	if snap.Tokens != 63 {
		t.Fatalf("Tokens = %d, want 63", snap.Tokens)
	}
	if snap.PaddedTokens != 84 {
		t.Fatalf("PaddedTokens = %d, want 84", snap.PaddedTokens)
	}
	if snap.PaddedRatio < 0.8 || snap.PaddedRatio >= 0.95 {
		t.Fatalf("PaddedRatio = %v, want in [0.8, 0.95)", snap.PaddedRatio)
	}
	if snap.State != BudgetWarning {
		t.Fatalf("State = %v, want BudgetWarning", snap.State)
	}
}

// TestEvaluate_Blocking lands the padded ratio >= 0.95 → BudgetBlocking.
func TestEvaluate_Blocking(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	//   content of 257 bytes → (257+3)/4 = 65 tokens
	//   msg tokens = 4 + 65 = 69
	//   EstimateMessagesTokens = 3 + 69 = 72
	//   padded = ceil(72 * 4/3) = 96
	//   paddedRatio against window 100 = 0.96 >= 0.95 → blocking
	content := strings.Repeat("a", 257)
	msgs := []Message{{Role: RoleUser, Content: content}}
	snap := m.Evaluate(msgs, nil, 100)
	if snap.Tokens != 72 {
		t.Fatalf("Tokens = %d, want 72", snap.Tokens)
	}
	if snap.PaddedTokens != 96 {
		t.Fatalf("PaddedTokens = %d, want 96", snap.PaddedTokens)
	}
	if snap.PaddedRatio < 0.95 {
		t.Fatalf("PaddedRatio = %v, want >= 0.95", snap.PaddedRatio)
	}
	if snap.State != BudgetBlocking {
		t.Fatalf("State = %v, want BudgetBlocking", snap.State)
	}
}

// TestEvaluate_CustomThresholds verifies that a stricter config lowers the
// bands so the same conversation crosses into warning earlier.
func TestEvaluate_CustomThresholds(t *testing.T) {
	m := NewTokenBudgetManager(BudgetConfig{
		WarningRatio:  0.5,
		BlockingRatio: 0.7,
	})
	// 63 raw tokens against window 100 → paddedRatio 0.84.
	content := strings.Repeat("a", 221)
	msgs := []Message{{Role: RoleUser, Content: content}}
	snap := m.Evaluate(msgs, nil, 100)
	// 0.84 >= 0.7 (custom blocking) → blocking, not warning.
	if snap.State != BudgetBlocking {
		t.Fatalf("State = %v, want BudgetBlocking under custom 0.7 threshold", snap.State)
	}
	if snap.WarningRatio != 0.5 || snap.BlockingRatio != 0.7 {
		t.Fatalf("thresholds not reflected: warn=%v block=%v", snap.WarningRatio, snap.BlockingRatio)
	}
}

// TestEvaluate_ZeroWindow guards against division issues: a zero/negative
// context window yields ratio 0 and BudgetOK (no spurious blocking).
func TestEvaluate_ZeroWindow(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	msgs := []Message{{Role: RoleUser, Content: "hello world"}}
	snap := m.Evaluate(msgs, nil, 0)
	if snap.Ratio != 0 || snap.PaddedRatio != 0 {
		t.Fatalf("ratios must be 0 for zero window: ratio=%v padded=%v", snap.Ratio, snap.PaddedRatio)
	}
	if snap.State != BudgetOK {
		t.Fatalf("State = %v, want BudgetOK for zero window", snap.State)
	}
	// Negative window behaves the same (defensive).
	snap = m.Evaluate(msgs, nil, -5)
	if snap.State != BudgetOK {
		t.Fatalf("State = %v, want BudgetOK for negative window", snap.State)
	}
}

// TestEvaluate_IncludesToolDefinitions confirms tool-definition tokens are
// counted against the budget (they consume context in provider requests).
func TestEvaluate_IncludesToolDefinitions(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	msgs := []Message{{Role: RoleUser, Content: "hi"}}
	without := m.Evaluate(msgs, nil, 100000).Tokens
	with := m.Evaluate(msgs, toolDefinitionsFixture(), 100000).Tokens
	if with <= without {
		t.Fatalf("tool defs must add tokens: without=%d with=%d", without, with)
	}
}

// TestEvaluate_CJKPadding confirms the drift-tolerant padded estimate for
// CJK-heavy content exceeds that of ASCII content of the same byte length —
// the scenario where padding matters most (heuristic underestimates CJK).
func TestEvaluate_CJKPadding(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	chineseMsgs := []Message{{Role: RoleUser, Content: strings.Repeat("你", 100)}} // 300 bytes
	asciiMsgs := []Message{{Role: RoleUser, Content: strings.Repeat("a", 300)}}   // 300 bytes
	cjkPadded := m.Evaluate(chineseMsgs, nil, 100000).PaddedTokens
	asciiPadded := m.Evaluate(asciiMsgs, nil, 100000).PaddedTokens
	if cjkPadded <= asciiPadded {
		t.Fatalf("CJK padded estimate should exceed ASCII of same byte length: cjk=%d ascii=%d",
			cjkPadded, asciiPadded)
	}
}

// TestEvaluate_NilMessages handles an empty conversation without panic.
// EstimateMessagesTokens carries a constant framing overhead (3 tokens) even
// for an empty slice, which the manager faithfully reports.
func TestEvaluate_NilMessages(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	snap := m.Evaluate(nil, nil, 1000)
	// 3 framing tokens, padded to 4.
	if snap.Tokens != 3 || snap.PaddedTokens != 4 {
		t.Fatalf("nil messages: Tokens=%d PaddedTokens=%d, want 3/4", snap.Tokens, snap.PaddedTokens)
	}
	if snap.State != BudgetOK {
		t.Fatalf("State = %v, want BudgetOK", snap.State)
	}
}

// TestEvaluate_StateFromPaddedRatio documents the key design decision: the
// state machine keys off the PADDED ratio (drift-tolerant upper bound), not
// the raw ratio. This makes Mady warn/block before real provider overflow.
func TestEvaluate_StateFromPaddedRatio(t *testing.T) {
	m := NewTokenBudgetManager(DefaultBudgetConfig())
	// Raw ratio ~0.63 (ok by raw), but padded ratio ~0.84 (warning by padded).
	content := strings.Repeat("a", 221)
	msgs := []Message{{Role: RoleUser, Content: content}}
	snap := m.Evaluate(msgs, nil, 100)
	if snap.Ratio >= 0.8 {
		t.Fatalf("raw Ratio %v should be < 0.8 (ok band)", snap.Ratio)
	}
	if snap.State != BudgetWarning {
		t.Fatalf("State must follow PADDED ratio: got %v, want BudgetWarning", snap.State)
	}
}

// toolDefinitionsFixture returns a small set of tool definitions for tests.
func toolDefinitionsFixture() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}
