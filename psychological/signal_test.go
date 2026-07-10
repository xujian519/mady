package psychological

import "testing"

func TestExtractTextualSignalsStrongNegative(t *testing.T) {
	s := extractTextualSignals("这个专利被驳回了，太糟糕了")
	if s.Sentiment > -0.5 {
		t.Errorf("expected strong negative sentiment, got %f", s.Sentiment)
	}
}

func TestExtractTextualSignalsStrongPositive(t *testing.T) {
	s := extractTextualSignals("专利申请顺利通过了，非常开心！")
	if s.Sentiment < 0.5 {
		t.Errorf("expected strong positive sentiment, got %f", s.Sentiment)
	}
}

func TestExtractTextualSignalsUncertainty(t *testing.T) {
	s := extractTextualSignals("我不确定这个方案是否可行，可能需要再想想")
	if s.Uncertainty < 0.5 {
		t.Errorf("expected high uncertainty, got %f", s.Uncertainty)
	}
}

func TestExtractTextualSignalsSelfBlame(t *testing.T) {
	s := extractTextualSignals("都是我的错，我能力不够")
	if s.BlameDirection > -0.3 {
		t.Errorf("expected self-blame (negative), got %f", s.BlameDirection)
	}
}

func TestExtractTextualSignalsOtherBlame(t *testing.T) {
	s := extractTextualSignals("公司的审查员根本不懂这个案子")
	if s.BlameDirection < 0.3 {
		t.Errorf("expected other-blame (positive), got %f", s.BlameDirection)
	}
}

func TestExtractTextualSignalsNoControl(t *testing.T) {
	s := extractTextualSignals("我没办法了，毫无选择")
	if s.PerceivedControl > 0.3 {
		t.Errorf("expected low control, got %f", s.PerceivedControl)
	}
}

func TestExtractTextualSignalsHasControl(t *testing.T) {
	s := extractTextualSignals("我有办法处理这个问题")
	if s.PerceivedControl < 0.6 {
		t.Errorf("expected high control, got %f", s.PerceivedControl)
	}
}

func TestExtractTextualSignalsSurprise(t *testing.T) {
	s := extractTextualSignals("没想到突然收到驳回通知，太意外了")
	if s.SurpriseLevel < 0.5 {
		t.Errorf("expected high surprise, got %f", s.SurpriseLevel)
	}
}

func TestExtractTextualSignalsNeutral(t *testing.T) {
	s := extractTextualSignals("今天是星期三")
	if s.Sentiment != 0 {
		t.Errorf("expected neutral sentiment, got %f", s.Sentiment)
	}
	if s.SurpriseLevel != 0.2 {
		t.Errorf("expected default surprise 0.2, got %f", s.SurpriseLevel)
	}
}

func TestBuildAppraisalFrame(t *testing.T) {
	signals := TextSignals{
		Sentiment: 0.7, Uncertainty: 0.7, BlameDirection: 0.7,
		PerceivedControl: 0.8, SurpriseLevel: 0.2, GoalImportance: 0.8,
	}
	frame := buildAppraisalFrame(signals)
	if frame.Desirability != 0.7 {
		t.Errorf("expected desirability 0.7, got %f", frame.Desirability)
	}
	if frame.Likelihood < 0.29 || frame.Likelihood > 0.31 {
		t.Errorf("expected likelihood ~0.3 (1-uncertainty), got %f", frame.Likelihood)
	}
}
