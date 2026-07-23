package autoresearch

import (
	"context"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Existing tests (preserved)
// =============================================================================

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
	// Sleep ensures the timer fires even under race-detector slowdown.
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

// =============================================================================
// New tests for P0 fixes
// =============================================================================

func TestAbort(t *testing.T) {
	c := NewResearchContract("ar-abort-1", "测试中止", "patent")
	c.MaxRounds = 10
	c.Start()

	c.AddEvidence(Evidence{
		Round:     1,
		Summary:   "第一轮研究",
		Findings:  []string{"发现1"},
		ToolsUsed: []string{"patent_search"},
	})

	c.Abort("审查员发出驳回决定，无需继续检索")

	if c.Status != StatusAborted {
		t.Errorf("after abort: got %s, want aborted", c.Status)
	}
	if c.AbortReason != "审查员发出驳回决定，无需继续检索" {
		t.Errorf("abort reason: got %q, want %q", c.AbortReason, "审查员发出驳回决定，无需继续检索")
	}
	if c.CompletedAt == nil {
		t.Error("CompletedAt should be set after abort")
	}
}

func TestAbortBlocksAllCriteriaMet(t *testing.T) {
	c := NewResearchContract("ar-abort-2", "测试", "legal")
	c.Start()

	c.SuccessCriteria = []SuccessCriterion{
		{Description: "标准1", Met: true},
		{Description: "标准2", Met: true},
	}

	if !c.AllCriteriaMet() {
		t.Error("all criteria should be met before abort")
	}

	c.Abort("外部中断")

	if c.AllCriteriaMet() {
		t.Error("aborted contract should not report all criteria met")
	}
}

func TestIsExpiredBeforeStart(t *testing.T) {
	c := NewResearchContract("ar-exp-1", "测试", "patent")
	c.MaxDuration = 1 * time.Hour

	if c.IsExpired() {
		t.Error("unstarted contract should not be expired")
	}
}

// =============================================================================
// New tests for P1 fixes: time-based expiry, evidence, cap, accessors, defaults
// =============================================================================

func TestTimeBasedExpiry(t *testing.T) {
	c := NewResearchContract("ar-exp-2", "测试", "legal")
	c.Start()
	c.MaxDuration = 1 * time.Nanosecond
	// Allow a tiny amount of time to elapse so the timer fires.
	time.Sleep(time.Microsecond)

	if !c.IsExpired() {
		t.Error("expected expiry after max duration")
	}
}

func TestAddEvidence(t *testing.T) {
	c := NewResearchContract("ar-ev-1", "测试", "patent")
	c.Start()
	c.AdvanceRound()

	ev := Evidence{
		Round:     1,
		Summary:   "第一轮关键词检索",
		Findings:  []string{"专利A公开了特征X", "专利B公开了特征Y"},
		ToolsUsed: []string{"google_patents", "cnipa_query"},
	}
	c.AddEvidence(ev)

	if len(c.Evidence) != 1 {
		t.Errorf("evidence count: got %d, want 1", len(c.Evidence))
	}
	if c.Evidence[0].Round != 1 {
		t.Errorf("evidence round: got %d, want 1", c.Evidence[0].Round)
	}
	if len(c.Evidence[0].Findings) != 2 {
		t.Errorf("findings count: got %d, want 2", len(c.Evidence[0].Findings))
	}
}

func TestEvidenceCap(t *testing.T) {
	c := NewResearchContract("ar-cap-1", "测试", "patent")
	c.MaxRounds = 3
	c.Start()

	for i := 0; i < 10; i++ {
		c.AdvanceRound()
		c.AddEvidence(Evidence{
			Round:   i + 1,
			Summary: "第N轮",
		})
	}

	if len(c.Evidence) > 3 {
		t.Errorf("evidence should be capped at MaxRounds=3, got %d", len(c.Evidence))
	}

	// Should contain the most recent entries (rounds 8, 9, 10).
	if c.Evidence[0].Round != 8 {
		t.Errorf("oldest evidence should be round 8, got round %d", c.Evidence[0].Round)
	}
}

func TestIllegalStateTransitions(t *testing.T) {
	t.Run("double start", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-1", "测试", "legal")
		c.Start()
		firstStarted := c.StartedAt
		c.Start() // no-op
		if !c.StartedAt.Equal(firstStarted) {
			t.Error("double start should not overwrite StartedAt")
		}
		if c.Status != StatusRunning {
			t.Error("double start should maintain running status")
		}
	})

	t.Run("double pause", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-2", "测试", "legal")
		c.Start()
		c.Pause()
		firstPaused := *c.PausedAt
		c.Pause() // no-op
		if !c.PausedAt.Equal(firstPaused) {
			t.Error("double pause should not overwrite PausedAt")
		}
	})

	t.Run("complete after abort is no-op", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-3", "测试", "patent")
		c.Start()
		c.Abort("reason")
		c.Complete() // no-op
		if c.Status != StatusAborted {
			t.Error("Complete after aborted should be no-op")
		}
	})

	t.Run("resume from running is no-op", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-4", "测试", "patent")
		c.Start()
		c.Resume() // no-op
		if c.Status != StatusRunning {
			t.Error("Resume from running should maintain running")
		}
	})

	t.Run("pause from idle is no-op", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-5", "测试", "legal")
		c.Pause() // no-op
		if c.Status != StatusIdle {
			t.Error("Pause from idle should be no-op")
		}
	})

	t.Run("complete from idle is no-op", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-6", "测试", "legal")
		c.Complete() // no-op
		if c.Status != StatusIdle {
			t.Error("Complete from idle should be no-op")
		}
	})

	t.Run("abort from idle is no-op", func(t *testing.T) {
		c := NewResearchContract("ar-illegal-7", "测试", "legal")
		c.Abort("test") // no-op
		if c.Status != StatusIdle {
			t.Error("Abort from idle should be no-op")
		}
	})
}

