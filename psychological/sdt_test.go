package psychological

import (
	"math"
	"testing"
)

func TestSDTInitialState(t *testing.T) {
	tracker := NewSDTTracker(nil)
	state := tracker.GetState()
	if math.Abs(state.Autonomy-0.5) > 0.01 {
		t.Errorf("expected autonomy 0.5, got %f", state.Autonomy)
	}
	if math.Abs(state.Competence-0.5) > 0.01 {
		t.Errorf("expected competence 0.5, got %f", state.Competence)
	}
}

func TestSDTUpdateAutonomyFrustration(t *testing.T) {
	tracker := NewSDTTracker(nil)
	signals := SDTSignals{
		AutonomyFrustration: 0.8,
		PerceivedDifficulty: 0.5,
	}
	state := tracker.UpdateFromSignals(signals)
	if state.Autonomy >= 0.5 {
		t.Errorf("expected autonomy to decrease from 0.5 after frustration, got %f", state.Autonomy)
	}
}

func TestSDTCompetenceUpdateFormula(t *testing.T) {
	tracker := NewSDTTracker(nil)
	// 难度略高于胜任感 → 应有适度增长
	signals := SDTSignals{
		PerceivedDifficulty: 0.55,
	}
	state := tracker.UpdateFromSignals(signals)
	// Deterding 公式下，gap=0.05 时变化很小
	if math.Abs(state.Competence-0.5) > 0.1 {
		t.Errorf("expected small competence change for small gap, got %f", state.Competence)
	}
}

func TestSDTMultiRoundDecay(t *testing.T) {
	tracker := NewSDTTracker(nil)
	// 手动设置高胜任感
	tracker.mu.Lock()
	tracker.state.Competence = 0.9
	tracker.mu.Unlock()
	for i := 0; i < 20; i++ {
		tracker.UpdateFromSignals(SDTSignals{PerceivedDifficulty: 0.5})
	}
	state := tracker.GetState()
	// 经过 20 轮衰减，应向 0.5 回归
	if state.Competence > 0.8 {
		t.Errorf("expected competence to decay toward 0.5 after 20 rounds, got %f", state.Competence)
	}
}

func TestSDTRestore(t *testing.T) {
	tracker := NewSDTTracker(nil)
	saved := SDTState{Autonomy: 0.7, Competence: 0.3, Relatedness: 0.6, Motivation: 0.53}
	tracker.RestoreState(saved, 10)
	state := tracker.GetState()
	if state.Autonomy != 0.7 {
		t.Errorf("expected restored autonomy 0.7, got %f", state.Autonomy)
	}
	if tracker.RoundCount() != 10 {
		t.Errorf("expected round 10, got %d", tracker.RoundCount())
	}
}

func TestSDTRelatednessUpdate(t *testing.T) {
	tracker := NewSDTTracker(nil)
	// 孤独信号
	state := tracker.UpdateFromSignals(SDTSignals{
		RelatednessLoneliness: 0.6,
		PerceivedDifficulty:   0.5,
	})
	if state.Relatedness >= 0.5 {
		t.Errorf("expected relatedness to decrease after loneliness, got %f", state.Relatedness)
	}
}

func TestSDTLowestNeed(t *testing.T) {
	tracker := NewSDTTracker(nil)
	tracker.mu.Lock()
	tracker.state = SDTState{Autonomy: 0.8, Competence: 0.3, Relatedness: 0.6, Motivation: 0.56}
	tracker.mu.Unlock()
	need := tracker.LowestNeed()
	if need != "competence" {
		t.Errorf("expected lowest need competence, got %s", need)
	}
}

func TestSDTReset(t *testing.T) {
	tracker := NewSDTTracker(nil)
	tracker.UpdateFromSignals(SDTSignals{AutonomyFrustration: 0.9, PerceivedDifficulty: 0.5})
	tracker.Reset()
	state := tracker.GetState()
	if math.Abs(state.Autonomy-0.5) > 0.01 {
		t.Errorf("expected reset autonomy 0.5, got %f", state.Autonomy)
	}
	if tracker.RoundCount() != 0 {
		t.Errorf("expected reset round 0, got %d", tracker.RoundCount())
	}
}
