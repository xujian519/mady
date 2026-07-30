package domains

import (
	"github.com/xujian519/mady/evaluate"
)

// ApprovalToTestCase converts a single ApprovalRecord (where the human
// modified the output) into a TestCase suitable for the second-layer Golden
// Benchmark. The AI's original output becomes the test input, and the
// human's modified version becomes the expected reference answer.
//
// Only DecisionModified records are converted — adopted outputs are already
// correct (no regression signal), and rejected outputs lack a usable
// expected answer. The caller should still review each candidate before
// adding it to the benchmark dataset.
func ApprovalToTestCase(record ApprovalRecord, domain string) evaluate.TestCase {
	return evaluate.TestCase{
		ID:                "regression_" + record.ID,
		Domain:            domain,
		Input:             record.OriginalOutput,
		Expected:          record.ModifiedOutput,
		RequiredCitations: []string{},
	}
}

// ApprovalToRegressionCandidates filters a batch of ApprovalRecords for
// DecisionModified entries and converts them to TestCases. This is the
// semi-automated pipeline from ApprovalGate 留痕 (B1) to Golden Benchmark
// 第二层 (C1): the human's edits encode implicit quality standards that
// the benchmark should capture.
//
// Records with empty ModifiedOutput are skipped — they indicate the human
// rejected without providing an alternative, which is not useful for
// regression testing.
func ApprovalToRegressionCandidates(records []ApprovalRecord, domain string) []evaluate.TestCase {
	var cases []evaluate.TestCase
	for _, r := range records {
		if r.Decision != DecisionModified {
			continue
		}
		if r.ModifiedOutput == "" {
			continue
		}
		cases = append(cases, ApprovalToTestCase(r, domain))
	}
	return cases
}
