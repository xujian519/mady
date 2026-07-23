package agentcore

import (
	"testing"
	"time"
)

// Breaker tests for shouldCompact / compactionState
// ---------------------------------------------------------------------------

func TestShouldCompact_SummaryFailureCooldown(t *testing.T) {
	cs := newCompactionState()
	cs.summaryFailureCooldown = time.Now().Add(time.Hour)

	// Even with a large context window and messages that would normally
	// trigger compaction, the cooldown should prevent it.
	msgs := []Message{
		{Role: RoleUser, Content: string(make([]byte, 1000000))},
	}
	if shouldCompact(msgs, nil, 100000, 0, 0, 0, cs) {
		t.Fatal("should not compact during summary failure cooldown")
	}
}

func TestShouldCompact_SummaryFailureCooldownExpired(t *testing.T) {
	cs := newCompactionState()
	cs.summaryFailureCooldown = time.Now().Add(-time.Second) // Already expired.

	msgs := []Message{
		{Role: RoleUser, Content: string(make([]byte, 1000000))},
	}
	if !shouldCompact(msgs, nil, 100000, 0, 0, 0, cs) {
		t.Fatal("should compact after cooldown has expired")
	}
}

// ---------------------------------------------------------------------------
// Ineffective compaction breaker
// ---------------------------------------------------------------------------

func TestShouldCompact_IneffectiveCompactions_Blocks(t *testing.T) {
	cs := newCompactionState()
	cs.ineffectiveCompactions = 2
	cs.ineffectiveCooldownUntil = time.Now().Add(time.Hour) // Active cooldown.

	// Even with large messages, the breaker should block compaction.
	msgs := []Message{
		{Role: RoleUser, Content: string(make([]byte, 1000000))},
	}
	if shouldCompact(msgs, nil, 100000, 0, 0, 0, cs) {
		t.Fatal("should not compact after 2 ineffective compactions (before cooldown expires)")
	}
}

func TestShouldCompact_IneffectiveCompactions_OneNotEnough(t *testing.T) {
	cs := newCompactionState()
	cs.ineffectiveCompactions = 1 // Only 1 ineffective: still allowed.

	msgs := []Message{
		{Role: RoleUser, Content: string(make([]byte, 1000000))},
	}
	if !shouldCompact(msgs, nil, 100000, 0, 0, 0, cs) {
		t.Fatal("should still compact with only 1 ineffective compaction")
	}
}

func TestShouldCompact_IneffectiveCompactions_RecoversAfterCooldown(t *testing.T) {
	cs := newCompactionState()
	cs.ineffectiveCompactions = 2
	cs.ineffectiveCooldownUntil = time.Now().Add(-time.Second) // Cooldown expired.

	msgs := []Message{
		{Role: RoleUser, Content: string(make([]byte, 1000000))},
	}
	if !shouldCompact(msgs, nil, 100000, 0, 0, 0, cs) {
		t.Fatal("should recover after ineffective compaction cooldown has expired")
	}
}

func TestShouldCompact_IneffectiveCooldownResetOnRun(t *testing.T) {
	cs := newCompactionState()
	cs.ineffectiveCompactions = 3
	cs.ineffectiveCooldownUntil = time.Now().Add(-time.Second) // Cooldown expired.

	// shouldCompact should now allow compaction.
	msgs := []Message{
		{Role: RoleUser, Content: string(make([]byte, 1000000))},
	}
	if !shouldCompact(msgs, nil, 100000, 0, 0, 0, cs) {
		t.Fatal("shouldCompact should allow after cooldown expired")
	}
}

// ---------------------------------------------------------------------------
// helper: verify the cooldown constants are sensible
// ---------------------------------------------------------------------------

func TestCompactionCooldownConstants(t *testing.T) {
	if summaryFailureCooldownSeconds <= 0 {
		t.Error("summaryFailureCooldownSeconds should be positive")
	}
	if ineffectiveCompactionCooldownSeconds <= 0 {
		t.Error("ineffectiveCompactionCooldownSeconds should be positive")
	}
}

func TestCompactionState_InitialState(t *testing.T) {
	cs := newCompactionState()

	if cs.lastSavingsPct != 100.0 {
		t.Errorf("initial lastSavingsPct = %f, want 100.0", cs.lastSavingsPct)
	}
	if cs.ineffectiveCompactions != 0 {
		t.Errorf("initial ineffectiveCompactions = %d, want 0", cs.ineffectiveCompactions)
	}
	if !cs.summaryFailureCooldown.IsZero() {
		t.Error("initial summaryFailureCooldown should be zero")
	}
	if !cs.ineffectiveCooldownUntil.IsZero() {
		t.Error("initial ineffectiveCooldownUntil should be zero")
	}
}
