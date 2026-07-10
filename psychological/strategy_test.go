package psychological

import (
	"strings"
	"testing"
)

func TestMatchStrategyValidation(t *testing.T) {
	input := StrategyInput{
		VAD:             VADVector{Valence: -0.7, Arousal: 0.8, Dominance: 0.3},
		DistortionCount: 2,
		BeliefIntensity: 0.7,
		SDT:             SDTState{Autonomy: 0.5, Competence: 0.3, Relatedness: 0.5, Motivation: 0.43},
		EMA:             EMAAssessment{CopingMode: CopeEmotionFocused},
	}
	result := matchStrategy(input)
	if result.Primary != StrategyValidation && result.Primary != StrategyReframing {
		t.Errorf("expected validation or reframing for negative + distortion, got %s", result.Primary)
	}
	if result.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %f", result.Confidence)
	}
}

func TestMatchStrategyEmpowerment(t *testing.T) {
	input := StrategyInput{
		VAD:             VADVector{Valence: 0, Arousal: 0.3, Dominance: 0.3},
		DistortionCount: 0,
		BeliefIntensity: 0.1,
		SDT:             SDTState{Autonomy: 0.3, Competence: 0.3, Relatedness: 0.5, Motivation: 0.36},
		EMA:             EMAAssessment{CopingMode: CopeEmotionFocused},
	}
	result := matchStrategy(input)
	if result.Primary != StrategyEmpowerment {
		t.Errorf("expected empowerment for low autonomy + low competence, got %s", result.Primary)
	}
}

func TestMatchStrategyLightHumor(t *testing.T) {
	input := StrategyInput{
		VAD:             VADVector{Valence: 0.1, Arousal: 0.2, Dominance: 0.5},
		Dominant:        EmoBoredom,
		DistortionCount: 0,
		BeliefIntensity: 0.0,
		SDT:             SDTState{Autonomy: 0.5, Competence: 0.5, Relatedness: 0.5, Motivation: 0.5},
		EMA:             EMAAssessment{CopingMode: CopeEmotionFocused},
	}
	result := matchStrategy(input)
	if result.Primary != StrategyLightHumor {
		t.Errorf("expected light_humor for boredom + no distortion, got %s", result.Primary)
	}
}

func TestMatchStrategySocratic(t *testing.T) {
	input := StrategyInput{
		VAD:             VADVector{Valence: 0, Arousal: 0.3, Dominance: 0.5},
		DistortionCount: 1,
		BeliefIntensity: 0.3,
		SDT:             SDTState{Autonomy: 0.5, Competence: 0.6, Relatedness: 0.5, Motivation: 0.53},
		EMA:             EMAAssessment{CopingMode: CopeProblemFocused},
	}
	result := matchStrategy(input)
	if result.Confidence < 0 || result.Confidence > 1 {
		t.Errorf("expected confidence in [0,1], got %f", result.Confidence)
	}
}

func TestComplementary(t *testing.T) {
	if isComplementary(StrategyValidation, StrategyRedirectAction) {
		t.Errorf("validation and redirect_action should be incompatible")
	}
	if !isComplementary(StrategyValidation, StrategyEmpowerment) {
		t.Errorf("validation and empowerment should be complementary")
	}
	if isComplementary(StrategyCognitiveRestructuring, StrategyLightHumor) {
		t.Errorf("cognitive_restructuring and light_humor should be incompatible")
	}
}

func TestBuildRationale(t *testing.T) {
	input := StrategyInput{
		DistortionCount: 2,
		SDT:             SDTState{Motivation: 0.3, LastUpdatedNeed: "competence"},
		VAD:             VADVector{Valence: -0.5},
	}
	r := buildRationale(StrategyValidation, input, 0.75)
	if len(r) == 0 {
		t.Errorf("expected non-empty rationale")
	}
	// 应包含关键信息
	if !strings.Contains(r, "validation") {
		t.Errorf("expected strategy name in rationale: %s", r)
	}
	if !strings.Contains(r, "2") {
		t.Errorf("expected distortion count in rationale: %s", r)
	}
}
