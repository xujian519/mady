package calibrate

import (
	"math"
	"testing"
)

const eps = 1e-10

func assertFloat64(t *testing.T, name string, got, expected float64) {
	t.Helper()
	if math.Abs(got-expected) > eps {
		t.Errorf("%s: expected %v, got %v", name, expected, got)
	}
}

func TestComputeECE_PerfectCalibration(t *testing.T) {
	// 20 个数据点，两桶。
	// Bucket 0 [0, 0.5): 10 个点，置信度 ~0.2，全部不正确
	// Bucket 1 [0.5, 1]: 10 个点，置信度 ~0.8，全部正确
	var points []CalibrationPoint
	for i := 0; i < 10; i++ {
		points = append(points, CalibrationPoint{
			PredictedConfidence: 0.2,
			ActualCorrect:       false,
		})
	}
	for i := 0; i < 10; i++ {
		points = append(points, CalibrationPoint{
			PredictedConfidence: 0.8,
			ActualCorrect:       true,
		})
	}

	report := ComputeECE(points, 2)
	// Bucket 0: avg_conf=0.2, avg_acc=0, diff=0.2, weight=0.5 → contr=0.1
	// Bucket 1: avg_conf=0.8, avg_acc=1.0, diff=0.2, weight=0.5 → contr=0.1
	// ECE = 0.2
	assertFloat64(t, "ECE", report.ECE, 0.2)
	if report.TotalCases != 20 {
		t.Errorf("expected 20 cases, got %d", report.TotalCases)
	}
	if report.CorrectCases != 10 {
		t.Errorf("expected 10 correct, got %d", report.CorrectCases)
	}
}

func TestComputeECE_SystematicOverconfidence(t *testing.T) {
	// 所有点置信度 0.9，只有一半正确
	points := make([]CalibrationPoint, 10)
	for i := 0; i < 10; i++ {
		points[i] = CalibrationPoint{
			PredictedConfidence: 0.9,
			ActualCorrect:       i < 5,
		}
	}

	report := ComputeECE(points, 5)
	// 所有点在同一桶 [0.8, 1.0)
	// avg_conf=0.9, avg_acc=0.5, diff=0.4, weight=1
	assertFloat64(t, "ECE", report.ECE, 0.4)
	if report.Overconfidence <= 0 {
		t.Error("expected overconfidence > 0")
	}
}

func TestComputeECE_Empty(t *testing.T) {
	report := ComputeECE(nil, 10)
	assertFloat64(t, "ECE", report.ECE, 0)
}

func TestComputeECE_InvalidBuckets(t *testing.T) {
	points := []CalibrationPoint{
		{PredictedConfidence: 0.8, ActualCorrect: true, CaseID: "c1"},
	}
	// numBuckets <= 0 默认 10
	report := ComputeECE(points, 0)
	// 点在桶 [0.8, 0.9)，avg_conf=0.8, avg_acc=1.0, diff=0.2, weight=1
	assertFloat64(t, "ECE", report.ECE, 0.2)
}

func TestComputeECE_SingleBucket(t *testing.T) {
	points := []CalibrationPoint{
		{PredictedConfidence: 0.9, ActualCorrect: true, CaseID: "c1"},
		{PredictedConfidence: 0.1, ActualCorrect: false, CaseID: "c2"},
	}
	report := ComputeECE(points, 1)
	// 单桶 [0, 1]: avg_conf=0.5, avg_acc=0.5, diff=0, ECE=0
	assertFloat64(t, "ECE", report.ECE, 0)
}

func TestSortByConfidence(t *testing.T) {
	points := []CalibrationPoint{
		{PredictedConfidence: 0.9, CaseID: "c3"},
		{PredictedConfidence: 0.3, CaseID: "c1"},
		{PredictedConfidence: 0.5, CaseID: "c2"},
	}
	SortByConfidence(points)
	if points[0].CaseID != "c1" || points[2].CaseID != "c3" {
		t.Error("sort by confidence failed")
	}
}

func TestReliabilityData(t *testing.T) {
	points := []CalibrationPoint{
		{PredictedConfidence: 0.05, ActualCorrect: true, CaseID: "c1"},
		{PredictedConfidence: 0.15, ActualCorrect: true, CaseID: "c2"},
		{PredictedConfidence: 0.95, ActualCorrect: false, CaseID: "c3"},
		{PredictedConfidence: 0.85, ActualCorrect: true, CaseID: "c4"},
	}

	report := ComputeECE(points, 2) // [0, 0.5), [0.5, 1]
	if len(report.ReliabilityData) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(report.ReliabilityData))
	}

	// Bucket 0: c1(0.05,true), c2(0.15,true) → avg_conf=0.1, avg_acc=1.0
	b0 := report.ReliabilityData[0]
	if b0.Count != 2 {
		t.Errorf("expected bucket 0 count=2, got %d", b0.Count)
	}
	assertFloat64(t, "bucket 0 avg_conf", b0.AvgConf, 0.1)
	assertFloat64(t, "bucket 0 avg_acc", b0.AvgAcc, 1.0)

	// Bucket 1: c3(0.95,false), c4(0.85,true) → avg_conf=0.9, avg_acc=0.5
	b1 := report.ReliabilityData[1]
	if b1.Count != 2 {
		t.Errorf("expected bucket 1 count=2, got %d", b1.Count)
	}
	assertFloat64(t, "bucket 1 avg_conf", b1.AvgConf, 0.9)
	assertFloat64(t, "bucket 1 avg_acc", b1.AvgAcc, 0.5)
}
