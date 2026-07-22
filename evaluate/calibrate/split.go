package calibrate

import (
	"context"

	"github.com/xujian519/mady/evaluate"
)

// SplitConfig 定义时间分割评估的配置。
type SplitConfig struct {
	TrainEra string   `json:"train_era"` // 用于训练的 Era 名称
	TestEras []string `json:"test_eras"` // 用于测试的 Era 名称列表
}

// SplitReport 是时间分割评估的结果。
type SplitReport struct {
	Train *evaluate.BatchReport `json:"train,omitempty"`
	Test  []EraReport           `json:"test,omitempty"`
	// Gap 衡量训练集和测试集之间的性能差距。
	// gap = train_pass_rate - avg_test_pass_rate，正值表示过拟合风险。
	Gap float64 `json:"gap"`
}

// EraReport 是单个 Era 的评估报告。
type EraReport struct {
	Era         string                `json:"era"`
	Report      *evaluate.BatchReport `json:"report"`
	Calibration *CalibrationReport    `json:"calibration,omitempty"`
}

// EvaluateSplit 按时间分割配置分别评估训练集和测试集。
// 它从 cases 中筛选出匹配 TrainEra 的作为训练集，匹配 TestEras 的作为测试集。
func EvaluateSplit(ctx context.Context, cases []evaluate.TestCase, run evaluate.RunFunc,
	eval *evaluate.Evaluator, cfg SplitConfig) (*SplitReport, error) {

	report := &SplitReport{}

	// 分割案例
	var trainCases []evaluate.TestCase
	testMap := make(map[string][]evaluate.TestCase)

	for _, tc := range cases {
		if tc.Era == cfg.TrainEra {
			trainCases = append(trainCases, tc)
		}
		for _, testEra := range cfg.TestEras {
			if tc.Era == testEra {
				testMap[testEra] = append(testMap[testEra], tc)
			}
		}
	}

	// 评估训练集
	if len(trainCases) > 0 {
		trainReport, err := eval.EvaluateBatch(ctx, trainCases, run)
		if err != nil {
			return nil, err
		}
		report.Train = trainReport
	}

	// 评估每个测试 Era
	var testPassRates []float64
	for _, era := range cfg.TestEras {
		eraCases := testMap[era]
		if len(eraCases) == 0 {
			continue
		}
		eraReport, err := eval.EvaluateBatch(ctx, eraCases, run)
		if err != nil {
			return nil, err
		}
		report.Test = append(report.Test, EraReport{
			Era:    era,
			Report: eraReport,
		})
		testPassRates = append(testPassRates, eraReport.PassRate)
	}

	// 计算 Gap：训练集和测试集之间的性能差距
	if report.Train != nil && len(testPassRates) > 0 {
		var avgTest float64
		for _, pr := range testPassRates {
			avgTest += pr
		}
		avgTest /= float64(len(testPassRates))
		report.Gap = report.Train.PassRate - avgTest
	}

	return report, nil
}

// FilterByEra 按 era 筛选 test case 列表。
func FilterByEra(cases []evaluate.TestCase, era string) []evaluate.TestCase {
	var filtered []evaluate.TestCase
	for _, tc := range cases {
		if tc.Era == era {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

// FilterByDifficulty 按难度筛选 test case 列表。
func FilterByDifficulty(cases []evaluate.TestCase, difficulty string) []evaluate.TestCase {
	var filtered []evaluate.TestCase
	for _, tc := range cases {
		if tc.Difficulty == difficulty {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}
