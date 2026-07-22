// Package calibrate 实现校准评估（Calibration）功能。
//
// 校准评估量化 LLM 判断的置信度与实际准确率之间的关系，
// 用于识别系统性 over-confidence 或 under-confidence。
package calibrate

import (
	"math"
	"sort"
)

// CalibrationPoint 是单次评估的校准数据点。
type CalibrationPoint struct {
	PredictedConfidence float64 `json:"predicted_confidence"` // 预测置信度（0-1）
	ActualCorrect       bool    `json:"actual_correct"`       // 与 ground truth 是否一致
	CaseID              string  `json:"case_id"`
}

// BucketStat 是单桶的统计数据，用于生成可靠性图。
type BucketStat struct {
	BinMin  float64 `json:"bin_min"`        // 桶下界
	BinMax  float64 `json:"bin_max"`        // 桶上界
	Count   int     `json:"count"`          // 桶内样本数
	AvgConf float64 `json:"avg_confidence"` // 桶内平均置信度
	AvgAcc  float64 `json:"avg_accuracy"`   // 桶内实际准确率
}

// CalibrationReport 是校准评估的完整报告。
type CalibrationReport struct {
	Points          []CalibrationPoint `json:"points"`
	ECE             float64            `json:"ece"`              // Expected Calibration Error
	ReliabilityData []BucketStat       `json:"reliability_data"` // 每桶统计（供绘图）
	Overconfidence  float64            `json:"overconfidence"`   // 系统性高估比例
	Underconfidence float64            `json:"underconfidence"`  // 系统性低估比例
	TotalCases      int                `json:"total_cases"`
	CorrectCases    int                `json:"correct_cases"`
	Accuracy        float64            `json:"accuracy"` // 整体准确率
}

// ComputeECE 计算 Expected Calibration Error（期望校准误差）。
// numBuckets 指定分桶数（默认 10，使用 0.1 的宽度）。
// ECE = Σ(bucket_count / total) * |avg_confidence - avg_accuracy|
func ComputeECE(points []CalibrationPoint, numBuckets int) CalibrationReport {
	if numBuckets <= 0 {
		numBuckets = 10
	}
	if len(points) == 0 {
		return CalibrationReport{ECE: 0}
	}

	bucketWidth := 1.0 / float64(numBuckets)
	buckets := make([]BucketStat, numBuckets)
	for i := range buckets {
		buckets[i].BinMin = float64(i) * bucketWidth
		buckets[i].BinMax = float64(i+1) * bucketWidth
	}

	correctCount := 0
	for _, p := range points {
		if p.ActualCorrect {
			correctCount++
		}
		bin := int(math.Min(float64(p.PredictedConfidence)/bucketWidth, float64(numBuckets-1)))
		if bin < 0 {
			bin = 0
		}
		buckets[bin].Count++
		buckets[bin].AvgConf += p.PredictedConfidence
		if p.ActualCorrect {
			buckets[bin].AvgAcc += 1.0
		}
	}

	// 计算每桶平均
	total := len(points)
	var ece float64
	var overCount, underCount int
	var filteredBuckets []BucketStat

	for i := range buckets {
		if buckets[i].Count == 0 {
			continue
		}
		buckets[i].AvgConf /= float64(buckets[i].Count)
		buckets[i].AvgAcc /= float64(buckets[i].Count)

		diff := buckets[i].AvgConf - buckets[i].AvgAcc
		ece += (float64(buckets[i].Count) / float64(total)) * math.Abs(diff)

		if diff > 0 {
			overCount += buckets[i].Count
		} else if diff < 0 {
			underCount += buckets[i].Count
		}

		filteredBuckets = append(filteredBuckets, buckets[i])
	}

	return CalibrationReport{
		Points:          points,
		ECE:             ece,
		ReliabilityData: filteredBuckets,
		Overconfidence:  float64(overCount) / float64(total),
		Underconfidence: float64(underCount) / float64(total),
		TotalCases:      total,
		CorrectCases:    correctCount,
		Accuracy:        float64(correctCount) / float64(total),
	}
}

// ComputeECEFromPairs is a convenience wrapper that builds CalibrationPoints
// from a flat slice of (confidence, isCorrect) pairs.
func ComputeECEFromPairs(pairs []struct {
	Confidence float64
	Correct    bool
	ID         string
}, numBuckets int) CalibrationReport {
	points := make([]CalibrationPoint, len(pairs))
	for i, p := range pairs {
		points[i] = CalibrationPoint{
			PredictedConfidence: p.Confidence,
			ActualCorrect:       p.Correct,
			CaseID:              p.ID,
		}
	}
	return ComputeECE(points, numBuckets)
}

// SortByConfidence 按置信度升序排列校准数据点。
func SortByConfidence(points []CalibrationPoint) {
	sort.Slice(points, func(i, j int) bool {
		return points[i].PredictedConfidence < points[j].PredictedConfidence
	})
}
