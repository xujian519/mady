package agentcore

import (
	"context"
	"testing"
	"time"
)

func TestFallbackRouter_SelectsPrimaryWhenHealthy(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini", "deepseek-v4-flash"},
		},
	}, nil, nil)

	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}

	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if req.Model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want gpt-4o-mini", req.Model)
	}
}

func TestFallbackRouter_FallbackToNextWhenDegraded(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 1,
		DegradeDuration:          5 * time.Minute,
	})
	ht.RecordFailure("gpt-4o-mini", false) // degrade primary

	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini", "deepseek-v4-flash"},
		},
	}, nil, ht)

	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}

	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if req.Model != "deepseek-v4-flash" {
		t.Fatalf("model = %q, want deepseek-v4-flash (fallback)", req.Model)
	}
}

func TestFallbackRouter_AllDegradedFallsBackToPrimary(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 1,
		DegradeDuration:          5 * time.Minute,
	})
	ht.RecordFailure("gpt-4o-mini", false)
	ht.RecordFailure("deepseek-v4-flash", false)

	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini", "deepseek-v4-flash"},
		},
	}, nil, ht)

	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}

	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	// 全部降级时回退到主模型
	if req.Model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want gpt-4o-mini (all degraded fallback)", req.Model)
	}
}

func TestFallbackRouter_NoCandidatesForComplexity(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini"},
		},
	}, nil, nil)

	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{
		Input: "分析这个专利的新颖性", // ComplexityHigh
	}

	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	// 未配置 High 的回退 → 不应修改 model
	if req.Model != "default" {
		t.Fatalf("model = %q, want default (unchanged)", req.Model)
	}
}

func TestFallbackRouter_NextFallback(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini", "deepseek-v4-flash", "gemini-2.0-flash"},
		},
	}, nil, nil)

	// 模拟 BeforeModelCall 设置复杂度
	fr.lastComplexity = ComplexityLow

	// 第一次回退
	next := fr.NextFallback("gpt-4o-mini")
	if next != "deepseek-v4-flash" {
		t.Fatalf("first fallback = %q, want deepseek-v4-flash", next)
	}
	// 第二次回退
	next = fr.NextFallback("deepseek-v4-flash")
	if next != "gemini-2.0-flash" {
		t.Fatalf("second fallback = %q, want gemini-2.0-flash", next)
	}
	// 无更多回退
	next = fr.NextFallback("gemini-2.0-flash")
	if next != "" {
		t.Fatalf("third fallback = %q, want empty", next)
	}
}

func TestFallbackRouter_NextFallbackNoCandidates(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{}, nil, nil)
	fr.lastComplexity = ComplexityLow

	next := fr.NextFallback("model-a")
	if next != "" {
		t.Fatal("expected empty when no candidates configured")
	}
}

func TestFallbackRouter_NextFallbackSingleCandidate(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"only-model"},
		},
	}, nil, nil)
	fr.lastComplexity = ComplexityLow

	next := fr.NextFallback("only-model")
	if next != "" {
		t.Fatal("expected empty when only one candidate")
	}
}

func TestFallbackRouter_Reset(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"model-a", "model-b"},
		},
	}, nil, nil)
	fr.lastComplexity = ComplexityLow
	fr.NextFallback("model-a") // 前进一次 → idx = 1

	// 验证 Reset 前索引非零
	if idx := fr.fallbackIndex[ComplexityLow]; idx != 1 {
		t.Fatalf("fallback index before reset = %d, want 1", idx)
	}

	fr.Reset()

	// Reset 后索引归零
	if idx := fr.fallbackIndex[ComplexityLow]; idx != 0 {
		t.Fatalf("fallback index after reset = %d, want 0", idx)
	}

	// Reset 后从头开始回退
	next := fr.NextFallback("model-a")
	if next != "model-b" {
		t.Fatalf("after reset, next fallback = %q, want model-b", next)
	}
}

func TestFallbackRouter_StickySession(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini", "deepseek-v4-flash"},
		},
		StickySession: true,
	}, nil, ht)

	// 第一次调用：选择 gpt-4o-mini
	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}
	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if req.Model != "gpt-4o-mini" {
		t.Fatalf("first call model = %q, want gpt-4o-mini", req.Model)
	}
	// 记录成功
	fr.HealthTracker.RecordSuccess("gpt-4o-mini")

	// 第二次调用（同一复杂度）：仍应选择 gpt-4o-mini（sticky）
	req2 := &ProviderRequest{Model: "default"}
	mcc2 := &ModelCallContext{Request: req2}
	if err := fr.BeforeModelCall(context.Background(), arc, mcc2); err != nil {
		t.Fatal(err)
	}
	if req2.Model != "gpt-4o-mini" {
		t.Fatalf("sticky: second call model = %q, want gpt-4o-mini", req2.Model)
	}
}

