package evaluate

import (
	"context"
	"fmt"
	"testing"
)

// ============================================================================
// Helpers: mock ApprovalRecord 实现
// ============================================================================

// mockApprovalRecord 是为测试准备的 ApprovalRecord 接口实现。
type mockApprovalRecord struct {
	decision string
}

func (r mockApprovalRecord) Decision() string { return r.decision }

// ============================================================================
// Tests: AdoptionRateMetric.Compute
// ============================================================================

func TestAdoptionRateMetric_Name(t *testing.T) {
	m := &AdoptionRateMetric{}
	if got := m.Name(); got != "adoption_rate" {
		t.Errorf("Name() = %q, want %q", got, "adoption_rate")
	}
}

func TestAdoptionRateMetric_AllAdopted(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 5, Modified: 0, Rejected: 0}
	score := m.Compute("", "")
	if score != 1.0 {
		t.Errorf("all adopted: Compute = %v, want 1.0", score)
	}
	if m.FullyAdopted() != 1.0 {
		t.Errorf("FullyAdopted = %v, want 1.0", m.FullyAdopted())
	}
	if m.Accepted() != 1.0 {
		t.Errorf("Accepted = %v, want 1.0", m.Accepted())
	}
	if m.Total() != 5 {
		t.Errorf("Total = %d, want 5", m.Total())
	}
}

func TestAdoptionRateMetric_PartialAdoption(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 6, Modified: 3, Rejected: 1}
	score := m.Compute("", "")
	// (6+3)/10 = 0.9
	want := 0.9
	if score != want {
		t.Errorf("partial adoption: Compute = %v, want %v", score, want)
	}
	if m.FullyAdopted() != 0.6 {
		t.Errorf("FullyAdopted = %v, want 0.6", m.FullyAdopted())
	}
	if m.Accepted() != 0.9 {
		t.Errorf("Accepted = %v, want 0.9", m.Accepted())
	}
	if m.RejectedRate() != 0.1 {
		t.Errorf("RejectedRate = %v, want 0.1", m.RejectedRate())
	}
	if m.Total() != 10 {
		t.Errorf("Total = %d, want 10", m.Total())
	}
}

func TestAdoptionRateMetric_AllRejected(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 0, Modified: 0, Rejected: 8}
	score := m.Compute("", "")
	if score != 0.0 {
		t.Errorf("all rejected: Compute = %v, want 0.0", score)
	}
	if m.FullyAdopted() != 0.0 {
		t.Errorf("FullyAdopted = %v, want 0.0", m.FullyAdopted())
	}
	if m.Accepted() != 0.0 {
		t.Errorf("Accepted = %v, want 0.0", m.Accepted())
	}
	if m.RejectedRate() != 1.0 {
		t.Errorf("RejectedRate = %v, want 1.0", m.RejectedRate())
	}
	if m.Total() != 8 {
		t.Errorf("Total = %d, want 8", m.Total())
	}
}

func TestAdoptionRateMetric_NoData(t *testing.T) {
	m := &AdoptionRateMetric{}
	// 无数据时应返回 1.0（尚无负面记录，视为满分）
	score := m.Compute("", "")
	if score != 1.0 {
		t.Errorf("no data: Compute = %v, want 1.0", score)
	}
	// Total/FullyAdopted/Accepted/RejectedRate 应返回 0
	if m.Total() != 0 {
		t.Errorf("Total = %d, want 0", m.Total())
	}
	if m.FullyAdopted() != 0.0 {
		t.Errorf("FullyAdopted = %v, want 0.0", m.FullyAdopted())
	}
	if m.Accepted() != 0.0 {
		t.Errorf("Accepted = %v, want 0.0", m.Accepted())
	}
	if m.RejectedRate() != 0.0 {
		t.Errorf("RejectedRate = %v, want 0.0", m.RejectedRate())
	}
}

// ============================================================================
// Tests: Record 方法
// ============================================================================

