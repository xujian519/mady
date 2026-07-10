package psychological

import (
	"strings"
	"testing"
)

func TestExecuteFullPipelineBasic(t *testing.T) {
	result := ExecuteFullPipeline("今天专利申请顺利通过了，非常开心！", nil)
	if len(result.Metadata.Steps) != 7 {
		t.Errorf("expected 7 steps, got %d: %v", len(result.Metadata.Steps), result.Metadata.Steps)
	}
	if result.Understanding.VAD.Valence <= 0 {
		t.Errorf("expected positive valence for happy message, got %f", result.Understanding.VAD.Valence)
	}
	if result.Strategy.Primary == "" {
		t.Errorf("expected non-empty strategy")
	}
	if result.Metadata.Version != "v2" {
		t.Errorf("expected version v2, got %s", result.Metadata.Version)
	}
}

func TestExecuteFullPipelineNegative(t *testing.T) {
	result := ExecuteFullPipeline("完蛋了，专利被驳回了，我完全失败了，都怪我太差劲", nil)
	if result.Understanding.VAD.Valence >= 0 {
		t.Errorf("expected negative valence for distressed message, got %f", result.Understanding.VAD.Valence)
	}
	if len(result.Understanding.Distortions.Distortions) == 0 {
		t.Errorf("expected cognitive distortions in distressed message")
	}
}

func TestExecuteFullPipelineNeutral(t *testing.T) {
	result := ExecuteFullPipeline("帮我查一下今天的天气", nil)
	if result.Metadata.Confidence < 0 || result.Metadata.Confidence > 1 {
		t.Errorf("expected confidence in [0,1], got %f", result.Metadata.Confidence)
	}
}

func TestExecuteFullPipelineWithTracker(t *testing.T) {
	tracker := NewSDTTracker(nil)
	// 多轮对话，追踪状态变化
	for i := 0; i < 3; i++ {
		result := ExecuteFullPipeline("今天心情不错", &PipelineConfig{SDTTracker: tracker})
		if len(result.Metadata.Steps) != 7 {
			t.Errorf("round %d: expected 7 steps, got %d", i, len(result.Metadata.Steps))
		}
	}
	if tracker.RoundCount() != 3 {
		t.Errorf("expected 3 rounds, got %d", tracker.RoundCount())
	}
}

func TestExecuteFullPipelineSkipDistortion(t *testing.T) {
	result := ExecuteFullPipeline("我应该做得更好，本不该犯这个错误", &PipelineConfig{
		SkipDistortionDetection: true,
	})
	if len(result.Understanding.Distortions.Distortions) != 0 {
		t.Errorf("expected no distortions when detection skipped, got %v", result.Understanding.Distortions.Distortions)
	}
}

func TestBuildContextBlock(t *testing.T) {
	result := ExecuteFullPipeline("我最近感觉很困惑，不知道该怎么办", nil)
	block := BuildContextBlock(result)
	if len(block) == 0 {
		t.Errorf("expected non-empty context block")
	}
	// 应包含关键部分
	if !strings.Contains(block, "【当前感知的用户心理状态】") {
		t.Errorf("expected context block to start with header")
	}
	if !strings.Contains(block, "【对话策略】") {
		t.Errorf("expected strategy section in context block")
	}
}

func TestBoolToFloat(t *testing.T) {
	if boolToFloat(true) != 1 {
		t.Errorf("expected 1 for true")
	}
	if boolToFloat(false) != 0 {
		t.Errorf("expected 0 for false")
	}
}
