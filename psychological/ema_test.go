package psychological

import "testing"

func TestComputeEMAPositiveProblemFocused(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: 0.8, Likelihood: 0.7, Unexpectedness: 0.2,
		Controllability: 0.9, CausalAttribution: 0.0,
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeProblemFocused {
		t.Errorf("expected problem_focused, got %s", ema.CopingMode)
	}
	if ema.GoalCongruence < 0.5 {
		t.Errorf("expected positive congruence, got %f", ema.GoalCongruence)
	}
}

func TestComputeEMANegativeReappraisal(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.7, Likelihood: 0.6, Unexpectedness: 0.3,
		Controllability: 0.8, CausalAttribution: 0.0,
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeReappraisal {
		t.Errorf("expected reappraisal, got %s", ema.CopingMode)
	}
}

func TestComputeEMANegativeAvoidance(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.8, Likelihood: 0.3, Unexpectedness: 0.5,
		Controllability: 0.2, CausalAttribution: 0.7, // 归因他人
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeAvoidance {
		t.Errorf("expected avoidance, got %s", ema.CopingMode)
	}
}

func TestComputeEMANegativeEmotionFocused(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.7, Likelihood: 0.3, Unexpectedness: 0.2,
		Controllability: 0.2, CausalAttribution: -0.5, // 归因自己
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeEmotionFocused {
		t.Errorf("expected emotion_focused, got %s", ema.CopingMode)
	}
}

func TestEMAtoOCCEmotionsJoyPride(t *testing.T) {
	ema := EMAAssessment{
		GoalRelevance: 0.7, GoalCongruence: 0.6,
		CopingPotential: 0.8, Agency: -0.5,
		FutureExpectancy: 0.3,
	}
	result := emaToOCCEmotions(ema)
	if result[EmoJoy] <= 0 {
		t.Errorf("expected joy from positive congruence, got %f", result[EmoJoy])
	}
	if result[EmoPride] <= 0 {
		t.Errorf("expected pride from self-agency + positive, got %f", result[EmoPride])
	}
}

func TestEMAtoOCCEmotionsDistressAnger(t *testing.T) {
	ema := EMAAssessment{
		GoalRelevance: 0.8, GoalCongruence: -0.7,
		CopingPotential: 0.3, Agency: 0.6, // 归因他人+负面
		FutureExpectancy: -0.4,
	}
	result := emaToOCCEmotions(ema)
	if result[EmoDistress] <= 0 {
		t.Errorf("expected distress from negative congruence, got %f", result[EmoDistress])
	}
	if result[EmoFear] <= 0 {
		t.Errorf("expected fear from negative future expectancy, got %f", result[EmoFear])
	}
	if result[EmoAnger] <= 0 {
		t.Errorf("expected anger from other-agency + negative, got %f", result[EmoAnger])
	}
}
