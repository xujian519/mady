package figure

import (
	"reflect"
	"testing"
)

func figWith(n int, comps []ElectricalComponent, nets []ElectricalNet) FigureAnalysisResult {
	return FigureAnalysisResult{FigureNumber: n, Electrical: &ElectricalAnalysis{Components: comps, Nets: nets}}
}

func TestCheckFigureConsistency(t *testing.T) {
	resistor := ElectricalComponent{Ref: "R1", Symbol: "resistor", Category: "passive", Name: "电阻", Value: "10kΩ"}
	capacitor := ElectricalComponent{Ref: "C2", Symbol: "capacitor", Category: "passive", Name: "电容"}

	cases := []struct {
		name            string
		figures         []FigureAnalysisResult
		claimContext    string
		wantConflicts   int
		wantMissing     []int
		wantMissingRefs []string
		wantOrphan      []string
		wantConsistent  bool
	}{
		{
			name:           "空输入",
			figures:        nil,
			wantConsistent: true,
		},
		{
			name: "单图无冲突",
			figures: []FigureAnalysisResult{
				figWith(1, []ElectricalComponent{resistor}, []ElectricalNet{{Name: "VCC", ConnectedRefs: []string{"R1.1"}}}),
			},
			wantConsistent: true,
		},
		{
			name: "跨图 symbol 冲突",
			figures: []FigureAnalysisResult{
				figWith(1, []ElectricalComponent{resistor}, nil),
				figWith(2, []ElectricalComponent{{Ref: "R1", Symbol: "capacitor", Category: "passive", Name: "电阻"}}, nil),
			},
			wantConflicts:  1,
			wantConsistent: false,
		},
		{
			name: "图号不连续（1、3 缺 2）",
			figures: []FigureAnalysisResult{
				figWith(1, []ElectricalComponent{resistor}, nil),
				figWith(3, []ElectricalComponent{capacitor}, nil),
			},
			wantMissing:    []int{2},
			wantConsistent: false,
		},
		{
			name: "claim 引用未在图出现",
			figures: []FigureAnalysisResult{
				figWith(1, []ElectricalComponent{resistor}, nil),
			},
			claimContext:    "包括电阻R1和电容C2",
			wantMissingRefs: []string{"C2"},
			wantConsistent:  false,
		},
		{
			name: "单引脚非电源网络孤立",
			figures: []FigureAnalysisResult{
				figWith(1, []ElectricalComponent{resistor}, []ElectricalNet{
					{Name: "N1", ConnectedRefs: []string{"R1.1"}},
					{Name: "GND", ConnectedRefs: []string{"R1.2"}},
				}),
			},
			wantOrphan:     []string{"N1"},
			wantConsistent: true, // 孤立网络不计入一致性命门
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CheckFigureConsistency(c.figures, c.claimContext)
			if got.Consistent != c.wantConsistent {
				t.Errorf("Consistent = %v, want %v", got.Consistent, c.wantConsistent)
			}
			if len(got.Conflicts) != c.wantConflicts {
				t.Errorf("len(Conflicts) = %d, want %d (%+v)", len(got.Conflicts), c.wantConflicts, got.Conflicts)
			}
			if !reflect.DeepEqual(got.MissingFigureNumbers, c.wantMissing) {
				t.Errorf("MissingFigureNumbers = %v, want %v", got.MissingFigureNumbers, c.wantMissing)
			}
			if !reflect.DeepEqual(got.MissingRefs, c.wantMissingRefs) {
				t.Errorf("MissingRefs = %v, want %v", got.MissingRefs, c.wantMissingRefs)
			}
			if !reflect.DeepEqual(got.OrphanNets, c.wantOrphan) {
				t.Errorf("OrphanNets = %v, want %v", got.OrphanNets, c.wantOrphan)
			}
		})
	}
}

func TestCheckFigureConsistencyConflictDetail(t *testing.T) {
	got := CheckFigureConsistency([]FigureAnalysisResult{
		figWith(1, []ElectricalComponent{{Ref: "R1", Symbol: "resistor", Category: "passive", Name: "电阻"}}, nil),
		figWith(2, []ElectricalComponent{{Ref: "R1", Symbol: "capacitor", Category: "passive", Name: "电阻"}}, nil),
	}, "")
	if len(got.Conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(got.Conflicts))
	}
	c := got.Conflicts[0]
	if c.Kind != "symbol" || c.Ref != "R1" {
		t.Errorf("conflict = %+v, want kind=symbol ref=R1", c)
	}
	if !reflect.DeepEqual(c.FigureNumbers, []int{1, 2}) {
		t.Errorf("FigureNumbers = %v, want [1 2]", c.FigureNumbers)
	}
	wantValues := []ConflictValue{{FigureNumber: 1, Value: "resistor"}, {FigureNumber: 2, Value: "capacitor"}}
	if !reflect.DeepEqual(c.Values, wantValues) {
		t.Errorf("Values = %+v, want %+v", c.Values, wantValues)
	}
	wantMsg := "标记 R1 在图 1,2 中 symbol 不一致：resistor / capacitor"
	if c.Message != wantMsg {
		t.Errorf("Message = %q, want %q", c.Message, wantMsg)
	}
	if got.Summary == "" {
		t.Error("Summary 不应为空")
	}
	if len(got.Warnings) == 0 || got.Warnings[0] != wantMsg {
		t.Errorf("Warnings = %v, want 首条为冲突消息", got.Warnings)
	}
}

func TestCheckFigureConsistencyGlobalMerge(t *testing.T) {
	got := CheckFigureConsistency([]FigureAnalysisResult{
		figWith(1, []ElectricalComponent{{Ref: "R1", Symbol: "resistor", Category: "passive", Name: "电阻"}}, nil),
		figWith(2, []ElectricalComponent{{Ref: "R1", Symbol: "resistor", Category: "passive", Name: "电阻", Value: "10kΩ"}}, nil),
	}, "")
	gc, ok := got.GlobalComponents["R1"]
	if !ok {
		t.Fatal("R1 未合并进 GlobalComponents")
	}
	if !reflect.DeepEqual(gc.FigureNumbers, []int{1, 2}) {
		t.Errorf("FigureNumbers = %v, want [1 2]", gc.FigureNumbers)
	}
	// 更具体的值（图2 带 value）应被保留
	if gc.Value != "10kΩ" {
		t.Errorf("Value = %q, want 10kΩ", gc.Value)
	}
	if !got.Consistent {
		t.Error("无维度冲突时应 Consistent")
	}
}

func TestExtractClaimRefs(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"包括电阻R1、电容C2和集成电路IC1", []string{"R1", "C2", "IC1"}},
		{"电阻 R1", []string{"R1"}},
		{"数字123与5号件", []string{}},
		{"见图5所示实施例", []string{}},
		{"含有M2标记", []string{}}, // M 非符号库前缀
		{"", nil},
	}
	for _, c := range cases {
		if got := ExtractClaimRefs(c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExtractClaimRefs(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestIsPowerNet(t *testing.T) {
	power := []string{"GND", "vcc", "VDD", "VEE", "VCC3.3", "vdd_io"}
	for _, n := range power {
		if !isPowerNet(n) {
			t.Errorf("isPowerNet(%q) = false, want true", n)
		}
	}
	nonPower := []string{"N1", "SIGNAL", "OUT", "n2"}
	for _, n := range nonPower {
		if isPowerNet(n) {
			t.Errorf("isPowerNet(%q) = true, want false", n)
		}
	}
}
