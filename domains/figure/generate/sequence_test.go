package generate

import (
	"strings"
	"testing"
)

func sampleSequence() ([]SequenceParticipant, []SequenceMessage) {
	participants := []SequenceParticipant{
		{ID: "ui", Label: "界面"},
		{ID: "core", Label: "核心模块"},
		{ID: "db", Label: "存储"},
	}
	messages := []SequenceMessage{
		{From: "ui", To: "core", Label: "请求"},
		{From: "core", To: "ui", Label: "响应", Kind: "return"},
		{From: "core", To: "db", Label: "写入", Kind: "async", Activate: true},
	}
	return participants, messages
}

func TestBuildSequenceDiagram(t *testing.T) {
	participants, messages := sampleSequence()
	figure, err := BuildSequenceDiagram(participants, messages, SequenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if figure.WidthMm <= 0 || figure.HeightMm <= 0 {
		t.Fatalf("尺寸异常: %+v", figure)
	}
	for _, want := range []string{
		`<rect`, `stroke="#000000"`, `>界面<`, `>核心模块<`,
		`>请求<`, `stroke-dasharray="1.5 1"`, `fill="none"`,
	} {
		if !strings.Contains(figure.SVG, want) {
			t.Errorf("SVG 缺少 %q", want)
		}
	}
	if len(figure.Labels) != 6 {
		t.Errorf("labels = %v, want 6 项", figure.Labels)
	}
}

func TestBuildSequenceDiagramErrors(t *testing.T) {
	participants, _ := sampleSequence()
	if _, err := BuildSequenceDiagram(nil, nil, SequenceOptions{}); errorCode(t, err) != ErrEmptyInput {
		t.Errorf("空参与者应报 empty_input, got %v", err)
	}
	unknown := []SequenceMessage{{From: "ui", To: "missing", Label: "x"}}
	if _, err := BuildSequenceDiagram(participants, unknown, SequenceOptions{}); err == nil {
		t.Error("未知终点应报错")
	}
	selfLoop := []SequenceMessage{{From: "ui", To: "ui", Label: "x"}}
	if _, err := BuildSequenceDiagram(participants, selfLoop, SequenceOptions{}); err == nil {
		t.Error("自环应报错")
	}
	badKind := []SequenceMessage{{From: "ui", To: "core", Label: "x", Kind: "signal"}}
	if _, err := BuildSequenceDiagram(participants, badKind, SequenceOptions{}); err == nil {
		t.Error("未知消息类型应报错")
	}
}

// 正常时序图应通过渲染复核（净距档位 0.7 毫米）。
func TestSequencePassesRenderCheck(t *testing.T) {
	participants, messages := sampleSequence()
	figure, err := BuildSequenceDiagram(participants, messages, SequenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	report, err := CheckFigure(figure.SVG, CheckOptions{FigureType: "sequence"})
	if err != nil {
		t.Fatal(err)
	}
	if blocking := BlockingFindings(report); len(blocking) != 0 {
		t.Fatalf("时序图不应有阻断级发现: %+v", blocking)
	}
	if report.TextCount == 0 {
		t.Error("应量测到文字")
	}
}
