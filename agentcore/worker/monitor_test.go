package worker

import (
	"testing"
	"time"
)

func TestMonitor_RecordAndStats(t *testing.T) {
	m := NewMonitor()

	// 添加几条记录
	m.Record(ExecutionRecord{
		WorkerName:  "test-worker-1",
		Tier:        TierReasoning,
		Mode:        "llm",
		StartedAt:   time.Now().Add(-5 * time.Second),
		Duration:    2 * time.Second,
		InputValid:  true,
		OutputValid: true,
		Success:     true,
	})
	m.Record(ExecutionRecord{
		WorkerName:  "test-worker-1",
		Tier:        TierReasoning,
		Mode:        "llm",
		StartedAt:   time.Now().Add(-3 * time.Second),
		Duration:    1 * time.Second,
		InputValid:  true,
		OutputValid: false,
		Success:     false,
		Degraded:    true,
		ErrorMsg:    "output validation failed",
	})
	m.Record(ExecutionRecord{
		WorkerName:  "test-worker-2",
		Tier:        TierChecker,
		Mode:        "graph",
		StartedAt:   time.Now().Add(-1 * time.Second),
		Duration:    500 * time.Millisecond,
		InputValid:  true,
		OutputValid: true,
		Success:     true,
	})

	// 验证统计
	stats := m.Stats()
	if len(stats) != 2 {
		t.Fatalf("期望 2 个 Worker 统计, 得到 %d", len(stats))
	}

	// Worker 1
	s1 := stats[0]
	if s1.WorkerName != "test-worker-1" {
		t.Errorf("WorkerName = %q", s1.WorkerName)
	}
	if s1.TotalExecutions != 2 {
		t.Errorf("TotalExecutions = %d, want 2", s1.TotalExecutions)
	}
	if s1.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", s1.SuccessCount)
	}
	if s1.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", s1.FailureCount)
	}
	if s1.DegradedCount != 1 {
		t.Errorf("DegradedCount = %d, want 1", s1.DegradedCount)
	}
	if s1.OutputViolations != 1 {
		t.Errorf("OutputViolations = %d, want 1", s1.OutputViolations)
	}
	if s1.Tier != "reasoning" {
		t.Errorf("Tier = %q, want reasoning", s1.Tier)
	}

	// Worker 2
	s2 := stats[1]
	if s2.TotalExecutions != 1 {
		t.Errorf("TotalExecutions = %d, want 1", s2.TotalExecutions)
	}
	if s2.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", s2.SuccessCount)
	}
}

func TestMonitor_Records(t *testing.T) {
	m := NewMonitor()
	m.Record(ExecutionRecord{WorkerName: "w1", StartedAt: time.Now()})
	m.Record(ExecutionRecord{WorkerName: "w2", StartedAt: time.Now()})

	records := m.Records()
	if len(records) != 2 {
		t.Fatalf("Records() = %d, want 2", len(records))
	}
}

func TestMonitor_Clear(t *testing.T) {
	m := NewMonitor()
	m.Record(ExecutionRecord{WorkerName: "w1"})
	m.Clear()

	records := m.Records()
	if len(records) != 0 {
		t.Fatalf("Clear 后 Records() = %d, want 0", len(records))
	}
}

func TestMonitor_EmptyStats(t *testing.T) {
	m := NewMonitor()
	stats := m.Stats()
	if stats != nil {
		t.Fatalf("空 Monitor 的 Stats 应为 nil, 得到 %+v", stats)
	}

	summary := m.Summary()
	if summary != "Monitor: 无执行记录" {
		t.Errorf("空 Monitor 的 Summary 不正确: %q", summary)
	}
}

func TestMonitor_Summary(t *testing.T) {
	m := NewMonitor()
	m.Record(ExecutionRecord{
		WorkerName: "worker-a", Tier: TierWork, Success: true, Duration: 1 * time.Second,
	})
	m.Record(ExecutionRecord{
		WorkerName: "worker-a", Tier: TierWork, Success: false, Degraded: true, Duration: 2 * time.Second,
	})

	summary := m.Summary()
	if summary == "" {
		t.Fatal("Summary 不应为空")
	}
	t.Logf("Summary:\n%s", summary)
}

func TestMonitor_Callback(t *testing.T) {
	callbackCalled := false
	m := NewMonitor().WithCallback(func(rec ExecutionRecord) {
		callbackCalled = true
		if rec.WorkerName != "cb-worker" {
			t.Errorf("callback WorkerName = %q", rec.WorkerName)
		}
	})

	m.Record(ExecutionRecord{WorkerName: "cb-worker"})
	if !callbackCalled {
		t.Error("callback 未被调用")
	}
}

func TestMonitor_Concurrent(t *testing.T) {
	m := NewMonitor()
	done := make(chan struct{})

	// 并发写入
	go func() {
		for i := 0; i < 50; i++ {
			m.Record(ExecutionRecord{WorkerName: "conc-worker", Duration: time.Duration(i) * time.Millisecond})
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 50; i++ {
			m.Record(ExecutionRecord{WorkerName: "conc-worker", Duration: time.Duration(i) * time.Millisecond})
		}
		done <- struct{}{}
	}()

	<-done
	<-done

	stats := m.Stats()
	if len(stats) != 1 {
		t.Fatalf("期望 1 个 Worker, 得到 %d", len(stats))
	}
	if stats[0].TotalExecutions != 100 {
		t.Errorf("TotalExecutions = %d, want 100", stats[0].TotalExecutions)
	}
}