func TestPausedAtCompletedAtAccessors(t *testing.T) {
	c := NewResearchContract("ar-access-1", "测试", "patent")

	_, ok := c.PausedAtTime()
	if ok {
		t.Error("PausedAtTime before pause should return false")
	}

	c.Start()
	c.Pause()
	pausedAt, ok := c.PausedAtTime()
	if !ok {
		t.Error("PausedAtTime after pause should return true")
	}
	if pausedAt.IsZero() {
		t.Error("PausedAtTime should return non-zero time")
	}

	_, ok = c.CompletedAtTime()
	if ok {
		t.Error("CompletedAtTime before completion should return false")
	}

	c.Resume()
	c.Complete()
	completedAt, ok := c.CompletedAtTime()
	if !ok {
		t.Error("CompletedAtTime after complete should return true")
	}
	if completedAt.IsZero() {
		t.Error("CompletedAtTime should return non-zero time")
	}
}

func TestNewContractDefaults(t *testing.T) {
	c := NewResearchContract("ar-def-1", "测试", "patent")

	if c.MaxRounds != 20 {
		t.Errorf("default MaxRounds: got %d, want 20", c.MaxRounds)
	}
	if c.MaxDuration != 30*time.Minute {
		t.Errorf("default MaxDuration: got %v, want 30m", c.MaxDuration)
	}
	if c.Status != StatusIdle {
		t.Errorf("default status: got %s, want idle", c.Status)
	}
}

func TestDomainValidation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"patent", "patent"},
		{"legal", "legal"},
		{"general", "general"},
		{"unknown", "general"},
		{"", "general"},
	}

	for _, tt := range tests {
		c := NewResearchContract("ar-dom-1", "测试", tt.input)
		if c.Domain != tt.expected {
			t.Errorf("domain %q: got %q, want %q", tt.input, c.Domain, tt.expected)
		}
	}
}

// =============================================================================
// Heartbeat full-path test
// =============================================================================

func TestHeartbeatFullPath(t *testing.T) {
	h := NewHeartbeat("ar-hb-1", 5*time.Second, 10*time.Second)

	// Initial state: fresh heartbeat should not be stale.
	if h.IsStale {
		t.Error("fresh heartbeat should not be stale")
	}

	// Beat.
	h.Beat()
	if h.BeatCount != 1 {
		t.Errorf("beat count: got %d, want 1", h.BeatCount)
	}
	if h.IsStale {
		t.Error("after beat should not be stale")
	}

	// Check within timeout: should not be stale.
	h.Check()
	if h.IsStale {
		t.Error("within timeout should not be stale")
	}

	// SinceLastBeat should be positive.
	since := h.SinceLastBeat()
	if since < 0 {
		t.Errorf("SinceLastBeat should be positive, got %v", since)
	}
}

// =============================================================================
// Concurrent safety test: must pass -race
// =============================================================================

