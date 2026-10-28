package patent

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestBuildOAResponsePlan_PerClaimRows(t *testing.T) {
	notice := &ParsedOfficeAction{
		RejectionType:  string(OaClarity),
		RejectionTypes: []OaRejectionType{OaClarity},
		AffectedClaims: []int{1, 2, 3},
	}
	plan := buildOAResponsePlan(notice, []int{1})

	if len(plan.AmendmentRows) != 3 {
		t.Fatalf("rows = %d, want 3（逐权项）", len(plan.AmendmentRows))
	}
	if plan.AmendmentRows[0].Kind != OAClaimIndependent || plan.AmendmentRows[0].Action != AmendmentClarify {
		t.Errorf("row0 = %+v, want 独立/澄清限定", plan.AmendmentRows[0])
	}
	for _, row := range plan.AmendmentRows[1:] {
		if row.Kind != OAClaimDependent || row.Action != AmendmentAdjustReference {
			t.Errorf("row = %+v, want 从属/从属引用调整", row)
		}
	}
	if !reflect.DeepEqual(plan.ClaimsToAmend, []int{1, 2, 3}) {
		t.Errorf("claimsToAmend = %v", plan.ClaimsToAmend)
	}
	if len(plan.Gaps) != 0 {
		t.Errorf("gaps = %+v, want none", plan.Gaps)
	}
	if plan.StrategySummary != "不清楚→修改" {
		t.Errorf("summary = %q", plan.StrategySummary)
	}
	if len(plan.Sections) != 1 || len(plan.Sections[0].Sections) == 0 {
		t.Errorf("sections = %+v", plan.Sections)
	}
}

func TestBuildOAResponsePlan_AmendmentWithoutClaims(t *testing.T) {
	notice := &ParsedOfficeAction{
		RejectionTypes: []OaRejectionType{OaSupport},
	}
	plan := buildOAResponsePlan(notice, nil)
	if len(plan.AmendmentRows) != 0 {
		t.Fatalf("rows = %+v, want none", plan.AmendmentRows)
	}
	if len(plan.Gaps) != 1 || plan.Gaps[0].Kind != PlanGapAmendmentWithoutClaims {
		t.Fatalf("gaps = %+v", plan.Gaps)
	}
	if !strings.Contains(plan.Gaps[0].Detail, "须人工确认") {
		t.Errorf("detail = %q", plan.Gaps[0].Detail)
	}
}

func TestBuildOAResponsePlan_ArgumentOnly(t *testing.T) {
	notice := &ParsedOfficeAction{
		RejectionTypes: []OaRejectionType{OaNovelty, OaInventiveness},
		AffectedClaims: []int{1, 2},
	}
	plan := buildOAResponsePlan(notice, []int{1})
	if len(plan.AmendmentRows) != 0 {
		t.Errorf("争辩型理由不应产生修改行，got %+v", plan.AmendmentRows)
	}
	if len(plan.Gaps) != 0 {
		t.Errorf("无修改行时不应报独立权项缺口，got %+v", plan.Gaps)
	}
	if plan.StrategySummary != "新颖性→争辩、创造性→争辩" {
		t.Errorf("summary = %q", plan.StrategySummary)
	}
}

func TestBuildOAResponsePlan_IndependentAbsentGap(t *testing.T) {
	notice := &ParsedOfficeAction{
		RejectionTypes: []OaRejectionType{OaClarity},
		AffectedClaims: []int{2, 3},
	}
	plan := buildOAResponsePlan(notice, nil) // 未给独立权项编号，按从属处理
	if len(plan.AmendmentRows) != 2 {
		t.Fatalf("rows = %+v", plan.AmendmentRows)
	}
	found := false
	for _, gap := range plan.Gaps {
		if gap.Kind == PlanGapIndependentAbsent {
			found = true
		}
	}
	if !found {
		t.Errorf("gaps = %+v, want independent-claim-absent", plan.Gaps)
	}
}

