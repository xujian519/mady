package psychological

import "testing"

func TestComputeOCCEmotionsJoy(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: 0.8, Unexpectedness: 0.3,
		Likelihood: 0.7, Praiseworthiness: 0.0,
		Appealingness: 0.0, CausalAttribution: 0.0,
		Controllability: 0.5, Deservingness: 0.5,
	}
	intensities := computeOCCEmotions(frame)
	if intensities[EmoJoy] <= 0 {
		t.Errorf("expected positive joy intensity, got %f", intensities[EmoJoy])
	}
	if intensities[EmoDistress] > 0.3 {
		t.Errorf("expected low distress for positive frame, got %f", intensities[EmoDistress])
	}
}

func TestComputeOCCEmotionsDistress(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.7, Unexpectedness: 0.6,
		Likelihood: 0.5, Praiseworthiness: 0.0,
		Appealingness: 0.0, CausalAttribution: 0.0,
		Controllability: 0.3, Deservingness: 0.5,
	}
	intensities := computeOCCEmotions(frame)
	if intensities[EmoDistress] <= 0 {
		t.Errorf("expected positive distress intensity, got %f", intensities[EmoDistress])
	}
}

func TestComputeOCCEmotionsPride(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: 0.8, Likelihood: 0.7, Unexpectedness: 0.2,
		Praiseworthiness: 0.6, CausalAttribution: -0.5, // 自己导致的正面事件
		Deservingness: 0.7, Controllability: 0.5,
	}
	intensities := computeOCCEmotions(frame)
	if intensities[EmoPride] <= 0 {
		t.Errorf("expected positive pride intensity, got %f", intensities[EmoPride])
	}
}

func TestComputeOCCEmotionsAnger(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.7, Likelihood: 0.6, Unexpectedness: 0.5,
		Praiseworthiness: -0.6, CausalAttribution: 0.7, // 他人导致的负面事件
		Deservingness: 0.8, Controllability: 0.3,
	}
	intensities := computeOCCEmotions(frame)
	if intensities[EmoAnger] <= 0 {
		t.Errorf("expected positive anger intensity, got %f", intensities[EmoAnger])
	}
}

func TestGetDominantEmotion(t *testing.T) {
	intensities := map[OCCEmotion]float64{
		EmoJoy:  0.6,
		EmoHope: 0.4,
		EmoFear: 0.05,
	}
	dominant, intensity := getDominantEmotion(intensities)
	if dominant != EmoJoy {
		t.Errorf("expected dominant emotion joy, got %s", dominant)
	}
	if intensity != 0.6 {
		t.Errorf("expected intensity 0.6, got %f", intensity)
	}
}

func TestGetDominantEmotionBelowThreshold(t *testing.T) {
	intensities := map[OCCEmotion]float64{
		EmoJoy: 0.05,
	}
	dominant, intensity := getDominantEmotion(intensities)
	if dominant != "" {
		t.Errorf("expected no dominant emotion (all below threshold), got %s", dominant)
	}
	if intensity != 0 {
		t.Errorf("expected intensity 0, got %f", intensity)
	}
}
