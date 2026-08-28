package slop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestCheck_CleanTechnicalText(t *testing.T) {
	// 有定量数据的技术文本：不应命中效果断言（数据支撑防线）。
	text := "实施例1中，处理器的温度从85℃降至62℃，降幅达27%。对比对比例1，本方案的散热效率提升了23%，且功耗降低15%。"
	report := Check(text)
	if report.Verdict != VerdictPass {
		t.Errorf("clean text should pass, got %s: %+v", report.Verdict, report.Findings)
	}
}

func TestCheck_UnsupportedEffectAssertion(t *testing.T) {
	// 无数据的效果断言：应命中。
	text := "本发明采用上述结构，具有显著的有益效果，散热性能得到显著提高。"
	report := Check(text)
	found := false
	for _, f := range report.Findings {
		if f.Category == CatUnsupportedEffect {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsupported-effect finding, got %+v", report.Findings)
	}
}

func TestCheck_NegationGuard(t *testing.T) {
	// 否定语境："效果并不显著"不是断言，不应命中。
	text := "对比例2的效果并不显著，说明缺少该组分无法达到预期。"
	report := Check(text)
	for _, f := range report.Findings {
		if f.Category == CatUnsupportedEffect {
			t.Errorf("negated sentence must not be flagged: %+v", f)
		}
	}
}

func TestCheck_ConclusoryRemark(t *testing.T) {
	text := "该方案与现有技术的区别显然是显而易见的，无需进一步说明。"
	report := Check(text)
	found := false
	for _, f := range report.Findings {
		if f.Category == CatConclusoryRemark {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected conclusory-remark finding, got %+v", report.Findings)
	}
}

func TestCheck_ConclusoryWithCitationAllowed(t *testing.T) {
	// 带对比文件引证的结论式表述可接受（引证防线）。
	text := "对比文件1的电路结构与本方案相同，本领域技术人员容易想到将该结构用于本申请的场景。"
	report := Check(text)
	for _, f := range report.Findings {
		if f.Category == CatConclusoryRemark {
			t.Errorf("cited conclusory remark should be allowed: %+v", f)
		}
	}
}

func TestCheck_BoilerplateFiller(t *testing.T) {
	text := "本方案具有广泛的应用前景。其他参数根据实际需要选择即可。"
	report := Check(text)
	found := 0
	for _, f := range report.Findings {
		if f.Category == CatBoilerplateFiller {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected 2 boilerplate findings, got %d: %+v", found, report.Findings)
	}
}

func TestCheck_RepeatedSentence(t *testing.T) {
	s := "所述壳体包括底座和上盖，所述底座设有散热孔组。"
	text := s + "另外一些特征如下。" + s
	report := Check(text)
	found := false
	for _, f := range report.Findings {
		if f.Category == CatRepeatedBoilerplate {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected repeated-boilerplate finding, got %+v", report.Findings)
	}
}

func TestCheck_VerdictNeedsRevision(t *testing.T) {
	// 加权命中 ≥5：直接 needs_revision（2 条效果断言=4 分 + 1 条空话=1 分）。
	text := "本发明效果十分显著。本方案取得了意料不到的技术效果。本方案具有广泛的应用前景。"
	report := Check(text)
	if report.Verdict != VerdictNeedsRevision {
		t.Errorf("dense slop should need revision, got %s: %+v", report.Verdict, report.Findings)
	}
}

func TestCheck_EmptyText(t *testing.T) {
	report := Check("  ")
	if report.Verdict != VerdictPass {
		t.Errorf("empty text should pass, got %s", report.Verdict)
	}
}

func TestSlopGateTool_EmptyInput(t *testing.T) {
	tool := NewSlopGateTool()
	result, err := tool.Func(context.Background(), json.RawMessage(`{"text":""}`))
	if err != nil {
		t.Fatal(err)
	}
	hr, ok := result.(agentcore.HandoffResult)
	if !ok || hr.Success || !strings.Contains(hr.Result, "text 不能为空") {
		t.Errorf("expected empty-input failure, got %#v", result)
	}
}

func TestSlopGateTool_Output(t *testing.T) {
	tool := NewSlopGateTool()
	result, err := tool.Func(context.Background(), json.RawMessage(`{"text":"本发明效果十分显著。本方案取得了意料不到的技术效果。"}`))
	if err != nil {
		t.Fatal(err)
	}
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string output, got %T", result)
	}
	var out struct {
		Ok          bool   `json:"ok"`
		Verdict     string `json:"verdict"`
		FindingNum  int    `json:"finding_num"`
		RevisionHit string `json:"revision_hint"`
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Ok || out.Verdict != VerdictNeedsRevision || out.FindingNum < 2 {
		t.Errorf("unexpected tool output: %s", s)
	}
}
