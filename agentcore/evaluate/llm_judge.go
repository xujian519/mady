package evaluate

import (
	"math/rand"
	"sort"
)

// LLMJudgeCaller is the minimal interface needed to perform LLM-based
// judgment comparison. Implementations wrap an LLM provider (e.g.
// agentcore.Provider) and handle prompt construction, context/timeout, and
// response parsing internally.
//
// This indirection keeps the evaluate package decoupled from agentcore.
type LLMJudgeCaller interface {
	// JudgeConsistency asks the LLM whether prediction and reference are
	// semantically consistent. Returns true if they agree, false if they
	// disagree. An error indicates the LLM call failed (the caller should
	// treat errors as "disagree" for safety).
	JudgeConsistency(prediction, reference string) (bool, error)
}

// NewLLMJudgeFunc wraps an LLMJudgeCaller into a JudgeFunc suitable for
// use with JudgeConsistency. On LLM errors, it falls back to "disagree"
// (false) — this is conservative: a failed judge should not silently pass.
func NewLLMJudgeFunc(caller LLMJudgeCaller) JudgeFunc {
	return func(prediction, reference string) bool {
		agree, err := caller.JudgeConsistency(prediction, reference)
		if err != nil {
			return false
		}
		return agree
	}
}

// ---------------------------------------------------------------------------
// Calibration — 抽样人工校准
// ---------------------------------------------------------------------------

// CalibrationSample represents a single case selected for human review.
// Low-confidence LLM judge results (score near the threshold) are prioritized
// so that human feedback can calibrate the judge over time.
type CalibrationSample struct {
	CaseID     string
	Prediction string
	Reference  string
	Score      float64 // the metric score that triggered selection
	Reason     string  // why this case was selected
}

// CollectCalibrationSamples selects cases from a BatchReport for human
// review. It prioritizes:
//  1. Cases that failed (score < threshold) — to catch false negatives
//  2. Cases with borderline scores (within ±0.1 of threshold) — to calibrate
//  3. A random sample of passing cases (at `rate`) — to catch false positives
//
// `predictions` maps CaseID to the actual prediction text.
// `cases` provides the reference text and citations for each case.
// `rate` controls the random sampling of passing cases (0.0-1.0).
// `threshold` is the pass threshold (typically 0.7).
func CollectCalibrationSamples(
	report *BatchReport,
	predictions map[string]string,
	cases []TestCase,
	rate float64,
	threshold float64,
) []CalibrationSample {
	if report == nil {
		return nil
	}

	caseMap := make(map[string]TestCase, len(cases))
	for _, c := range cases {
		caseMap[c.ID] = c
	}

	var failed, borderline, passing []CalibrationSample

	for _, r := range report.Results {
		tc, ok := caseMap[r.CaseID]
		if !ok {
			continue
		}
		pred := predictions[r.CaseID]

		sample := CalibrationSample{
			CaseID:     r.CaseID,
			Prediction: pred,
			Reference:  tc.Expected,
			Score:      r.Average,
		}

		switch {
		case !r.Passed:
			sample.Reason = "failed — potential false negative"
			failed = append(failed, sample)
		case r.Average >= threshold-0.1 && r.Average <= threshold+0.1:
			sample.Reason = "borderline — near threshold"
			borderline = append(borderline, sample)
		default:
			sample.Reason = "random sample — false positive check"
			passing = append(passing, sample)
		}
	}

	// All failed + all borderline + random sample of passing.
	sampled := make([]CalibrationSample, 0, len(failed)+len(borderline)+len(passing))
	sampled = append(sampled, failed...)
	sampled = append(sampled, borderline...)

	if len(passing) > 0 && rate > 0 {
		n := int(float64(len(passing)) * rate)
		if n < 1 {
			n = 1
		}
		rand.Shuffle(len(passing), func(i, j int) {
			passing[i], passing[j] = passing[j], passing[i]
		})
		if n > len(passing) {
			n = len(passing)
		}
		sampled = append(sampled, passing[:n]...)
	}

	sort.Slice(sampled, func(i, j int) bool {
		return sampled[i].Score < sampled[j].Score
	})

	return sampled
}
