package claimchart

import (
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestBuildClaimChart_Basic(t *testing.T) {
	input := ChartInput{
		Mode: "invalidity",
		ClaimText: `1. 一种智能终端，其特征在于，包括：
处理器、存储器、以及用于采集用户手势的传感器；
所述处理器被配置为根据所述手势执行对应操作。`,
		Targets: []ChartTargetInput{
			{ID: "D1", Kind: "prior-art", Title: "对比文件1"},
		},
		CaseID: "test-case",
	}

	chart, err := BuildClaimChart(input)
	if err != nil {
		t.Fatalf("BuildClaimChart failed: %v", err)
	}
	if chart.Mode != ModeInvalidity {
		t.Errorf("mode = %q, want invalidity", chart.Mode)
	}
	if chart.CaseID != "test-case" {
		t.Errorf("caseId = %q", chart.CaseID)
	}
	if len(chart.Elements) == 0 {
		t.Fatal("expected parsed elements")
	}
	if len(chart.ClaimNos) == 0 || chart.ClaimNos[0] != 1 {
		t.Errorf("claimNos = %v", chart.ClaimNos)
	}
	if chart.DraftNotice == "" {
		t.Error("expected draft notice")
	}
}

func TestParseClaimElements(t *testing.T) {
	text := `1. 一种通信装置，其特征在于，包括：天线、射频模块，以及基带处理器。
2. 根据权利要求1所述的通信装置，其特征在于，所述天线为阵列天线。`
	elements := parseClaimElements(text)
	if len(elements) == 0 {
		t.Fatal("expected elements")
	}
	if elements[0].ClaimNo != 1 {
		t.Errorf("first element claimNo = %d", elements[0].ClaimNo)
	}
	if elements[0].Kind != ElementPreamble {
		t.Errorf("first element kind = %q, want preamble", elements[0].Kind)
	}

	var claim2Count int
	for _, e := range elements {
		if e.ClaimNo == 2 {
			claim2Count++
		}
	}
	if claim2Count == 0 {
		t.Error("expected elements for claim 2")
	}
}

func TestHandleClaimChart_InvalidInput(t *testing.T) {
	args := json.RawMessage(`{"mode":"invalidity","claim_text":"","targets":[]}`)
	res, err := handleClaimChart(nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hr, ok := res.(agentcore.HandoffResult); ok && hr.Success {
		t.Error("expected failure for empty claim text")
	}
}
