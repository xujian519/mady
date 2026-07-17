package autoresearch

import (
	"testing"
	"time"
)

func TestResearchLifecycle(t *testing.T) {
	c := NewResearchContract("ar-001", "专利无效宣告检索", "patent")
	c.MaxRounds = 5

	if c.Status != StatusIdle {
		t.Errorf("initial status: got %s, want idle", c.Status)
	}

	c.Start()
	if c.Status != StatusRunning {
		t.Errorf("after start: got %s, want running", c.Status)
	}

	c.AdvanceRound()
	if c.CurrentRound != 1 {
		t.Errorf("round: got %d, want 1", c.CurrentRound)
	}

	c.Pause()
	if c.Status != StatusPaused {
		t.Errorf("after pause: got %s, want paused", c.Status)
	}

	c.Resume()
	if c.Status != StatusRunning {
		t.Errorf("after resume: got %s, want running", c.Status)
	}

	c.Complete()
	if c.Status != StatusCompleted {
		t.Errorf("after complete: got %s, want completed", c.Status)
	}
}

func TestResearchExpiry(t *testing.T) {
	c := NewResearchContract("ar-002", "测试", "legal")
	c.MaxRounds = 3
	c.Start()

	c.AdvanceRound() // round 1
	c.AdvanceRound() // round 2
	c.AdvanceRound() // round 3 (== max)

	if !c.IsExpired() {
		t.Error("expected expired after max rounds")
	}
}

func TestHeartbeat(t *testing.T) {
	h := NewHeartbeat("ar-001", 5*time.Second, 30*time.Second)

	h.Beat()
	if h.BeatCount != 1 {
		t.Errorf("beat count: got %d, want 1", h.BeatCount)
	}
	if h.IsStale {
		t.Error("fresh heartbeat should not be stale")
	}

	// Simulate timeout by checking with a very short timeout.
	h2 := NewHeartbeat("ar-002", 5*time.Second, 1*time.Nanosecond)
	time.Sleep(time.Millisecond)
	h2.Check()
	if !h2.IsStale {
		t.Error("heartbeat with nanosecond timeout should be stale")
	}
}

func TestDirectionPivot(t *testing.T) {
	c := NewResearchContract("ar-003", "专利检索", "patent")
	c.Start()

	c.RecordDirectionChange("关键词检索", "语义检索", "关键词命中率低")
	c.RecordDirectionChange("语义检索", "IPC分类检索", "语义噪音大")

	if c.DirectionPivotCount() != 2 {
		t.Errorf("pivot count: got %d, want 2", c.DirectionPivotCount())
	}
}

func TestAllCriteriaMet(t *testing.T) {
	c := NewResearchContract("ar-004", "法律分析", "legal")
	if c.AllCriteriaMet() {
		t.Error("empty criteria should not be all met")
	}

	c.SuccessCriteria = []SuccessCriterion{
		{Description: "找到相关法条", Met: true},
		{Description: "找到相关判例", Met: false},
	}
	if c.AllCriteriaMet() {
		t.Error("not all criteria are met yet")
	}

	c.SuccessCriteria[1].Met = true
	if !c.AllCriteriaMet() {
		t.Error("all criteria should be met")
	}
}
