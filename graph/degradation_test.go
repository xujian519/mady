package graph

import (
	"testing"
)

func TestMarkDegraded(t *testing.T) {
	state := PregelState{}
	MarkDegraded(state, "prior_art", []string{}, DegradationRetrieverNil, "检索器未配置")

	// 降级值被设置。
	val, ok := state["prior_art"]
	if !ok {
		t.Fatal("prior_art key not set")
	}
	slice, ok := val.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", val)
	}
	if len(slice) != 0 {
		t.Errorf("expected empty slice, got %v", slice)
	}

	// 降级标记被设置。
	if !IsDegraded(state, "prior_art") {
		t.Fatal("expected degraded")
	}

	mark := GetDegradationMark(state, "prior_art")
	if mark == nil {
		t.Fatal("mark is nil")
	}
	if mark.Reason != DegradationRetrieverNil {
		t.Errorf("expected retriever_unavailable, got %s", mark.Reason)
	}
	if mark.Severity != "warning" {
		t.Errorf("expected warning, got %s", mark.Severity)
	}
}

func TestMarkDegradedCritical(t *testing.T) {
	state := PregelState{}
	MarkDegradedCritical(state, "db", nil, DegradationSearchFailed, "数据库不可用")

	mark := GetDegradationMark(state, "db")
	if mark == nil {
		t.Fatal("mark is nil")
	}
	if mark.Severity != "critical" {
		t.Errorf("expected critical, got %s", mark.Severity)
	}
}

func TestIsDegraded_NotDegraded(t *testing.T) {
	state := PregelState{"prior_art": []string{"real data"}}
	if IsDegraded(state, "prior_art") {
		t.Error("should not be degraded")
	}
}

func TestGetDegradationMark_Nil(t *testing.T) {
	state := PregelState{}
	if mark := GetDegradationMark(state, "nonexistent"); mark != nil {
		t.Errorf("expected nil, got %v", mark)
	}
}

func TestHasDegradation(t *testing.T) {
	state := PregelState{}
	if HasDegradation(state) {
		t.Error("empty state should not have degradation")
	}

	MarkDegraded(state, "key1", nil, DegradationNotImplemented, "未实现")
	if !HasDegradation(state) {
		t.Error("should have degradation after MarkDegraded")
	}
}

func TestDegradationSummary(t *testing.T) {
	state := PregelState{}
	MarkDegraded(state, "key1", nil, DegradationRetrieverNil, "检索器未配置")
	MarkDegraded(state, "key2", []string{}, DegradationSearchFailed, "检索失败")

	summary := DegradationSummary(state)
	if len(summary) != 2 {
		t.Fatalf("expected 2 marks, got %d", len(summary))
	}
}

func TestDegradationMark_Error(t *testing.T) {
	mark := DegradationMark{
		Reason:   DegradationNotImplemented,
		Message:  "功能未实现",
		Severity: "warning",
	}
	errStr := mark.Error()
	expected := "[not_implemented] 功能未实现"
	if errStr != expected {
		t.Errorf("expected %q, got %q", expected, errStr)
	}
}

func TestDegradationMark_MultipleKeys(t *testing.T) {
	// 验证多个 key 都有各自的降级标记，互不干扰。
	state := PregelState{
		"real_key": "real_value",
	}
	MarkDegraded(state, "degraded_a", "fallback_a", DegradationRetrieverNil, "a 降级")
	MarkDegraded(state, "degraded_b", "fallback_b", DegradationSearchFailed, "b 降级")

	// real_key 不应被标记为降级。
	if IsDegraded(state, "real_key") {
		t.Error("real_key should not be degraded")
	}

	// 各自独立。
	if !IsDegraded(state, "degraded_a") {
		t.Error("degraded_a should be degraded")
	}
	if !IsDegraded(state, "degraded_b") {
		t.Error("degraded_b should be degraded")
	}

	markA := GetDegradationMark(state, "degraded_a")
	if markA.Reason != DegradationRetrieverNil {
		t.Errorf("wrong reason for a: %s", markA.Reason)
	}
	markB := GetDegradationMark(state, "degraded_b")
	if markB.Reason != DegradationSearchFailed {
		t.Errorf("wrong reason for b: %s", markB.Reason)
	}
}
