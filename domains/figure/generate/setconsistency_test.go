package generate

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExtractNumeralRefs(t *testing.T) {
	text := "如图(1)所示，公式(3)中，壳体(10)的盖板(20)，表(2)与步骤(4)。"
	got := ExtractNumeralRefs(text, 0)
	want := []int{10, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refs = %v, want %v", got, want)
	}
	if limited := ExtractNumeralRefs(text, 1); !reflect.DeepEqual(limited, []int{10}) {
		t.Errorf("limit 1 = %v, want [10]", limited)
	}
	if fullWidth := ExtractNumeralRefs("壳体（10）与盖板（20）连接。", 0); !reflect.DeepEqual(fullWidth, []int{10, 20}) {
		t.Errorf("full-width = %v, want [10 20]", fullWidth)
	}
	if direct := ExtractNumeralRefs("壳体20，盖板30。", 0); !reflect.DeepEqual(direct, []int{20, 30}) {
		t.Errorf("direct = %v, want [20 30]", direct)
	}
	if empty := ExtractNumeralRefs("   ", 0); empty != nil {
		t.Errorf("empty = %v, want nil", empty)
	}
}

func TestExtractNumeralRefsInlineRefs(t *testing.T) {
	// 标记后紧跟连词或单字冠词：真实专利文本里最常见的写法，不能漏抽。
	cases := map[string][]int{
		"壳体10与盖板20连接。": {10, 20},
		"壳体10和盖板20固定。": {10, 20},
		"轴10上套设有轴承20。": {10, 20},
		"件10的侧面贴合件20。": {10, 20},
		"10 壳体\n20 盖板": {10, 20},
	}
	for text, want := range cases {
		if got := ExtractNumeralRefs(text, 0); !reflect.DeepEqual(got, want) {
			t.Errorf("ExtractNumeralRefs(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestExtractNumeralRefsGuards(t *testing.T) {
	text := "权利要求1所述的结构，图1所示，公式(3)，实施例5，步骤4，共3个，" +
		"附图标记10，2020年10月1日，厚度10mm，含量10％，夹角30°，" +
		"S10为驱动信号，S20为反馈信号，间隙0.5毫米。"
	if got := ExtractNumeralRefs(text, 0); got != nil {
		t.Errorf("guards = %v, want nil", got)
	}
}

func TestHasNumeralPrefix(t *testing.T) {
	cases := map[string]bool{
		"公式":      true,
		"方式":      false,
		"附图":      true,
		"图":       true,
		"实施例":     true,
		"步骤":      true,
		"本发明":     false,
		"权利要求":    true,
		"标记":      true,
		"编号":      true,
		"序号":      true,
		"年":       true,
		"formula": true,
		"Form":    false,
	}
	for prefix, want := range cases {
		if got := HasNumeralPrefix(prefix); got != want {
			t.Errorf("HasNumeralPrefix(%q) = %v, want %v", prefix, got, want)
		}
	}
}

func TestCheckFigureSetClean(t *testing.T) {
	report, err := CheckFigureSet(SetConsistencyInput{
		Figures: []SetFigure{
			{Number: 1, Refs: []SetRef{{Ref: "10", Name: "壳体"}, {Ref: "20", Name: "盖板"}}},
			{Number: 2, Refs: []SetRef{{Ref: "10", Name: "壳体"}}},
		},
		Text: "如图1所示，壳体(10)与盖板(20)固定连接。",
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if !report.Consistent {
		t.Fatalf("expected consistent, got %+v", report)
	}
	if len(report.Conflicts) != 0 || len(report.MissingRefs) != 0 {
		t.Errorf("unexpected findings: %+v", report)
	}
	if !strings.Contains(report.Summary, "一致") {
		t.Errorf("summary = %q", report.Summary)
	}
}

func TestCheckFigureSetNameConflict(t *testing.T) {
	report, err := CheckFigureSet(SetConsistencyInput{
		Figures: []SetFigure{
			{Number: 1, Refs: []SetRef{{Ref: "10", Name: "壳体"}}},
			{Number: 2, Refs: []SetRef{{Ref: "10", Name: "基座"}}},
		},
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if report.Consistent {
		t.Fatal("expected inconsistent")
	}
	if len(report.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want 1", report.Conflicts)
	}
	conflict := report.Conflicts[0]
	if conflict.Ref != "10" || !reflect.DeepEqual(conflict.FigureNumbers, []int{1, 2}) {
		t.Errorf("conflict = %+v", conflict)
	}
	if len(conflict.Values) != 2 || conflict.Values[0].Value != "壳体" || conflict.Values[1].Value != "基座" {
		t.Errorf("values = %+v", conflict.Values)
	}
	if !strings.Contains(conflict.Message, "名称不一致") {
		t.Errorf("message = %q", conflict.Message)
	}
}

func TestCheckFigureSetIgnoresUnnamedRefs(t *testing.T) {
	// 负控制：同一标记在一张图上无名称时，不构成名称冲突。
	report, err := CheckFigureSet(SetConsistencyInput{
		Figures: []SetFigure{
			{Number: 1, Refs: []SetRef{{Ref: "10", Name: "壳体"}}},
			{Number: 2, Refs: []SetRef{{Ref: "10"}}},
		},
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if len(report.Conflicts) != 0 || !report.Consistent {
		t.Errorf("unnamed ref should not conflict: %+v", report)
	}
}

func TestCheckFigureSetNumberGaps(t *testing.T) {
	report, err := CheckFigureSet(SetConsistencyInput{
		Figures: []SetFigure{{Number: 1}, {Number: 1}, {Number: 3}},
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if !reflect.DeepEqual(report.DuplicateFigureNumbers, []int{1}) {
		t.Errorf("duplicates = %v, want [1]", report.DuplicateFigureNumbers)
	}
	if !reflect.DeepEqual(report.MissingFigureNumbers, []int{2}) {
		t.Errorf("missing = %v, want [2]", report.MissingFigureNumbers)
	}
	if report.Consistent {
		t.Error("expected inconsistent")
	}
	if len(report.Warnings) != 2 {
		t.Errorf("warnings = %v, want 图号重复 + 图号不连续", report.Warnings)
	}
}

func TestCheckFigureSetMissingTextRefs(t *testing.T) {
	report, err := CheckFigureSet(SetConsistencyInput{
		Figures: []SetFigure{{Number: 1, Refs: []SetRef{{Ref: "10", Name: "壳体"}}}},
		Text:    "壳体(10)与螺钉(30)连接。",
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if !reflect.DeepEqual(report.MissingRefs, []int{30}) {
		t.Errorf("missing refs = %v, want [30]", report.MissingRefs)
	}
	if report.Consistent {
		t.Error("expected inconsistent")
	}
}

func TestCheckFigureSetAbstractFigure(t *testing.T) {
	report, err := CheckFigureSet(SetConsistencyInput{
		Figures:        []SetFigure{{Number: 1}, {Number: 2}},
		AbstractFigure: 5,
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if !strings.Contains(report.AbstractFigureProblem, "不在附图集合") {
		t.Errorf("problem = %q", report.AbstractFigureProblem)
	}
	if report.Consistent {
		t.Error("expected inconsistent")
	}

	ok, err := CheckFigureSet(SetConsistencyInput{
		Figures:        []SetFigure{{Number: 1}, {Number: 2}},
		AbstractFigure: 2,
	})
	if err != nil {
		t.Fatalf("CheckFigureSet: %v", err)
	}
	if ok.AbstractFigureProblem != "" || !ok.Consistent {
		t.Errorf("abstract figure 2 should be valid: %+v", ok)
	}
}

func TestCheckFigureSetErrors(t *testing.T) {
	if _, err := CheckFigureSet(SetConsistencyInput{}); errorCode(t, err) != ErrEmptyInput {
		t.Fatalf("empty: %v", err)
	}
	if _, err := CheckFigureSet(SetConsistencyInput{
		Figures: []SetFigure{{Number: 0}},
	}); errorCode(t, err) != ErrInvalidLabel {
		t.Fatalf("number 0: %v", err)
	}
}

func TestConsistencyToolRun(t *testing.T) {
	tool := NewConsistencyTool()
	raw, err := tool.Func(context.Background(), json.RawMessage(`{
		"figures": [
			{"number": 1, "refs": [{"ref": "10", "name": "壳体"}]},
			{"number": 3, "refs": [{"ref": "10", "name": "基座"}]}
		],
		"abstract_figure": 2
	}`))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	report, ok := raw.(SetConsistencyReport)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	if report.FigureCount != 2 {
		t.Errorf("figure count = %d", report.FigureCount)
	}
	if !reflect.DeepEqual(report.MissingFigureNumbers, []int{2}) {
		t.Errorf("missing = %v, want [2]", report.MissingFigureNumbers)
	}
	if len(report.Conflicts) != 1 || report.AbstractFigureProblem == "" {
		t.Errorf("report = %+v", report)
	}
	if !tool.ReadOnly {
		t.Error("consistency tool should be read-only")
	}
}
