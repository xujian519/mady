package agentcore

import (
	"runtime"
	"testing"
	"time"
)

func TestHealthTracker_InitiallyHealthy(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	if !ht.IsHealthy("deepseek-v4-flash") {
		t.Fatal("new model should be healthy")
	}
}

func TestHealthTracker_RecordSuccess(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	ht.RecordSuccess("deepseek-v4-flash")
	if !ht.IsHealthy("deepseek-v4-flash") {
		t.Fatal("model should be healthy after success")
	}
	d := ht.DetailOf("deepseek-v4-flash")
	if d == nil {
		t.Fatal("expected detail")
	}
	if d.TotalCalls != 1 {
		t.Fatalf("total calls = %d, want 1", d.TotalCalls)
	}
	if d.ConsecutiveFails != 0 {
		t.Fatalf("consecutive fails = %d, want 0", d.ConsecutiveFails)
	}
}

func TestHealthTracker_DegradeAfterConsecutiveFailures(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 3,
		DegradeDuration:          30 * time.Second,
	})
	// 前两次失败：仍健康
	ht.RecordFailure("model-a", false)
	ht.RecordFailure("model-a", false)
	if !ht.IsHealthy("model-a") {
		t.Fatal("model should still be healthy after 2 failures (< threshold)")
	}
	// 第三次失败：触发降级
	ht.RecordFailure("model-a", false)
	if ht.IsHealthy("model-a") {
		t.Fatal("model should be degraded after 3 consecutive failures")
	}
	if ht.HealthOf("model-a") != HealthStatusDegraded {
		t.Fatalf("status = %s, want degraded", ht.HealthOf("model-a"))
	}
}

func TestHealthTracker_NonRetryableErrorImmediateDegrade(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	ht.RecordFailure("model-b", true) // non-retryable
	if ht.IsHealthy("model-b") {
		t.Fatal("non-retryable error should immediately degrade")
	}
}

func TestHealthTracker_RecoveryAfterSuccess(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 2,
		DegradeDuration:          1 * time.Hour, // 长降级期，确保用 SuccessRecoveryCount 恢复
		SuccessRecoveryCount:     2,
	})
	ht.RecordFailure("model-c", false)
	ht.RecordFailure("model-c", false)
	if ht.IsHealthy("model-c") {
		t.Fatal("model should be degraded after 2 failures")
	}
	// 连续成功 2 次恢复
	ht.RecordSuccess("model-c")
	if ht.IsHealthy("model-c") {
		t.Fatal("model should still be degraded after 1 success (need 2)")
	}
	ht.RecordSuccess("model-c")
	if !ht.IsHealthy("model-c") {
		t.Fatal("model should be healthy after 2 consecutive successes")
	}
}

func TestHealthTracker_MultipleModelsIndependent(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 2,
		DegradeDuration:          5 * time.Minute,
	})
	ht.RecordFailure("model-d", false)
	ht.RecordFailure("model-d", false) // degraded
	ht.RecordSuccess("model-e")        // healthy

	if ht.IsHealthy("model-d") {
		t.Fatal("model-d should be degraded")
	}
	if !ht.IsHealthy("model-e") {
		t.Fatal("model-e should be healthy")
	}
}

func TestHealthTracker_Snapshot(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 1,
		DegradeDuration:          5 * time.Minute,
	})
	ht.RecordFailure("model-f", false) // degraded
	ht.RecordSuccess("model-g")
	ht.RecordSuccess("model-h")

	snap := ht.Snapshot()
	if snap["model-f"] != HealthStatusDegraded {
		t.Fatalf("model-f status = %s, want degraded", snap["model-f"])
	}
	if snap["model-g"] != HealthStatusHealthy {
		t.Fatalf("model-g status = %s, want healthy", snap["model-g"])
	}
	if snap["model-h"] != HealthStatusHealthy {
		t.Fatalf("model-h status = %s, want healthy", snap["model-h"])
	}
}

func TestHealthTracker_DetailOfUnknown(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	if d := ht.DetailOf("unknown-model"); d != nil {
		t.Fatal("expected nil for unknown model")
	}
}

func TestHealthTracker_DefaultConfig(t *testing.T) {
	cfg := DefaultHealthConfig()
	if cfg.ConsecutiveFailThreshold != 3 {
		t.Fatalf("ConsecutiveFailThreshold = %d, want 3", cfg.ConsecutiveFailThreshold)
	}
	if cfg.DegradeDuration != 30*time.Second {
		t.Fatalf("DegradeDuration = %v, want 30s", cfg.DegradeDuration)
	}
	if cfg.SuccessRecoveryCount != 2 {
		t.Fatalf("SuccessRecoveryCount = %d, want 2", cfg.SuccessRecoveryCount)
	}
}

func TestHealthTracker_CustomConfig(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 5,
		DegradeDuration:          60 * time.Second,
		SuccessRecoveryCount:     3,
	})
	// 默认值应被覆盖
	if ht.config.ConsecutiveFailThreshold != 5 {
		t.Fatalf("ConsecutiveFailThreshold = %d, want 5", ht.config.ConsecutiveFailThreshold)
	}
	if ht.config.DegradeDuration != 60*time.Second {
		t.Fatalf("DegradeDuration = %v, want 60s", ht.config.DegradeDuration)
	}
	if ht.config.SuccessRecoveryCount != 3 {
		t.Fatalf("SuccessRecoveryCount = %d, want 3", ht.config.SuccessRecoveryCount)
	}
}

func TestHealthTracker_NilConfig(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	// 应使用默认值
	if ht.config.ConsecutiveFailThreshold != 3 {
		t.Fatal("nil config should use defaults")
	}
}

func TestHealthTracker_HealthStatusString(t *testing.T) {
	if HealthStatusHealthy.String() != "healthy" {
		t.Fatalf(`got %q, want "healthy"`, HealthStatusHealthy.String())
	}
	if HealthStatusDegraded.String() != "degraded" {
		t.Fatalf(`got %q, want "degraded"`, HealthStatusDegraded.String())
	}
}

func TestHealthTracker_DegradeDurationExpiry(t *testing.T) {
	// 使用极短的降级期验证自动恢复
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 1,
		DegradeDuration:          1 * time.Millisecond,
	})
	ht.RecordFailure("model-x", false)
	if ht.IsHealthy("model-x") {
		t.Fatal("model should be degraded immediately after failure")
	}
	// 等待降级期过后（轮询，不 sleep）
	deadline := time.After(time.Second)
	for {
		if ht.IsHealthy("model-x") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("model did not recover within timeout")
		default:
		}
		runtime.Gosched()
	}
}
