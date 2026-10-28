package generate

import (
	"strings"
	"testing"
)

func errorCode(t *testing.T, err error) ErrorCode {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	classified, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	return classified.Code
}

func TestAssignNumerals(t *testing.T) {
	ids := []string{"a", "b", "c"}

	assignments, err := AssignNumerals(ids, NumeralOptions{FigureNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"100", "102", "104"}
	for i, assignment := range assignments {
		if assignment.Numeral != want[i] {
			t.Errorf("%s numeral = %s, want %s", assignment.ID, assignment.Numeral, want[i])
		}
	}

	assignments, err = AssignNumerals(ids, NumeralOptions{FigureNumber: 2})
	if err != nil {
		t.Fatal(err)
	}
	if assignments[0].Numeral != "200" {
		t.Errorf("图2 起点 = %s, want 200", assignments[0].Numeral)
	}

	// 显式标号优先，自动分配跳过已占用号。
	assignments, err = AssignNumerals(ids, NumeralOptions{
		FigureNumber: 1,
		Explicit:     map[string]string{"b": "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if assignments[1].Numeral != "100" || assignments[0].Numeral != "102" {
		t.Errorf("显式标号未生效: %+v", assignments)
	}

	// reserved 只约束自动分配。
	assignments, err = AssignNumerals([]string{"a"}, NumeralOptions{FigureNumber: 1, Reserved: []string{"100", "102"}})
	if err != nil {
		t.Fatal(err)
	}
	if assignments[0].Numeral != "104" {
		t.Errorf("reserved 未跳过: %s", assignments[0].Numeral)
	}

	if _, err := AssignNumerals(ids, NumeralOptions{
		FigureNumber: 1,
		Explicit:     map[string]string{"a": "100", "b": "100"},
	}); errorCode(t, err) != ErrConflictingNumeral {
		t.Errorf("重复显式标号应报 conflicting_numeral, got %v", err)
	}
}

func TestBuildFlowchart(t *testing.T) {
	steps := []Step{
		{ID: "start", Label: "开始", Shape: "ellipse", Next: []Next{{ID: "check"}}},
		{ID: "check", Label: "合格？", Shape: "diamond", Next: []Next{
			{ID: "done", Label: "是"}, {ID: "start", Label: "否"},
		}},
		{ID: "done", Label: "结束", Shape: "ellipse"},
	}
	dot, err := BuildFlowchart(steps, Options{FigureNumber: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"digraph Flowchart {",
		`"start" [label="200. 开始", shape=ellipse];`,
		`"check" [label="202. 合格？", shape=diamond];`,
		`"check" -> "done" [label="是"];`,
		`"check" -> "start" [label="否"];`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT 缺少 %q\n%s", want, dot)
		}
	}
	if strings.Contains(dot, "fillcolor") {
		t.Error("grayscale 模式不应出现填充色")
	}
}

func TestBuildFlowchartRejectsBadInput(t *testing.T) {
	if _, err := BuildFlowchart(nil, Options{}); errorCode(t, err) != ErrEmptyInput {
		t.Errorf("空输入应报 empty_input, got %v", err)
	}

	decisionWithoutLabel := []Step{
		{ID: "a", Label: "判断", Shape: "diamond", Next: []Next{{ID: "b"}}},
		{ID: "b", Label: "结束"},
	}
	if _, err := BuildFlowchart(decisionWithoutLabel, Options{}); errorCode(t, err) != ErrMissingEdgeLabel {
		t.Errorf("决策分支缺标签应报 missing_edge_label, got %v", err)
	}

	unknownNext := []Step{
		{ID: "a", Label: "开始", Next: []Next{{ID: "missing"}}},
	}
	if _, err := BuildFlowchart(unknownNext, Options{}); errorCode(t, err) != ErrUnknownID {
		t.Errorf("未知后继应报 unknown_id, got %v", err)
	}

	badShape := []Step{{ID: "a", Label: "开始", Shape: "star"}}
	if _, err := BuildFlowchart(badShape, Options{}); errorCode(t, err) != ErrInvalidShape {
		t.Errorf("未知形状应报 invalid_shape, got %v", err)
	}
}

func TestBuildFlowchartEscapesLabels(t *testing.T) {
	dot, err := BuildFlowchart([]Step{
		{ID: "a", Label: `引号"与反斜杠\`, Next: nil},
	}, Options{EmbedNumerals: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dot, `label="引号\"与反斜杠\\"`) {
		t.Errorf("label 转义失败:\n%s", dot)
	}
}

func TestBuildBlockDiagram(t *testing.T) {
	blocks := []Block{
		{ID: "in", Label: "输入", Type: "input"},
		{ID: "proc", Label: "处理", Type: "process"},
	}
	connections := []Connection{{From: "in", To: "proc", Label: "信号"}}

	grayscale, err := BuildBlockDiagram(blocks, connections, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(grayscale, `"in" [label="输入 (100)", shape=invhouse];`) {
		t.Errorf("框图块形状/标号不符:\n%s", grayscale)
	}
	if strings.Contains(grayscale, "fillcolor") {
		t.Error("grayscale 不应填充")
	}

	semantic, err := BuildBlockDiagram(blocks, connections, Options{Style: StyleSemantic})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(semantic, "fillcolor=lightblue") || !strings.Contains(semantic, "style=filled") {
		t.Errorf("semantic 模式应输出填充:\n%s", semantic)
	}

	badType := []Block{{ID: "x", Label: "X", Type: "unknown"}}
	if _, err := BuildBlockDiagram(badType, nil, Options{}); errorCode(t, err) != ErrInvalidShape {
		t.Errorf("未知块类型应报 invalid_shape, got %v", err)
	}
	if _, err := BuildBlockDiagram(blocks, []Connection{{From: "in", To: "nope"}}, Options{}); errorCode(t, err) != ErrUnknownID {
		t.Errorf("未知连接目标应报 unknown_id, got %v", err)
	}
}

func TestBuildHierarchy(t *testing.T) {
	tree := []HierarchyNode{{
		ID: "root", Label: "系统",
		Children: []HierarchyNode{
			{ID: "a", Label: "部件A", Children: []HierarchyNode{{ID: "a1", Label: "零件A1"}}},
			{ID: "b", Label: "部件B"},
		},
	}}
	dot, err := BuildHierarchy(tree, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// 先序分配：root=100、a=102、a1=104、b=106。
	for _, want := range []string{
		`"root" [label="系统 (100)"];`,
		`"a" [label="部件A (102)"];`,
		`"a1" [label="零件A1 (104)"];`,
		`"b" [label="部件B (106)"];`,
		`"root" -> "a";`,
		`"a" -> "a1";`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT 缺少 %q\n%s", want, dot)
		}
	}

	duplicate := []HierarchyNode{{ID: "x", Label: "X"}, {ID: "x", Label: "Y"}}
	if _, err := BuildHierarchy(duplicate, Options{}); errorCode(t, err) != ErrUnknownID {
		t.Errorf("重复 id 应报 unknown_id, got %v", err)
	}
}

func TestBuildStateDiagram(t *testing.T) {
	states := []State{
		{ID: "start", Label: "", Kind: "initial"},
		{ID: "idle", Label: "待机"},
		{ID: "end", Label: "结束", Kind: "final"},
	}
	transitions := []Transition{
		{From: "start", To: "idle"},
		{From: "idle", To: "end", Label: "完成"},
	}
	dot, err := BuildStateDiagram(states, transitions, Options{FigureNumber: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dot, `"start" [label="", shape=circle, style=filled, fillcolor=black`) {
		t.Errorf("initial 伪状态不符:\n%s", dot)
	}
	if !strings.Contains(dot, `"idle" [label="300. 待机", shape=box, style=rounded];`) {
		t.Errorf("状态标号应从图3系列起点开始:\n%s", dot)
	}
	if !strings.Contains(dot, `"end" [label="302. 结束", shape=doublecircle];`) {
		t.Errorf("final 形状不符:\n%s", dot)
	}
	if !strings.Contains(dot, `"idle" -> "end" [label="完成"];`) {
		t.Errorf("转移标签缺失:\n%s", dot)
	}
}

func TestTemplates(t *testing.T) {
	for _, name := range AllTemplates() {
		dot, err := Template(name, Options{})
		if err != nil {
			t.Fatalf("模板 %s: %v", name, err)
		}
		if !strings.HasPrefix(dot, "digraph ") {
			t.Errorf("模板 %s 输出不是 DOT:\n%s", name, dot)
		}
	}

	method, err := Template(TemplateMethodSteps, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, numeral := range []string{"101", "102", "103", "104", "105"} {
		if !strings.Contains(method, numeral+". ") {
			t.Errorf("method_steps 缺少固定编号 %s", numeral)
		}
	}

	if _, err := Template("unknown", Options{}); errorCode(t, err) != ErrInvalidTemplate {
		t.Errorf("未知模板应报 invalid_template, got %v", err)
	}
}

func TestPageAttributes(t *testing.T) {
	dot, err := BuildFlowchart([]Step{{ID: "a", Label: "开始"}}, Options{
		Page: &PageOptions{Size: "a4", Orientation: "landscape", DPI: 300, MarginCm: 2.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`page="11.69,8.27";`,
		"dpi=300;",
		"orientation=landscape;",
		`margin="`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("页面属性缺少 %q\n%s", want, dot)
		}
	}

	bad := Options{Page: &PageOptions{Size: "a5"}}
	if _, err := BuildFlowchart([]Step{{ID: "a", Label: "x"}}, bad); errorCode(t, err) != ErrInvalidPage {
		t.Errorf("未知页面尺寸应报 invalid_page, got %v", err)
	}
}

func TestSanitizeID(t *testing.T) {
	if got, err := SanitizeID("步骤 1"); err != nil || got != "___1" {
		t.Errorf("SanitizeID = %q, %v", got, err)
	}
	if got, err := SanitizeID("中文"); err != nil || got != "__" {
		t.Errorf("SanitizeID(中文) = %q, %v", got, err)
	}
	if _, err := SanitizeID(""); err == nil {
		t.Error("空 id 应报 invalid_id")
	}
}
