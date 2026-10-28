package adversary

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestScanClaimNumbers(t *testing.T) {
	cases := map[string][]int{
		"权利要求1-3、5所述":  {1, 2, 3, 5},
		"权利要求 1，2 和 4": {1, 2, 4},
		"如权利要求2所述":     {2},
		"无引用":          nil,
	}
	for text, want := range cases {
		if got := ScanClaimNumbers(text); !reflect.DeepEqual(got, want) {
			t.Errorf("ScanClaimNumbers(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestScanDisputeText(t *testing.T) {
	text := "不符合专利法第22条第3款的规定，也不符合第二十六条第四款；另参见 A33。"
	got := ScanDisputeText(text)
	want := []string{"A22.3", "A26.4", "A33"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("statutes = %v, want %v", got, want)
	}
	if statutes := ScanDisputeText("不符合专利法第四十二条"); len(statutes) != 0 {
		t.Errorf("未收录条号不应产出候选，got %v", statutes)
	}
}

func TestLocateDisputesIndependent(t *testing.T) {
	officeAction := &OfficeAction{
		RejectionTypes: []string{"novelty", "inventiveness"},
		AffectedClaims: []int{1, 2},
	}
	disputes := LocateDisputesIndependent(officeAction, "本申请涉及A33的修改问题")
	if len(disputes) != 3 {
		t.Fatalf("disputes = %+v", disputes)
	}
	if disputes[0].Statute != "A22.2" || disputes[1].Statute != "A22.3" || disputes[2].Statute != "A33" {
		t.Errorf("statutes = %s/%s/%s", disputes[0].Statute, disputes[1].Statute, disputes[2].Statute)
	}
	if len(disputes[0].Claims) != 2 || disputes[0].Confidence != 0.8 {
		t.Errorf("dispute0 = %+v", disputes[0])
	}
	if disputes[2].Confidence != 0.5 {
		t.Errorf("正文补充扫描置信度 = %v, want 0.5", disputes[2].Confidence)
	}

	unverified := LocateDisputesIndependent(nil, "权利要求1所述，涉及专利法第22条第3款。")
	if len(unverified) != 1 || unverified[0].Tag != TagUnverified || unverified[0].Confidence != 0.3 {
		t.Errorf("unverified = %+v", unverified)
	}
	if !reflect.DeepEqual(unverified[0].Claims, []int{1}) {
		t.Errorf("claims = %v", unverified[0].Claims)
	}
}

func TestAssessEvidenceIndependent(t *testing.T) {
	officeAction := &OfficeAction{
		RejectionTypes: []string{"novelty", "inventiveness"},
		AffectedClaims: []int{1, 2},
	}
	plan := []PlanEntry{{Ground: "novelty", Strategy: "argument", Claims: []int{1}}}
	disputes := AssessEvidenceIndependent(officeAction, plan)
	if len(disputes) != 1 {
		t.Fatalf("disputes = %+v", disputes)
	}
	if disputes[0].Statute != "A22.3" || disputes[0].Relation != RelationDisagreement || disputes[0].Confidence != 0.9 {
		t.Errorf("dispute = %+v", disputes[0])
	}

	misaligned := AssessEvidenceIndependent(officeAction, []PlanEntry{
		{Ground: "novelty", Strategy: "argument", Claims: []int{5}},
		{Ground: "inventiveness", Strategy: "argument", Claims: []int{2}},
	})
	if len(misaligned) != 1 || !strings.Contains(misaligned[0].Point, "无交集") {
		t.Errorf("misaligned = %+v", misaligned)
	}
	if AssessEvidenceIndependent(nil, plan) != nil {
		t.Error("无通知书时不应评估")
	}
}

func TestDetectDivergenceAndConfidence(t *testing.T) {
	disputes := []Dispute{
		{Statute: "A22.2", Confidence: 0.8, Relation: RelationMissedByBaseline},
		{Statute: "A22.3", Confidence: 0.9, Relation: RelationDisagreement},
		{Statute: "A26.4", Confidence: 0.5, Relation: RelationMissedByBaseline},
	}
	marked := DetectDivergence(disputes, []PlanEntry{{Ground: "novelty", Strategy: "argument"}})
	if marked[0].Relation != RelationAgreement {
		t.Errorf("A22.2 = %q, want agreement", marked[0].Relation)
	}
	if marked[1].Relation != RelationDisagreement {
		t.Errorf("分歧标记不应被覆盖，got %q", marked[1].Relation)
	}
	if marked[2].Relation != RelationMissedByBaseline {
		t.Errorf("A26.4 = %q, want missed_by_baseline", marked[2].Relation)
	}

	wantConfidence := (0.8*0.8 + 0.9*0.9 + 0.5*0.5) / (0.8 + 0.9 + 0.5)
	if confidence := ComputeOverallConfidence(marked); confidence < wantConfidence-1e-9 || confidence > wantConfidence+1e-9 {
		t.Errorf("confidence = %v, want %v", confidence, wantConfidence)
	}
	if ComputeOverallConfidence(nil) != 0 {
		t.Error("无争议点置信度应为 0")
	}
}

func TestBuildReportAndRender(t *testing.T) {
	disputes := []Dispute{
		{Statute: "A22.2", Claims: []int{1}, Point: "一致项", Relation: RelationAgreement, Confidence: 0.8, Tag: TagHeuristic},
		{Statute: "A22.3", Claims: []int{2}, Point: "分歧项", Relation: RelationDisagreement,
			DivergenceReason: "权项不对齐", Confidence: 0.9, Tag: TagHeuristic},
		{Statute: "A33", Point: "基线未覆盖", Relation: RelationMissedByBaseline, Confidence: 0.5, Tag: TagUnverified},
	}
	report := BuildReport(disputes, "摘要文本")
	if len(report.Agreements) != 1 || len(report.Disagreements) != 1 || len(report.MissedByBaseline) != 1 || len(report.Unverified) != 1 {
		t.Fatalf("report = %+v", report)
	}
	rendered := RenderReport(report)
	for _, want := range []string{"共 3 条争议点", "摘要：摘要文本", "分歧项", "A22.3", "原因：权项不对齐", "未验证项", "限制说明"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered 缺少 %q", want)
		}
	}
}

func TestParseModelOutput(t *testing.T) {
	raw := "```json\n{\"disputes\":[{" +
		"\"statute\":\"A22.3\",\"claims\":[1,2],\"point\":\"缺少技术启示\",\"weakness\":\"未引用对比文件2\",\"confidence\":0.85" +
		"}],\"overall_assessment\":\"论证偏弱\",\"divergence_reason\":\"证据不足\"}\n```"
	parsed, ok := ParseModelOutput(raw)
	if !ok {
		t.Fatal("应能解析被代码围栏包裹的 JSON")
	}
	if len(parsed.Disputes) != 1 {
		t.Fatalf("disputes = %+v", parsed.Disputes)
	}
	dispute := parsed.Disputes[0]
	if dispute.Statute != "A22.3" || dispute.Confidence != 0.85 || !strings.Contains(dispute.Point, "未引用对比文件2") {
		t.Errorf("dispute = %+v", dispute)
	}
	if dispute.DivergenceReason != "未引用对比文件2" {
		t.Errorf("reason = %q", dispute.DivergenceReason)
	}
	if parsed.OverallAssessment != "论证偏弱" || parsed.DivergenceReason != "证据不足" {
		t.Errorf("parsed = %+v", parsed)
	}

	if _, ok := ParseModelOutput("不是 JSON"); ok {
		t.Error("非法输入应返回 false")
	}
	// 缺 statute 的条目被丢弃，confidence 缺失取 0.5。
	partial, ok := ParseModelOutput(`{"disputes":[{"point":"x"},{"statute":"A33"}]}`)
	if !ok || len(partial.Disputes) != 1 || partial.Disputes[0].Confidence != 0.5 {
		t.Errorf("partial = %+v ok=%v", partial, ok)
	}
}

func TestMergeWithModelRequiresHigherConfidence(t *testing.T) {
	heuristic := []Dispute{{Statute: "A26.4", Point: "确定性结论", Confidence: 0.5, Tag: TagHeuristic}}
	equal := []Dispute{{Statute: "A26.4", Point: "模型空壳", Confidence: 0.5, Tag: TagHeuristic}}
	merged := MergeWithModel(heuristic, equal)
	if merged[0].Point != "确定性结论" {
		t.Errorf("置信度相等时不应覆盖确定性结果，got %q", merged[0].Point)
	}
	higher := []Dispute{{Statute: "A26.4", Point: "模型高置信", Confidence: 0.9, Tag: TagHeuristic}}
	if got := MergeWithModel(heuristic, higher); got[0].Point != "模型高置信" {
		t.Errorf("模型置信度更高时应覆盖，got %q", got[0].Point)
	}
}

func TestCheckToolEmptyOfficeActionUsesTextScan(t *testing.T) {
	result, err := runCheckTool(t, `{
		"office_action": {},
		"claims_text": "权利要求1所述方案涉及专利法第22条第3款。"
	}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if len(result.Report.Disputes) == 0 {
		t.Fatalf("空通知书应回退正文扫描，got %+v", result.Report)
	}
	if result.Report.Disputes[0].Tag != TagUnverified || result.Report.Disputes[0].Statute != "A22.3" {
		t.Errorf("dispute = %+v", result.Report.Disputes[0])
	}
}

func TestIsolationChecks(t *testing.T) {
	if result := CheckInputIsolation([]string{"office_action", "claims"}); !result.Clean {
		t.Errorf("result = %+v", result)
	}
	leaked := CheckInputIsolation([]string{"office_action", "claim_chart", "draft"})
	if leaked.Clean || len(leaked.LeakedKeys) != 2 {
		t.Errorf("leaked = %+v", leaked)
	}
	if result := ScanOutputForLeaks("本报告引用 claim_chart 的结论"); result.Clean {
		t.Error("应检出禁止术语")
	}
}

func runCheckTool(t *testing.T, args string) (CheckResult, error) {
	t.Helper()
	raw, err := NewCheckTool().Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return CheckResult{}, err
	}
	result, ok := raw.(CheckResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return result, nil
}

const checkArgsBase = `{
	"office_action": {"rejection_types": ["novelty", "inventiveness"], "affected_claims": [1, 2]},
	"claims_text": "",
	"response_plan": [{"ground": "novelty", "strategy": "argument", "claims": [1]}]
}`

func TestCheckToolReturnsPrompt(t *testing.T) {
	result, err := runCheckTool(t, checkArgsBase)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.ModelUsed {
		t.Error("未传 model_output 时不应标记模型已用")
	}
	if !strings.Contains(result.Prompt, "【答复计划】") || !strings.Contains(result.Prompt, "A22.3") {
		t.Errorf("prompt = %q", result.Prompt)
	}
	if len(result.Report.Agreements) != 1 || len(result.Report.Disagreements) != 1 {
		t.Errorf("report = %+v", result.Report)
	}
	if !strings.Contains(result.Report.Summary, "模型不可用") {
		t.Errorf("summary = %q", result.Report.Summary)
	}
}

func TestCheckToolMergesModelOutput(t *testing.T) {
	args := strings.TrimSuffix(checkArgsBase, "}") + `,
		"model_output": "{\"disputes\":[{\"statute\":\"A22.3\",\"claims\":[1,2],\"point\":\"缺少技术启示论证\",\"confidence\":0.95}],\"overall_assessment\":\"答复计划对创造性论证不足\"}"
	}`
	result, err := runCheckTool(t, args)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if !result.ModelUsed {
		t.Error("应标记模型已用")
	}
	if result.Report.Summary != "答复计划对创造性论证不足" {
		t.Errorf("summary = %q", result.Report.Summary)
	}
	if len(result.Report.Disagreements) != 1 || result.Report.Disagreements[0].Confidence != 0.95 {
		t.Errorf("disagreements = %+v", result.Report.Disagreements)
	}
}

func TestCheckToolRejectsLeakingModelOutput(t *testing.T) {
	args := strings.TrimSuffix(checkArgsBase, "}") + `,
		"model_output": "{\"disputes\":[{\"statute\":\"A22.3\",\"point\":\"参见 claim_chart\",\"confidence\":0.9}],\"overall_assessment\":\"x\"}"
	}`
	result, err := runCheckTool(t, args)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.ModelUsed {
		t.Error("输出泄漏时应回退，不标记模型已用")
	}
	if !strings.Contains(result.Note, "禁止术语") {
		t.Errorf("note = %q", result.Note)
	}
}

func TestCheckToolRejectsForbiddenStateKeys(t *testing.T) {
	args := strings.TrimSuffix(checkArgsBase, "}") + `, "state_keys": ["office_action", "claim_chart"]}`
	if _, err := runCheckTool(t, args); err == nil {
		t.Error("输入含禁止键应报错")
	}
}

func TestReviewFinalDraft(t *testing.T) {
	officeAction := &OfficeAction{RejectionTypes: []string{"inventiveness"}, AffectedClaims: []int{1}}
	weak := ReviewFinalDraft(ReviewInput{
		FinalDraft:   "本申请具备新颖性，应予授权。",
		Claims:       "1. 一种装置，包括壳体。",
		OfficeAction: officeAction,
	})
	if weak.ArgumentStrength != StrengthInsufficient {
		t.Errorf("strength = %q, want insufficient（遗漏创造性）", weak.ArgumentStrength)
	}
	if len(weak.MissedRejections) != 1 || weak.MissedRejections[0] != "inventiveness" {
		t.Errorf("missed = %v", weak.MissedRejections)
	}

	strong := ReviewFinalDraft(ReviewInput{
		FinalDraft:         "关于创造性：对比文件 CN123456A 未公开区别特征，权利要求1具备创造性。",
		ReferenceDocuments: "对比文件 CN123456A 公开了一种装置。",
		Claims:             "1. 一种装置，包括壳体。",
		OfficeAction:       officeAction,
	})
	if strong.ArgumentStrength != StrengthStrong {
		t.Errorf("strength = %q, want strong（证据：%v）", strong.ArgumentStrength, strong.EvidenceGaps)
	}
	if len(strong.MissedRejections) != 0 || len(strong.EvidenceGaps) != 0 {
		t.Errorf("strong = %+v", strong)
	}
	if strong.OverallConfidence != 0.8 {
		t.Errorf("confidence = %v, want 0.8", strong.OverallConfidence)
	}
}

func TestReviewTool(t *testing.T) {
	raw, err := NewReviewTool().Func(context.Background(), json.RawMessage(`{
		"final_draft": "关于创造性：对比文件 CN123456A 未公开区别特征，权利要求1具备创造性。",
		"reference_documents": "CN123456A",
		"claims": "1. 一种装置。",
		"office_action": {"rejection_types": ["inventiveness"], "affected_claims": [1]}
	}`))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	result, ok := raw.(ReviewResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	if result.Report.ArgumentStrength != StrengthStrong {
		t.Errorf("strength = %q", result.Report.ArgumentStrength)
	}
	if !strings.Contains(result.Rendered, "换位审查报告") {
		t.Errorf("rendered = %q", result.Rendered)
	}
}
