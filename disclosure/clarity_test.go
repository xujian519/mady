package disclosure

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

func TestDetectSignals(t *testing.T) {
	text := "本发明提供一种智能传感器。现有技术存在测量不足的问题。有益效果在于提高了精度。本领域技术人员可照实施例实现。"
	c := DetectSignals(text)
	if !c.Solution || !c.Problem || !c.Effect || !c.Enablement {
		t.Fatalf("expected all four signals, got %+v", c)
	}
}

func TestDetectSignals_Missing(t *testing.T) {
	c := DetectSignals("只有噪声")
	if c.Solution || c.Problem || c.Effect || c.Enablement {
		t.Fatalf("expected no signals for noise, got %+v", c)
	}
}

func TestSignalWeightedScore(t *testing.T) {
	if got := SignalWeightedScore(SignalCounts{Solution: true}); got != 0.3 {
		t.Errorf("solution weight = %v, want 0.3", got)
	}
	if got := SignalWeightedScore(SignalCounts{Problem: true, Effect: true}); got != 0.5 {
		t.Errorf("problem+effect weight = %v, want 0.5", got)
	}
	if got := SignalWeightedScore(SignalCounts{Problem: true, Solution: true, Effect: true, Enablement: true}); got != 1.0 {
		t.Errorf("all signals weight = %v, want 1.0", got)
	}
}

func TestComputeClarity(t *testing.T) {
	full := "本发明提供一种智能传感器。现有技术存在测量不足的问题。有益效果在于提高了精度。本领域技术人员可照实施例实现。"
	if got := ComputeClarity(0, full); got != 0.25 {
		t.Errorf("semantic 0 + all signals = 0.25, got %v", got)
	}
	if got := ComputeClarity(0.5, "只有噪声"); got != 0.375 {
		t.Errorf("semantic 0.5 + no signal = 0.375, got %v", got)
	}
}

func TestClarityNode_BelowThresholdInterrupts(t *testing.T) {
	state := graph.PregelState{}
	SetExtraction(state, &ExtractionResult{
		Problems: []string{"缺乏问题描述"},
		Features: []TechFeature{{Description: "某个特征"}},
		Effects:  []string{},
	})
	_, err := clarityNode()(context.Background(), state)
	if err == nil {
		t.Fatal("expected InterruptError for low clarity")
	}
	if !agentcore.IsInterrupt(err) {
		t.Fatalf("expected agentcore interrupt, got %v", err)
	}
}

func TestClarityNode_AboveThresholdPasses(t *testing.T) {
	state := graph.PregelState{}
	SetExtraction(state, &ExtractionResult{
		Problems: []string{"本发明所要解决的技术问题是提高精度，现有技术存在测量不足的缺陷。"},
		Features: []TechFeature{{Description: "本发明提供一种智能传感器，其特征在于包含导电涂层。"}},
		Effects:  []string{"有益效果在于提高了精度，解决了测量不准的问题。本领域技术人员可照实施例实现。"},
	})
	_, err := clarityNode()(context.Background(), state)
	if err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if score, ok := state[StateKeyClarity].(float64); !ok || score < ClarityThreshold {
		t.Errorf("expected clarity stored above threshold, got %v (state=%v)", score, state[StateKeyClarity])
	}
}
