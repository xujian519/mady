package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// fullDocumentaryInput 返回一份声明齐全的书证输入。
func fullDocumentaryInput() StatutoryEvidenceInput {
	return StatutoryEvidenceInput{
		EvidenceID:         "evi-doc",
		Kind:               KindDocumentary,
		Acquisition:        EvTypeGeneral,
		FactToProve:        "双方于 2025 年 3 月签订买卖合同",
		Content:            "合同约定分三期付款",
		Source:             "原件由当事人保管",
		HasOriginal:        new(true),
		SignatureVerified:  new(true),
		FormationTimeClear: new(true),
		CollectionLegal:    new(true),
		FormLegal:          new(true),
	}
}

func findingsByRule(a StatutoryAssessment) map[string]StatutoryFinding {
	m := make(map[string]StatutoryFinding, len(a.Findings))
	for _, f := range a.Findings {
		m[f.RuleID] = f
	}
	return m
}

// 八类法定证据各取一个评分用例。
func TestAssessStatutoryEvidence_EightKinds(t *testing.T) {
	cases := []struct {
		name  string
		input StatutoryEvidenceInput
	}{
		{"书证", fullDocumentaryInput()},
		{"物证", StatutoryEvidenceInput{
			Kind: KindPhysical, HasOriginal: new(true),
			StorageIntegrityVerified: new(true),
			CollectionLegal:          new(true), FormLegal: new(true),
		}},
		{"视听资料", StatutoryEvidenceInput{
			Kind: KindAudioVisual, ContentHash: "sha256:abc", HashVerified: new(true),
			Timestamp: "2025-03-01T10:00:00Z", StorageIntegrityVerified: new(true),
			CollectionLegal: new(true), FormLegal: new(true),
		}},
		{"电子数据", StatutoryEvidenceInput{
			Kind: KindElectronic, Source: "邮件服务器", ContentHash: "sha256:def",
			HashVerified: new(true), Timestamp: "2025-03-01T10:00:00Z",
			StorageIntegrityVerified: new(true),
			CollectionLegal:          new(true), FormLegal: new(true),
		}},
		{"证人证言", StatutoryEvidenceInput{
			Kind: KindWitnessTestimony, Source: "证人张某", Content: "见证了交付过程",
			WitnessHasConflict: new(false), Corroborated: new(true),
			CollectionLegal: new(true), FormLegal: new(true),
		}},
		{"当事人陈述", StatutoryEvidenceInput{
			Kind: KindPartyStatement, Content: "原告主张已付款",
			Corroborated:    new(true),
			CollectionLegal: new(true), FormLegal: new(true),
		}},
		{"鉴定意见", StatutoryEvidenceInput{
			Kind: KindExpertOpinion, Content: "鉴定材料为合同原件",
			ExpertQualified: new(true), ExpertProcedureLegal: new(true),
			ExpertConclusionClear: new(true),
		}},
		{"勘验笔录", StatutoryEvidenceInput{
			Kind: KindInspectionRecord, InspectionHasWitness: new(true),
			InspectionRecordComplete: new(true),
			CollectionLegal:          new(true), FormLegal: new(true),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AssessStatutoryEvidence(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Overall <= 0 || got.Overall > 1 {
				t.Errorf("overall %f outside (0,1]", got.Overall)
			}
			for _, dim := range []DimensionJudgment{got.Relevance, got.Legality, got.Authenticity} {
				if dim.Score < 0 || dim.Score > 1 {
					t.Errorf("%s score %f outside [0,1]", dim.Dimension, dim.Score)
				}
				if dim.Level == "" || dim.Reasoning == "" {
					t.Errorf("%s missing level or reasoning", dim.Dimension)
				}
			}
			if len(got.Findings) == 0 {
				t.Error("expected at least one finding")
			}
			if got.KindLabel == "" || got.Reasoning == "" {
				t.Error("expected kind label and reasoning")
			}
		})
	}
}

// 两维组合至少 5 例：同一份证据在两个轴上各取各的规则。
func TestAssessStatutoryEvidence_TwoAxisCombinations(t *testing.T) {
	cases := []struct {
		name     string
		input    StatutoryEvidenceInput
		wantKind string
		wantAcq  string
	}{
		{
			name: "书证×互联网公开",
			input: StatutoryEvidenceInput{
				Kind: KindDocumentary, Acquisition: EvTypeInternetPublication,
				HasOriginal: new(true), SignatureVerified: new(true),
				PublicDateClear: new(true), PubliclyAccessible: new(true),
			},
			wantKind: "LEGAL-EVI-001", wantAcq: "LEGAL-EVI-ACQ-001",
		},
		{
			name: "电子数据×互联网公开",
			input: StatutoryEvidenceInput{
				Kind: KindElectronic, Acquisition: EvTypeInternetPublication,
				ContentHash: "sha256:abc", HashVerified: new(true),
				Timestamp: "2025-03-01", StorageIntegrityVerified: new(true),
				PublicDateClear: new(true), PubliclyAccessible: new(false),
			},
			wantKind: "LEGAL-EVI-004", wantAcq: "LEGAL-EVI-ACQ-002",
		},
		{
			name: "证人证言×域外取得",
			input: StatutoryEvidenceInput{
				Kind: KindWitnessTestimony, Acquisition: EvTypeOverseas,
				WitnessHasConflict: new(false), Corroborated: new(true),
				Notarized: new(true),
			},
			wantKind: "LEGAL-EVI-007", wantAcq: "LEGAL-EVI-ACQ-004",
		},
		{
			name: "书证×外文形成",
			input: StatutoryEvidenceInput{
				Kind: KindDocumentary, Acquisition: EvTypeForeignLang,
				HasOriginal: new(true), SignatureVerified: new(true),
				Translated: new(true),
			},
			wantKind: "LEGAL-EVI-002", wantAcq: "LEGAL-EVI-ACQ-005",
		},
		{
			name: "物证×使用公开",
			input: StatutoryEvidenceInput{
				Kind: KindPhysical, Acquisition: EvTypePublicUse,
				HasOriginal: new(true), PublicUseFourElements: new(false),
			},
			wantKind: "LEGAL-EVI-013", wantAcq: "LEGAL-EVI-ACQ-003",
		},
		{
			name: "勘验笔录×公证",
			input: StatutoryEvidenceInput{
				Kind: KindInspectionRecord, Acquisition: EvTypeNotarial,
				InspectionHasWitness: new(true), InspectionRecordComplete: new(true),
				Notarized: new(true),
			},
			wantKind: "LEGAL-EVI-015", wantAcq: "LEGAL-EVI-ACQ-006",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AssessStatutoryEvidence(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			byRule := findingsByRule(got)
			kindFinding, ok := byRule[tc.wantKind]
			if !ok {
				t.Fatalf("missing kind-axis finding %s", tc.wantKind)
			}
			if kindFinding.Axis != AxisKind {
				t.Errorf("finding %s axis = %q, want %q", tc.wantKind, kindFinding.Axis, AxisKind)
			}
			acqFinding, ok := byRule[tc.wantAcq]
			if !ok {
				t.Fatalf("missing acquisition-axis finding %s", tc.wantAcq)
			}
			if acqFinding.Axis != AxisAcquisition {
				t.Errorf("finding %s axis = %q, want %q", tc.wantAcq, acqFinding.Axis, AxisAcquisition)
			}
		})
	}
}

// 两轴互不覆盖：换一个轴不改变另一个轴的检查结果与分数贡献。
func TestAssessStatutoryEvidence_AxesDoNotOverride(t *testing.T) {
	base := fullDocumentaryInput()
	internet := base
	internet.Acquisition = EvTypeInternetPublication
	internet.PublicDateClear = new(true)
	internet.PubliclyAccessible = new(true)
	publicUse := base
	publicUse.Acquisition = EvTypePublicUse
	publicUse.PublicUseFourElements = new(true)

	a1, err := AssessStatutoryEvidence(internet)
	if err != nil {
		t.Fatalf("internet: %v", err)
	}
	a2, err := AssessStatutoryEvidence(publicUse)
	if err != nil {
		t.Fatalf("public use: %v", err)
	}
	if got, want := axisSnapshot(a1, AxisKind), axisSnapshot(a2, AxisKind); got != want {
		t.Errorf("kind axis changed with acquisition context:\n got %s\nwant %s", got, want)
	}

	electronic := base
	electronic.Kind = KindElectronic
	electronic.Acquisition = EvTypeInternetPublication
	electronic.ContentHash = "sha256:abc"
	electronic.HashVerified = new(true)
	electronic.Timestamp = "2025-03-01"
	electronic.StorageIntegrityVerified = new(true)
	electronic.PublicDateClear = new(true)
	electronic.PubliclyAccessible = new(true)

	a3, err := AssessStatutoryEvidence(electronic)
	if err != nil {
		t.Fatalf("electronic: %v", err)
	}
	if got, want := axisSnapshot(a3, AxisAcquisition), axisSnapshot(a1, AxisAcquisition); got != want {
		t.Errorf("acquisition axis changed with evidence kind:\n got %s\nwant %s", got, want)
	}
}

// axisSnapshot 把某一轴的 finding 压缩成可比较的字符串（含分值贡献）。
func axisSnapshot(a StatutoryAssessment, axis string) string {
	var parts []string
	for _, f := range a.Findings {
		if f.Axis != axis {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s=%.2f", f.RuleID, boolText(f.Passed), f.Delta))
	}
	return strings.Join(parts, ",")
}

func boolText(v bool) string {
	if v {
		return "pass"
	}
	return "fail"
}

// 未知种类/场景必须报错，不降级为通用评分。
func TestAssessStatutoryEvidence_UnknownAxesRejected(t *testing.T) {
	_, err := AssessStatutoryEvidence(StatutoryEvidenceInput{Kind: "hearsay"})
	if err == nil || !strings.Contains(err.Error(), "未知的法定证据种类") {
		t.Fatalf("expected unknown-kind error, got %v", err)
	}

	_, err = AssessStatutoryEvidence(StatutoryEvidenceInput{
		Kind:        KindDocumentary,
		Acquisition: EvidenceType("telepathy"),
	})
	if err == nil || !strings.Contains(err.Error(), "未知的取证场景") {
		t.Fatalf("expected unknown-acquisition error, got %v", err)
	}
}

// 负控制：书证未提供原件必须被标记为严重问题，且真实性低于提供原件的同份证据。
func TestAssessStatutoryEvidence_MissingOriginalFlagged(t *testing.T) {
	missing := fullDocumentaryInput()
	missing.HasOriginal = new(false)

	got, err := AssessStatutoryEvidence(missing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, ok := findingsByRule(got)["LEGAL-EVI-001"]
	if !ok {
		t.Fatal("missing LEGAL-EVI-001 finding")
	}
	if f.Passed || f.Severity != judgmentCritical {
		t.Errorf("LEGAL-EVI-001 = passed %v severity %q, want failed/critical", f.Passed, f.Severity)
	}

	withOriginal, err := AssessStatutoryEvidence(fullDocumentaryInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Authenticity.Score >= withOriginal.Authenticity.Score {
		t.Errorf("authenticity without original %f >= with original %f",
			got.Authenticity.Score, withOriginal.Authenticity.Score)
	}
}

// 负控制：证人有未披露的利害关系必须被标记为严重问题。
func TestAssessStatutoryEvidence_WitnessConflictFlagged(t *testing.T) {
	in := StatutoryEvidenceInput{
		Kind:               KindWitnessTestimony,
		WitnessHasConflict: new(true),
		Corroborated:       new(true),
	}
	got, err := AssessStatutoryEvidence(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, ok := findingsByRule(got)["LEGAL-EVI-007"]
	if !ok || f.Passed || f.Severity != judgmentCritical {
		t.Fatalf("LEGAL-EVI-007 = %+v, want failed/critical", f)
	}

	disclosed := in
	disclosed.ConflictDisclosed = new(true)
	got, err = AssessStatutoryEvidence(disclosed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f := findingsByRule(got)["LEGAL-EVI-007"]; !f.Passed {
		t.Errorf("disclosed conflict should pass, got %+v", f)
	}
}

// 负控制：电子数据未核验存储完整性必须被标记为严重问题。
func TestAssessStatutoryEvidence_ElectronicIntegrityFlagged(t *testing.T) {
	in := StatutoryEvidenceInput{
		Kind:        KindElectronic,
		ContentHash: "sha256:abc",
		Timestamp:   "2025-03-01",
	}
	got, err := AssessStatutoryEvidence(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, ok := findingsByRule(got)["LEGAL-EVI-004"]
	if !ok || f.Passed || f.Severity != judgmentCritical {
		t.Fatalf("LEGAL-EVI-004 = %+v, want failed/critical", f)
	}
}

// 负控制：电子数据/证人证言明确声明形式不合法时必须扣分，不能被静默忽略。
func TestAssessStatutoryEvidence_FormIllegalPenalized(t *testing.T) {
	for _, kind := range []EvidenceKind{KindElectronic, KindWitnessTestimony} {
		legal := StatutoryEvidenceInput{
			Kind: kind, FormLegal: new(true), CollectionLegal: new(true),
			ContentHash: "sha256:abc", Timestamp: "2025-03-01",
			StorageIntegrityVerified: new(true),
		}
		illegal := legal
		illegal.FormLegal = new(false)
		undeclared := legal
		undeclared.FormLegal = nil

		a1, err := AssessStatutoryEvidence(legal)
		if err != nil {
			t.Fatalf("%s legal: %v", kind, err)
		}
		a2, err := AssessStatutoryEvidence(illegal)
		if err != nil {
			t.Fatalf("%s illegal: %v", kind, err)
		}
		a0, err := AssessStatutoryEvidence(undeclared)
		if err != nil {
			t.Fatalf("%s undeclared: %v", kind, err)
		}
		if a2.Legality.Score >= a1.Legality.Score {
			t.Errorf("%s: illegal form legality %f should be below legal %f",
				kind, a2.Legality.Score, a1.Legality.Score)
		}
		if a2.Legality.Score >= a0.Legality.Score {
			t.Errorf("%s: illegal form legality %f should be below undeclared %f",
				kind, a2.Legality.Score, a0.Legality.Score)
		}
		if !strings.Contains(a2.Legality.Reasoning, "形式合法性存疑") {
			t.Errorf("%s: reasoning missing 形式合法性存疑: %s", kind, a2.Legality.Reasoning)
		}
	}
}

// 负控制：鉴定意见明确声明收集不合法时必须记录严重问题，不能被静默忽略。
func TestAssessStatutoryEvidence_ExpertIllegalCollectionFlagged(t *testing.T) {
	in := StatutoryEvidenceInput{
		Kind: KindExpertOpinion, CollectionLegal: new(false),
		ExpertQualified: new(true), ExpertProcedureLegal: new(true),
	}
	got, err := AssessStatutoryEvidence(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, ok := findingsByRule(got)["LEGAL-EVI-GEN-002"]
	if !ok || f.Passed || f.Severity != judgmentCritical {
		t.Fatalf("LEGAL-EVI-GEN-002 = %+v, want failed/critical", f)
	}

	legal := in
	legal.CollectionLegal = new(true)
	a1, err := AssessStatutoryEvidence(legal)
	if err != nil {
		t.Fatalf("legal: %v", err)
	}
	if got.Legality.Score >= a1.Legality.Score {
		t.Errorf("illegal collection legality %f should be below legal %f",
			got.Legality.Score, a1.Legality.Score)
	}
}

// 未声明与明确为否的处理必须不同：未声明只记未通过，不扣分。
func TestAssessStatutoryEvidence_UndeclaredDoesNotPenalize(t *testing.T) {
	undeclared := fullDocumentaryInput()
	undeclared.HasOriginal = nil

	explicitFalse := fullDocumentaryInput()
	explicitFalse.HasOriginal = new(false)

	a1, err := AssessStatutoryEvidence(undeclared)
	if err != nil {
		t.Fatalf("undeclared: %v", err)
	}
	a2, err := AssessStatutoryEvidence(explicitFalse)
	if err != nil {
		t.Fatalf("explicit false: %v", err)
	}
	if a1.Authenticity.Score <= a2.Authenticity.Score {
		t.Errorf("undeclared authenticity %f should be higher than explicit false %f",
			a1.Authenticity.Score, a2.Authenticity.Score)
	}
	f := findingsByRule(a1)["LEGAL-EVI-001"]
	if f.Passed || !strings.Contains(f.Reason, "未声明") {
		t.Errorf("undeclared finding = %+v, want failed with 未声明 reason", f)
	}
}

// 极端输入下三项评分仍须落在 [0,1]。
func TestAssessStatutoryEvidence_ScoreBounds(t *testing.T) {
	in := StatutoryEvidenceInput{
		Kind:                     KindElectronic,
		Acquisition:              EvTypeInternetPublication,
		CollectionLegal:          new(false),
		FormLegal:                new(false),
		StorageIntegrityVerified: new(false),
		PublicDateClear:          new(false),
		PubliclyAccessible:       new(false),
	}
	got, err := AssessStatutoryEvidence(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range []float64{got.Relevance.Score, got.Legality.Score, got.Authenticity.Score, got.Overall} {
		if s < 0 || s > 1 {
			t.Errorf("score %f outside [0,1]", s)
		}
	}
}

func TestStatutoryTool_Run(t *testing.T) {
	tool := newStatutoryTool()
	if !tool.ReadOnly {
		t.Error("evaluate_legal_evidence must be read-only")
	}
	required, ok := tool.Parameters["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "kind" {
		t.Errorf("required = %v, want [kind]", tool.Parameters["required"])
	}

	args := `{"kind":"documentary","has_original":true,"signature_verified":true,"fact_to_prove":"合同成立"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assessment, ok := result.(StatutoryAssessment)
	if !ok {
		t.Fatalf("expected StatutoryAssessment, got %T", result)
	}
	if assessment.EvidenceID == "" {
		t.Error("expected auto-generated evidence id")
	}
	if assessment.Kind != KindDocumentary {
		t.Errorf("kind = %q, want documentary", assessment.Kind)
	}
}

func TestStatutoryTool_RejectsUnknownKind(t *testing.T) {
	tool := newStatutoryTool()
	_, err := tool.Func(context.Background(), json.RawMessage(`{"kind":"hearsay"}`))
	if err == nil || !strings.Contains(err.Error(), "未知的法定证据种类") {
		t.Fatalf("expected unknown-kind error, got %v", err)
	}
}
