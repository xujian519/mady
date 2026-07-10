package psychological

import (
	"math"
	"testing"
)

func TestOCCtoVADJoy(t *testing.T) {
	intensities := map[OCCEmotion]float64{EmoJoy: 0.8}
	vad := occToVAD(intensities)
	if math.Abs(vad.Valence-0.81) > 0.01 {
		t.Errorf("expected valence ~0.81, got %f", vad.Valence)
	}
	if math.Abs(vad.Arousal-0.51) > 0.01 {
		t.Errorf("expected arousal ~0.51, got %f", vad.Arousal)
	}
}

func TestOCCtoVADEmpty(t *testing.T) {
	vad := occToVAD(map[OCCEmotion]float64{})
	if vad.Valence != 0 || vad.Arousal != 0.5 || vad.Dominance != 0.5 {
		t.Errorf("expected neutral VAD for empty input, got %+v", vad)
	}
}

func TestOCCtoVADMultiEmotion(t *testing.T) {
	intensities := map[OCCEmotion]float64{
		EmoJoy:  0.6,
		EmoHope: 0.3,
		EmoFear: 0.2,
	}
	vad := occToVAD(intensities)
	// 加权平均应偏向正面（joy 和 hope 权重更高）
	if vad.Valence <= 0 {
		t.Errorf("expected positive valence from joy+hope weighted sum, got %f", vad.Valence)
	}
}

func TestMergeIntensities(t *testing.T) {
	occ := map[OCCEmotion]float64{EmoJoy: 0.5, EmoHope: 0.3}
	ema := map[OCCEmotion]float64{EmoJoy: 0.7, EmoFear: 0.4}
	merged := mergeIntensities(occ, ema)
	if merged[EmoJoy] != 0.7 {
		t.Errorf("expected merged joy 0.7 (max), got %f", merged[EmoJoy])
	}
	if merged[EmoFear] != 0.4 {
		t.Errorf("expected fear from EMA, got %f", merged[EmoFear])
	}
	if merged[EmoHope] != 0.3 {
		t.Errorf("expected hope from OCC, got %f", merged[EmoHope])
	}
}
