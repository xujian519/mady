package psychological

import "testing"

func TestDetectCatastrophizing(t *testing.T) {
	d := detectDistortions("这个案子完蛋了，万一被驳回就全完了")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistCatastrophizing {
			found = true
		}
	}
	if !found {
		t.Errorf("expected catastrophizing, got %v", d.Distortions)
	}
}

func TestDetectShouldStatements(t *testing.T) {
	d := detectDistortions("我应该做得更好，本不该犯这个错误")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistShouldStatements {
			found = true
		}
	}
	if !found {
		t.Errorf("expected should_statements, got %v", d.Distortions)
	}
}

func TestDetectAllOrNothing(t *testing.T) {
	d := detectDistortions("我完全失败了，彻底没希望了")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistAllOrNothing {
			found = true
		}
	}
	if !found {
		t.Errorf("expected all_or_nothing, got %v", d.Distortions)
	}
}

func TestDetectLabeling(t *testing.T) {
	d := detectDistortions("我真是个废物，太差劲了")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistLabeling {
			found = true
		}
	}
	if !found {
		t.Errorf("expected labeling, got %v", d.Distortions)
	}
}

func TestDetectPersonalization(t *testing.T) {
	d := detectDistortions("都怪我，全是我的错")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistPersonalization {
			found = true
		}
	}
	if !found {
		t.Errorf("expected personalization, got %v", d.Distortions)
	}
}

func TestNoDistortion(t *testing.T) {
	d := detectDistortions("今天的天气很好，适合出门散步")
	if len(d.Distortions) > 0 {
		t.Errorf("expected no distortions, got %v", d.Distortions)
	}
}

func TestHasSevereDistortionMultiple(t *testing.T) {
	d := DistortionDetection{
		Distortions:      []CognitiveDistortion{DistCatastrophizing, DistLabeling, DistPersonalization},
		BeliefIntensity:  0.8,
	}
	if !hasSevereDistortion(d) {
		t.Errorf("expected severe distortion (multiple)")
	}
}

func TestHasSevereDistortionHighIntensity(t *testing.T) {
	d := DistortionDetection{
		Distortions:      []CognitiveDistortion{DistShouldStatements},
		BeliefIntensity:  0.9,
	}
	if !hasSevereDistortion(d) {
		t.Errorf("expected severe distortion (high intensity)")
	}
}

func TestHasSevereDistortionKeyType(t *testing.T) {
	d := DistortionDetection{
		Distortions:      []CognitiveDistortion{DistCatastrophizing},
		BeliefIntensity:  0.3,
	}
	if !hasSevereDistortion(d) {
		t.Errorf("expected severe distortion (catastrophizing)")
	}
}

func TestGenerateReframes(t *testing.T) {
	d := detectDistortions("我完全失败了，完蛋了")
	reframes := generateReframes(d.Distortions)
	if len(reframes) == 0 {
		t.Errorf("expected reframes for detected distortions")
	}
	for _, r := range reframes {
		if r.Reframe == "" {
			t.Errorf("expected non-empty reframe for %s", r.Distortion)
		}
	}
}
