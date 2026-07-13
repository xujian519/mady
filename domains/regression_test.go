package domains

import (
	"testing"
	"time"
)

func TestApprovalToTestCase(t *testing.T) {
	record := ApprovalRecord{
		ID:             "appr_001",
		SessionID:      "sess-1",
		CaseID:         "case-1",
		Timestamp:      time.Now(),
		TriggerKeyword: "专利结论",
		OriginalOutput: "该发明具备新颖性。",
		Decision:       DecisionModified,
		ModifiedOutput: "该发明具备新颖性。根据《专利法》第二十二条第二款，新颖性要求不属于现有技术。",
	}

	tc := ApprovalToTestCase(record, "patent")
	if tc.ID != "regression_appr_001" {
		t.Errorf("ID = %s, want regression_appr_001", tc.ID)
	}
	if tc.Domain != "patent" {
		t.Errorf("Domain = %s, want patent", tc.Domain)
	}
	if tc.Input != record.OriginalOutput {
		t.Error("Input should be OriginalOutput")
	}
	if tc.Expected != record.ModifiedOutput {
		t.Error("Expected should be ModifiedOutput")
	}
}

func TestApprovalToRegressionCandidates(t *testing.T) {
	records := []ApprovalRecord{
		{
			ID:             "r1",
			OriginalOutput: "AI 原始输出1",
			Decision:       DecisionModified,
			ModifiedOutput: "人工修改后的输出1",
		},
		{
			ID:             "r2",
			OriginalOutput: "AI 原始输出2",
			Decision:       DecisionAdopted,
			ModifiedOutput: "",
		},
		{
			ID:             "r3",
			OriginalOutput: "AI 原始输出3",
			Decision:       DecisionModified,
			ModifiedOutput: "", // empty → should be skipped
		},
		{
			ID:             "r4",
			OriginalOutput: "AI 原始输出4",
			Decision:       DecisionRejected,
			ModifiedOutput: "",
		},
		{
			ID:             "r5",
			OriginalOutput: "AI 原始输出5",
			Decision:       DecisionModified,
			ModifiedOutput: "人工修改后的输出5",
		},
	}

	cases := ApprovalToRegressionCandidates(records, "patent")
	if len(cases) != 2 {
		t.Fatalf("expected 2 regression candidates, got %d", len(cases))
	}
	if cases[0].Input != "AI 原始输出1" {
		t.Errorf("case 0 Input = %s", cases[0].Input)
	}
	if cases[1].Expected != "人工修改后的输出5" {
		t.Errorf("case 1 Expected = %s", cases[1].Expected)
	}
}

func TestApprovalToRegressionCandidates_Empty(t *testing.T) {
	cases := ApprovalToRegressionCandidates(nil, "patent")
	if len(cases) != 0 {
		t.Errorf("expected 0 cases for nil input, got %d", len(cases))
	}
}