func TestBuildOAResponsePlan_NoGround(t *testing.T) {
	notice := &ParsedOfficeAction{RejectionType: "unknown"}
	plan := buildOAResponsePlan(notice, nil)
	if len(plan.Gaps) != 1 || plan.Gaps[0].Kind != PlanGapNoGround {
		t.Fatalf("gaps = %+v, want no-ground-detected", plan.Gaps)
	}
	if !strings.Contains(plan.Gaps[0].Detail, "unknown") {
		t.Errorf("detail = %q", plan.Gaps[0].Detail)
	}
	if plan.StrategySummary != "综合答复" {
		t.Errorf("summary = %q", plan.StrategySummary)
	}
}

func TestBuildOAResponsePlan_UnknownGroundFallsBack(t *testing.T) {
	notice := &ParsedOfficeAction{
		RejectionTypes: []OaRejectionType{"other"},
		AffectedClaims: []int{1},
	}
	plan := buildOAResponsePlan(notice, []int{1})
	if len(plan.Sections) != 1 || plan.Sections[0].Strategy != StrategyCombined {
		t.Fatalf("sections = %+v, want combined", plan.Sections)
	}
	if plan.Sections[0].Sections[0] != unknownGroundSection {
		t.Errorf("sections = %v", plan.Sections[0].Sections)
	}
	if len(plan.AmendmentRows) != 1 || plan.AmendmentRows[0].Action != AmendmentNone {
		t.Errorf("rows = %+v", plan.AmendmentRows)
	}
	if plan.AmendmentRows[0].Basis != unknownGroundBasis {
		t.Errorf("basis = %q", plan.AmendmentRows[0].Basis)
	}
}

func TestDetectIndependentClaims(t *testing.T) {
	text := `1. 一种智能门锁，其特征在于包括锁体和指纹模块。
2. 根据权利要求1所述的智能门锁，其特征在于所述锁体为合金锁体。
3. 一种智能门锁的控制方法，其特征在于包括指纹比对步骤。
4. 如权利要求3所述的方法，其特征在于还包括报警步骤。`
	got := detectIndependentClaims(text)
	if !reflect.DeepEqual(got, []int{1, 3}) {
		t.Errorf("independent = %v, want [1 3]", got)
	}
}

func TestDetectIndependentClaims_NoHeadings(t *testing.T) {
	if got := detectIndependentClaims("本发明公开了一种门锁。"); got != nil {
		t.Errorf("no headings should return nil, got %v", got)
	}
	if got := detectIndependentClaims(""); got != nil {
		t.Errorf("empty text should return nil, got %v", got)
	}
}

func TestAnalyzeClaimsNode_UsesClaimTextForIndependentKind(t *testing.T) {
	parsed := &ParsedOfficeAction{
		RejectionType:  string(OaClarity),
		RejectionTypes: []OaRejectionType{OaClarity},
		AffectedClaims: []int{1, 2},
	}
	state := graph.PregelState{
		OAStateParsed:         parsed,
		OAStateRejectionTypes: parsed.RejectionTypes,
		OAStateAffectedClaims: parsed.AffectedClaims,
		OAStateClaimText: "1. 一种门锁，其特征在于包括锁体。\n" +
			"2. 根据权利要求1所述的门锁，其特征在于锁体为合金。",
	}
	out, err := analyzeClaimsNode(context.Background(), state)
	if err != nil {
		t.Fatalf("analyzeClaimsNode: %v", err)
	}
	amendments := out.GetString(OAStateClaimAmendments)
	if !strings.Contains(amendments,
		"| 不清楚 | 澄清限定 | [原内容] | [建议修改] | 专利法第26条第4款（清楚） | 权利要求1（独立） |") {
		t.Errorf("expected independent claim row with clarify action:\n%s", amendments)
	}
	if !strings.Contains(amendments, "权利要求2（从属）") {
		t.Errorf("expected dependent claim row:\n%s", amendments)
	}
	if strings.Contains(amendments, "没有独立权利要求被点出") {
		t.Errorf("given claim text, independent-claim-absent gap should be gone:\n%s", amendments)
	}
}