func TestConcurrentSafety(t *testing.T) {
	c := NewResearchContract("ar-con-1", "测试", "patent")
	c.Start()
	c.MaxRounds = 1000

	var wg sync.WaitGroup
	wg.Add(3)

	// Writer 1: advance rounds.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.AdvanceRound()
		}
	}()

	// Writer 2: add evidence and record direction changes.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.AddEvidence(Evidence{Summary: "test"})
			c.RecordDirectionChange("A", "B", "test")
		}
	}()

	// Reader: read-only operations.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = c.IsExpired()
			_ = c.AllCriteriaMet()
			_ = c.DirectionPivotCount()
			_, _ = c.PausedAtTime()
			_, _ = c.CompletedAtTime()
		}
	}()

	wg.Wait()

	// Verify no corruption occurred.
	if c.CurrentRound != 100 {
		t.Errorf("concurrent advance: got %d, want 100", c.CurrentRound)
	}
}

func TestContractIDAccessor(t *testing.T) {
	c := NewResearchContract("ar-cid-1", "测试ID", "patent")
	if c.ContractID() != "ar-cid-1" {
		t.Errorf("ContractID: got %q, want %q", c.ContractID(), "ar-cid-1")
	}
}

func TestCreateHeartbeat(t *testing.T) {
	c := NewResearchContract("ar-hb-create-1", "测试创建心跳", "patent")
	h := c.CreateHeartbeat(5*time.Second, 30*time.Second)
	if h.ContractID != "ar-hb-create-1" {
		t.Errorf("heartbeat ContractID: got %q, want %q",
			h.ContractID, "ar-hb-create-1")
	}
	if h.Interval != 5*time.Second {
		t.Errorf("interval: got %v, want 5s", h.Interval)
	}
}

func TestTruncateString(t *testing.T) {
	// Short string: no truncation.
	short := "hello"
	if got := truncateString(short, 10); got != short {
		t.Errorf("short: got %q, want %q", got, short)
	}

	// Long string: truncation occurs.
	long := "this is a very long string that needs truncation"
	got := truncateString(long, 10)
	if len(got) != 13 { // 10 chars + "..."
		t.Errorf("truncated len: got %d, want 13", len(got))
	}
	if got != "this is a ..." {
		t.Errorf("truncated: got %q, want %q", got, "this is a ...")
	}
}

func TestEvidenceTrimmedLogging(t *testing.T) {
	// Trigger the "evidence trimmed" debug path by exceeding the cap.
	c := NewResearchContract("ar-trim-1", "测试裁剪日志", "patent")
	c.MaxRounds = 2
	c.Start()

	for i := 0; i < 5; i++ {
		c.AdvanceRound()
		c.AddEvidence(Evidence{Summary: "test"})
	}
	if len(c.Evidence) > 2 {
		t.Errorf("evidence should be capped at 2, got %d", len(c.Evidence))
	}
}

// TestDeepCopyAllFields verifies that deepCopyContract handles all optional
// fields (PausedAt, CompletedAt, SuccessCriteria, Directions).
func TestDeepCopyAllFields(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	c := NewResearchContract("ar-deep-all-1", "全字段深拷贝", "patent")
	c.Start()
	c.Pause()
	c.Resume()
	c.Complete()

	c.SuccessCriteria = []SuccessCriterion{
		{Description: "标准1", Met: true, Evidence: "已满足"},
	}
	c.RecordDirectionChange("策略A", "策略B", "方向变化")

	if err := s.SaveContract(ctx, c); err != nil {
		t.Fatalf("SaveContract: %v", err)
	}

	loaded, err := s.LoadContract(ctx, "ar-deep-all-1")
	if err != nil {
		t.Fatalf("LoadContract: %v", err)
	}

	// Verify PausedAt was deep-copied.
	_, pausedOK := loaded.PausedAtTime()
	if !pausedOK {
		t.Error("PausedAt should survive round-trip")
	}

	// Verify CompletedAt was deep-copied.
	_, completedOK := loaded.CompletedAtTime()
	if !completedOK {
		t.Error("CompletedAt should survive round-trip")
	}

	// Verify SuccessCriteria was deep-copied.
	if len(loaded.SuccessCriteria) != 1 {
		t.Fatalf("criteria count: got %d, want 1", len(loaded.SuccessCriteria))
	}
	if loaded.SuccessCriteria[0].Description != "标准1" {
		t.Errorf("criteria desc: got %q", loaded.SuccessCriteria[0].Description)
	}

	// Verify Directions was deep-copied.
	if len(loaded.Directions) != 1 {
		t.Fatalf("directions count: got %d, want 1", len(loaded.Directions))
	}
	if loaded.Directions[0].From != "策略A" {
		t.Errorf("direction From: got %q", loaded.Directions[0].From)
	}

	// Mutate original to confirm isolation.
	c.SuccessCriteria[0].Description = "已篡改"
	if loaded.SuccessCriteria[0].Description != "标准1" {
		t.Error("deep copy should isolate SuccessCriteria")
	}
}