func TestAdoptionRateMetric_Record(t *testing.T) {
	m := &AdoptionRateMetric{}

	m.Record("adopted")
	m.Record("modified")
	m.Record("rejected")
	m.Record("adopted")

	if m.Adopted != 2 {
		t.Errorf("Adopted = %d, want 2", m.Adopted)
	}
	if m.Modified != 1 {
		t.Errorf("Modified = %d, want 1", m.Modified)
	}
	if m.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1", m.Rejected)
	}
	if m.Total() != 4 {
		t.Errorf("Total = %d, want 4", m.Total())
	}

	score := m.Compute("", "")
	want := 3.0 / 4.0 // (2+1)/4 = 0.75
	if score != want {
		t.Errorf("Compute after Record = %v, want %v", score, want)
	}
}

func TestAdoptionRateMetric_RecordUnknown(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 1, Modified: 1, Rejected: 1}
	// 未知决策应静默忽略
	m.Record("unknown")
	m.Record("")
	if m.Total() != 3 {
		t.Errorf("Total after unknown records = %d, want 3", m.Total())
	}
}

// ============================================================================
// Tests: FromApprovalRecords 批量导入
// ============================================================================

func TestFromApprovalRecords(t *testing.T) {
	records := []ApprovalRecord{
		mockApprovalRecord{decision: "adopted"},
		mockApprovalRecord{decision: "adopted"},
		mockApprovalRecord{decision: "modified"},
		mockApprovalRecord{decision: "rejected"},
		mockApprovalRecord{decision: "adopted"},
	}

	m := FromApprovalRecords(records)
	if m.Adopted != 3 {
		t.Errorf("Adopted = %d, want 3", m.Adopted)
	}
	if m.Modified != 1 {
		t.Errorf("Modified = %d, want 1", m.Modified)
	}
	if m.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1", m.Rejected)
	}
	if m.Total() != 5 {
		t.Errorf("Total = %d, want 5", m.Total())
	}

	score := m.Compute("", "")
	want := 4.0 / 5.0 // (3+1)/5 = 0.8
	if score != want {
		t.Errorf("Compute = %v, want %v", score, want)
	}
}

func TestFromApprovalRecords_Empty(t *testing.T) {
	m := FromApprovalRecords(nil)
	if m.Adopted != 0 || m.Modified != 0 || m.Rejected != 0 {
		t.Error("empty input should produce zero counters")
	}
	if m.Total() != 0 {
		t.Errorf("Total = %d, want 0", m.Total())
	}
}

// ============================================================================
// Tests: AdoptionRateMetric 作为 Metric 在 Evaluator 中运行
// ============================================================================

func TestAdoptionRateMetric_InEvaluator(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 8, Modified: 1, Rejected: 1}
	e := NewEvaluator(m)

	// Evaluator.Evaluate 会调用 Compute，忽略 prediction/reference
	result := e.Evaluate("anything", "anything", nil)
	if !result.Passed {
		t.Error("8/10 adoption should pass with default 0.7 threshold")
	}
	if result.Scores["adoption_rate"] != 0.9 {
		t.Errorf("adoption_rate score = %v, want 0.9", result.Scores["adoption_rate"])
	}
}

func TestAdoptionRateMetric_InEvaluatorLowAdoption(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 1, Modified: 0, Rejected: 9}
	e := NewEvaluator(m).WithThreshold(0.5)

	result := e.Evaluate("", "", nil)
	if result.Passed {
		t.Error("1/10 adoption should not pass with threshold 0.5")
	}
	if result.Scores["adoption_rate"] != 0.1 {
		t.Errorf("adoption_rate score = %v, want 0.1", result.Scores["adoption_rate"])
	}
}

