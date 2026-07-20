package psychological

import (
	"strings"
	"testing"
)

func TestExecuteFullPipeline(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantValence  string // "positive", "negative", "neutral"
		wantStrategy string
	}{
		{
			name:         "positive text",
			text:         "谢谢你的帮助，这个分析非常专业，我很满意！",
			wantValence:  "positive",
			wantStrategy: string(StrategyNeutral),
		},
		{
			name:         "negative anxious text",
			text:         "这太让人失望了，老是驳回我的意见，我真的很担心很害怕！",
			wantValence:  "negative",
			wantStrategy: string(StrategyEmpathetic),
		},
		{
			name:         "neutral text",
			text:         "请帮我查询这篇专利的法律状态。",
			wantValence:  "neutral",
			wantStrategy: string(StrategyNeutral),
		},
		{
			name:         "frustrated urgent text",
			text:         "这个很重要很紧急！我很担心这个案子会被驳回，不知道该怎么办，太让人崩溃了。",
			wantValence:  "negative",
			wantStrategy: string(StrategyCalming),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExecuteFullPipeline(tt.text, nil)

			// Check valence
			v := result.Emotion.VAD.Valence
			switch tt.wantValence {
			case "positive":
				if v <= 0 {
					t.Errorf("expected positive valence, got %.2f", v)
				}
			case "negative":
				if v >= 0 {
					t.Errorf("expected negative valence, got %.2f", v)
				}
			}

			// Check strategy
			if string(result.Strategy.Primary) != tt.wantStrategy {
				t.Errorf("expected strategy %s, got %s", tt.wantStrategy, result.Strategy.Primary)
			}

			// Check dominant emotion is set
			if result.Emotion.DominantEmotion == "" {
				t.Error("dominant emotion should not be empty")
			}

			// Check confidence
			if result.Metadata.Confidence < 0.5 || result.Metadata.Confidence > 1.0 {
				t.Errorf("confidence %.2f out of range [0.5, 1.0]", result.Metadata.Confidence)
			}

			t.Logf("text=%q, VAD=(%.2f,%.2f,%.2f), emotion=%s, strategy=%s, confidence=%.2f",
				tt.text, v, result.Emotion.VAD.Arousal, result.Emotion.VAD.Dominance,
				result.Emotion.DominantEmotion, result.Strategy.Primary, result.Metadata.Confidence)
		})
	}
}

func TestBuildContextBlock(t *testing.T) {
	result := ExecuteFullPipeline("我很担心这个案子被驳回", nil)
	block := BuildContextBlock(result)

	if !strings.Contains(block, "【当前感知的用户心理状态】") {
		t.Error("context block should contain psychological state header")
	}
	if !strings.Contains(block, "【对话策略】") {
		t.Error("context block should contain strategy header")
	}
	if !strings.Contains(block, "VAD") {
		t.Error("context block should contain VAD values")
	}
}

func TestPipelineConfig(t *testing.T) {
	cfg := &PipelineConfig{SkipDistortionDetection: true}
	result := ExecuteFullPipeline("test", cfg)
	if result.Emotion.DominantEmotion == "" {
		t.Error("dominant emotion should be set even with config")
	}
}
