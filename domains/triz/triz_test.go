package triz

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const sampleSource = "本方案通过增大散热面积提高散热效率，但整机体积随之增大。实测散热效率提升12%，整机体积增加8%。"

func TestLookupCellKnownValues(t *testing.T) {
	ids, err := LookupCell(3, 1) // 改善「运动物体长度」/ 恶化「运动物体重量」
	if err != nil {
		t.Fatalf("LookupCell: %v", err)
	}
	want := []int{8, 15, 29, 34}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("cell(3,1) = %v, want %v", ids, want)
	}
}

func TestLookupCellDiagonalAndGap(t *testing.T) {
	ids, err := LookupCell(5, 5)
	if err != nil {
		t.Fatalf("diagonal: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("对角线格应无推荐原理，got %v", ids)
	}
	gap, err := LookupCell(2, 1) // 矩阵转录空缺格
	if err != nil {
		t.Fatalf("gap: %v", err)
	}
	if len(gap) != 0 {
		t.Errorf("空缺格应无推荐原理，got %v", gap)
	}
}

func TestLookupCellOutOfRange(t *testing.T) {
	for _, pair := range [][2]int{{0, 1}, {1, 40}, {-1, 5}} {
		if _, err := LookupCell(pair[0], pair[1]); err == nil {
			t.Errorf("LookupCell(%d,%d) 应报错", pair[0], pair[1])
		}
	}
}

func TestParametersAndPrinciples(t *testing.T) {
	params := Parameters()
	if len(params) != 39 || params[0].Name == "" || params[38].Number != 39 {
		t.Fatalf("parameters = %d 条，首条 %+v", len(params), params[0])
	}
	if ParamLabel(17) != "温度" {
		t.Errorf("ParamLabel(17) = %q", ParamLabel(17))
	}
	if ParamLabel(99) != "99" {
		t.Errorf("未知编号应回退为编号本身，got %q", ParamLabel(99))
	}
	principles := Principles()
	if len(principles) != 40 {
		t.Fatalf("principles = %d 条", len(principles))
	}
	principle, ok := PrincipleByNo(1)
	if !ok || principle.Name == "" || principle.Description == "" {
		t.Fatalf("PrincipleByNo(1) = %+v, ok=%v", principle, ok)
	}
}

func TestBuildAnalysisRecommended(t *testing.T) {
	analysis, err := BuildAnalysis(Extraction{
		Contradictions: []ContradictionInput{{
			Improving: 3, Worsening: 1,
			Statement: "提高散热效率的同时整机体积增大",
			Evidence:  "整机体积随之增大",
			SolutionDirections: []string{
				"把散热面积分散到多个独立面",
			},
		}},
	}, sampleSource)
	if err != nil {
		t.Fatalf("BuildAnalysis: %v", err)
	}
	if len(analysis.Contradictions) != 1 {
		t.Fatalf("contradictions = %d", len(analysis.Contradictions))
	}
	item := analysis.Contradictions[0]
	if item.ID != "C1" || item.MatrixStatus != MatrixRecommended {
		t.Errorf("id/status = %q/%q", item.ID, item.MatrixStatus)
	}
	if item.Improving.Name != "运动物体长度" || item.Worsening.Name != "运动物体重量" {
		t.Errorf("参数名取自行参数表？got %+v / %+v", item.Improving, item.Worsening)
	}
	if len(item.Principles) != 4 || item.Principles[0].Number != 8 {
		t.Errorf("principles = %+v", item.Principles)
	}
	if analysis.DroppedForEvidence != 0 || len(analysis.Unmapped) != 0 {
		t.Errorf("unexpected drops: %+v", analysis)
	}
	if !strings.Contains(RenderAnalysis(analysis), "推荐原理") {
		t.Error("报告缺少推荐原理段")
	}
}

func TestBuildAnalysisPhysicalAndGap(t *testing.T) {
	analysis, err := BuildAnalysis(Extraction{
		Contradictions: []ContradictionInput{
			{Improving: 5, Worsening: 5, Statement: "面积既大又小", Evidence: "增大散热面积"},
			{Improving: 2, Worsening: 1, Statement: "转录空缺", Evidence: "整机体积增加8%"},
		},
	}, sampleSource)
	if err != nil {
		t.Fatalf("BuildAnalysis: %v", err)
	}
	if analysis.Contradictions[0].MatrixStatus != MatrixPhysical {
		t.Errorf("status = %q, want physical", analysis.Contradictions[0].MatrixStatus)
	}
	if len(analysis.Contradictions[0].Principles) != 0 {
		t.Error("物理矛盾不应带推荐原理")
	}
	if analysis.Contradictions[1].MatrixStatus != MatrixGap {
		t.Errorf("status = %q, want gap", analysis.Contradictions[1].MatrixStatus)
	}
}

func TestBuildAnalysisDropsUngroundedEvidence(t *testing.T) {
	analysis, err := BuildAnalysis(Extraction{
		Contradictions: []ContradictionInput{
			{Improving: 3, Worsening: 1, Statement: "编造的矛盾", Evidence: "交底书里没有这句话"},
			{Improving: 40, Worsening: 1, Statement: "编号越界", Evidence: "增大散热面积"},
		},
	}, sampleSource)
	if err != nil {
		t.Fatalf("BuildAnalysis: %v", err)
	}
	if len(analysis.Contradictions) != 0 {
		t.Fatalf("contradictions = %d, want 0", len(analysis.Contradictions))
	}
	if analysis.DroppedForEvidence != 1 {
		t.Errorf("dropped = %d, want 1", analysis.DroppedForEvidence)
	}
	if len(analysis.Unmapped) != 2 {
		t.Fatalf("unmapped = %+v", analysis.Unmapped)
	}
	if !strings.Contains(analysis.Unmapped[0].Reason, "无法在交底书原文中定位") {
		t.Errorf("reason[0] = %q", analysis.Unmapped[0].Reason)
	}
	if !strings.Contains(analysis.Unmapped[1].Reason, "1-39") {
		t.Errorf("reason[1] = %q", analysis.Unmapped[1].Reason)
	}
}

func TestBuildAnalysisMergesGaps(t *testing.T) {
	analysis, err := BuildAnalysis(Extraction{
		ParameterGaps: []ParameterGapInput{
			{Parameter: 17, Missing: []GapKind{GapUnit}, Detail: "未给温度单位"},
			{Parameter: 17, Missing: []GapKind{GapTargetValue}},
			{Parameter: 17, Missing: []GapKind{"bogus"}},
			{Parameter: 0, Missing: []GapKind{GapUnit}},
		},
	}, "")
	if err != nil {
		t.Fatalf("BuildAnalysis: %v", err)
	}
	if len(analysis.ParameterGaps) != 1 {
		t.Fatalf("gaps = %+v", analysis.ParameterGaps)
	}
	gap := analysis.ParameterGaps[0]
	wantKinds := []GapKind{GapTargetValue, GapUnit}
	if !reflect.DeepEqual(gap.Missing, wantKinds) {
		t.Errorf("missing = %v, want %v（按全集顺序）", gap.Missing, wantKinds)
	}
	if gap.Detail != "未给温度单位" {
		t.Errorf("detail = %q", gap.Detail)
	}
	// 非法维度与非法编号各记一条未采纳。
	if len(analysis.Unmapped) != 2 || !strings.Contains(analysis.Unmapped[0].Reason, "非法") {
		t.Errorf("unmapped = %+v", analysis.Unmapped)
	}
}

func runTrizTool(t *testing.T, args string) (ToolResult, error) {
	t.Helper()
	tool := NewTool()
	raw, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return ToolResult{}, err
	}
	result, ok := raw.(ToolResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return result, nil
}

func TestToolNeedsExtraction(t *testing.T) {
	result, err := runTrizTool(t, `{"text":"本方案通过增大散热面积提高散热效率。","focus":"散热"}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.Status != "needs_extraction" {
		t.Fatalf("status = %q", result.Status)
	}
	if !strings.Contains(result.Prompt, "39 个经典工程参数") || !strings.Contains(result.Prompt, "关注方向：散热") {
		t.Error("抽取提示不完整")
	}
	if result.Schema == nil {
		t.Error("缺少 JSON Schema")
	}
}

func TestToolAnalyzesExtraction(t *testing.T) {
	result, err := runTrizTool(t, `{
		"text": "本方案通过增大散热面积提高散热效率，但整机体积随之增大。",
		"extraction": {
			"contradictions": [{
				"improving": 3, "worsening": 1,
				"statement": "提高散热效率的同时整机体积增大",
				"evidence": "整机体积随之增大"
			}],
			"parameter_gaps": [{"parameter": 17, "missing": ["unit"]}]
		}
	}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.Status != "analyzed" || result.Analysis == nil {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Analysis.Contradictions) != 1 {
		t.Fatalf("contradictions = %+v", result.Analysis.Contradictions)
	}
	if !strings.Contains(result.Report, "改善「运动物体长度（3）」") {
		t.Errorf("report = %q", result.Report)
	}
	if !strings.Contains(result.Report, "温度（17）：缺单位") {
		t.Errorf("report 缺少缺口段: %q", result.Report)
	}
}

func TestToolRejectsInvalidArgs(t *testing.T) {
	if _, err := runTrizTool(t, `{"text":""}`); err == nil {
		t.Error("空 text 应报错")
	}
	if _, err := runTrizTool(t, `{"text":"x","extraction":{"contradictions":[{"improving":"abc"}]}}`); err == nil {
		t.Error("类型错误的 extraction 应报错")
	}
}