func TestFallbackRouter_StickySessionSwitchesOnDegrade(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 1,
		DegradeDuration:          5 * time.Minute,
	})
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"gpt-4o-mini", "deepseek-v4-flash"},
		},
		StickySession: true,
	}, nil, ht)

	// 第一次调用：选择 gpt-4o-mini
	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}
	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if req.Model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want gpt-4o-mini", req.Model)
	}

	// 降级 gpt-4o-mini
	ht.RecordFailure("gpt-4o-mini", false)

	// 第二次调用：应切换到 deepseek-v4-flash
	req2 := &ProviderRequest{Model: "default"}
	mcc2 := &ModelCallContext{Request: req2}
	if err := fr.BeforeModelCall(context.Background(), arc, mcc2); err != nil {
		t.Fatal(err)
	}
	if req2.Model != "deepseek-v4-flash" {
		t.Fatalf("after degrade: model = %q, want deepseek-v4-flash", req2.Model)
	}
}

func TestFallbackRouter_AfterModelCallRecordsSuccess(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"model-s"},
		},
	}, nil, ht)

	fr.lastModel = "model-s"

	// 模拟成功响应
	mcc := &ModelCallContext{
		Request:  &ProviderRequest{Model: "model-s"},
		Response: &ProviderResponse{Content: "ok"},
	}
	fr.AfterModelCall(context.Background(), nil, mcc)

	d := ht.DetailOf("model-s")
	if d == nil {
		t.Fatal("expected health detail")
	}
	if d.TotalCalls != 1 {
		t.Fatalf("total calls = %d, want 1", d.TotalCalls)
	}
}

func TestFallbackRouter_AfterModelCallRecordsFailure(t *testing.T) {
	ht := NewProviderHealthTracker(&HealthConfig{
		ConsecutiveFailThreshold: 1,
		DegradeDuration:          5 * time.Minute,
	})
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"model-f"},
		},
	}, nil, ht)

	fr.lastModel = "model-f"

	// 模拟失败响应
	mcc := &ModelCallContext{
		Request: nil, // runAfterModelCall 传入 nil
		Err:     NewRetryableError("test", "test error", nil),
	}
	fr.AfterModelCall(context.Background(), nil, mcc)

	if ht.IsHealthy("model-f") {
		t.Fatal("model should be degraded after failure record")
	}
}

func TestFallbackRouter_AfterModelCallUsesLastModelWhenRequestNil(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"model-l"},
		},
	}, nil, ht)

	// BeforeModelCall 设置 lastModel 为 "model-l"
	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}
	_ = fr.BeforeModelCall(context.Background(), arc, mcc)

	// AfterModelCall 时 Request 为 nil（runAfterModelCall 场景）
	// 应使用 lastModel（从 BeforeModelCall 缓存）记录成功
	mcc2 := &ModelCallContext{
		Request:  nil,
		Response: &ProviderResponse{Content: "ok"},
	}
	fr.AfterModelCall(context.Background(), nil, mcc2)

	d := ht.DetailOf("model-l")
	if d == nil {
		t.Fatal("expected health detail for model-l (via lastModel)")
	}
	if d.TotalCalls != 1 {
		t.Fatalf("total calls = %d, want 1", d.TotalCalls)
	}
}

func TestFallbackRouter_NilCandidatesConfigSkipsIntervention(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{}, nil, nil)

	req := &ProviderRequest{Model: "keep-me"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}

	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if req.Model != "keep-me" {
		t.Fatalf("model = %q, want keep-me (unchanged)", req.Model)
	}
}

func TestFallbackRouter_NilClassifierDefaults(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"cheap-model", "expensive-model"},
		},
	}, nil, nil)

	if fr.Classifier == nil {
		t.Fatal("classifier should default to DefaultClassifier")
	}

	req := &ProviderRequest{Model: "default"}
	mcc := &ModelCallContext{Request: req}
	arc := &AgentRunContext{Input: "hello"}

	if err := fr.BeforeModelCall(context.Background(), arc, mcc); err != nil {
		t.Fatal(err)
	}
	if req.Model != "cheap-model" {
		t.Fatalf("model = %q, want cheap-model", req.Model)
	}
}

func TestFallbackRouter_NilHealthTrackerDefaults(t *testing.T) {
	fr := NewFallbackRouter(FallbackConfig{}, nil, nil)
	if fr.HealthTracker == nil {
		t.Fatal("health tracker should default to non-nil")
	}
}

// TestFallbackIntegration_AgentConfig verifies the FallbackRouter wires into
// Agent config correctly and doesn't break validation.
func TestFallbackIntegration_AgentConfig(t *testing.T) {
	ht := NewProviderHealthTracker(nil)
	fr := NewFallbackRouter(FallbackConfig{
		Candidates: map[Complexity][]string{
			ComplexityLow: {"stub", "fallback-model"},
		},
	}, nil, ht)

	cfg := StubConfig(&stubProvider{})
	cfg.FallbackRouter = fr

	if err := cfg.Validate(); err != nil {
		t.Fatalf("config with FallbackRouter should validate: %v", err)
	}
}