func TestAdoptionRateMetric_InEvaluatorBatch(t *testing.T) {
	// AdoptionRateMetric 在 EvaluateBatch 中应对每条用例返回同一聚合值
	m := &AdoptionRateMetric{Adopted: 5, Modified: 3, Rejected: 2}
	e := NewEvaluator(m).WithThreshold(0.5)

	cases := []TestCase{
		{ID: "c1", Input: "x", Expected: "y"},
		{ID: "c2", Input: "x", Expected: "y"},
	}
	report, err := e.EvaluateBatch(context.Background(), cases, func(_ context.Context, input string) (string, error) {
		return "result for " + input, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCases != 2 {
		t.Errorf("TotalCases = %d, want 2", report.TotalCases)
	}
	if report.AggregateScores["adoption_rate"] != 0.8 {
		t.Errorf("aggregate adoption_rate = %v, want 0.8", report.AggregateScores["adoption_rate"])
	}
}

// ============================================================================
// Tests: ApprovalRecordFunc 适配器
// ============================================================================

func TestApprovalRecordFunc(t *testing.T) {
	// ApprovalRecordFunc 适配已有结构体（如 domains.ApprovalRecord）
	type externalRecord struct {
		decision string
	}
	rec := externalRecord{decision: "modified"}

	adapter := ApprovalRecordFunc(func() string {
		return rec.decision
	})

	if adapter.Decision() != "modified" {
		t.Errorf("Decision() = %q, want %q", adapter.Decision(), "modified")
	}

	// 可在 FromApprovalRecords 中使用
	records := []ApprovalRecord{adapter}
	m := FromApprovalRecords(records)
	if m.Modified != 1 {
		t.Errorf("Modified = %d, want 1", m.Modified)
	}
}

// ============================================================================
// Tests: 累加模式（复用同一 metric 实例）
// ============================================================================

func TestAdoptionRateMetric_Accumulate(t *testing.T) {
	m := &AdoptionRateMetric{}

	// 批次 1: 全通过
	m.Record("adopted")
	m.Record("adopted")
	m.Record("adopted")
	if m.Compute("", "") != 1.0 {
		t.Error("batch 1 should be 1.0")
	}

	// 批次 2: 追加拒绝
	m.Record("rejected")
	if m.Compute("", "") != 0.75 {
		t.Errorf("after rejection: Compute = %v, want 0.75", m.Compute("", ""))
	}

	// 批次 3: 再追加
	m.Record("modified")
	if m.Compute("", "") != 0.8 {
		t.Errorf("after modified: Compute = %v, want 0.8", m.Compute("", ""))
	}

	if m.Total() != 5 {
		t.Errorf("Total = %d, want 5", m.Total())
	}
}

// ============================================================================
// Tests: 边界情况
// ============================================================================

func TestAdoptionRateMetric_ZeroAdoptedWithData(t *testing.T) {
	// 有数据但全 rejection
	m := &AdoptionRateMetric{Rejected: 3}
	if m.Compute("", "") != 0.0 {
		t.Error("all rejected should be 0")
	}
	if m.FullyAdopted() != 0.0 {
		t.Error("FullyAdopted should be 0")
	}
	if m.Accepted() != 0.0 {
		t.Error("Accepted should be 0")
	}
	if m.RejectedRate() != 1.0 {
		t.Error("RejectedRate should be 1.0")
	}
}

func TestAdoptionRateMetric_PredictionRefIgnored(t *testing.T) {
	m := &AdoptionRateMetric{Adopted: 3, Modified: 1, Rejected: 1}
	// prediction/reference 参数被忽略，相同内部状态返回相同结果
	s1 := m.Compute("anything", "anything")
	s2 := m.Compute("", "")
	s3 := m.Compute("foo", "bar")
	if s1 != s2 || s2 != s3 {
		t.Error("Compute should be invariant of prediction/reference parameters")
	}
}

// ============================================================================
// Example
// ============================================================================

func ExampleAdoptionRateMetric() {
	m := &AdoptionRateMetric{}
	m.Record("adopted")
	m.Record("modified")
	m.Record("rejected")
	fmt.Printf("采纳率: %.0f%%\n", m.Compute("", "")*100)
	// Output: 采纳率: 67%
}
