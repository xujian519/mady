package specdrafting

import "testing"

// =============================================================================
// 结构完整性规则测试
// =============================================================================

func TestStructureSectionsRule_MissingSections(t *testing.T) {
	rule := &structureSectionsRule{baseRule: newBaseRule("structure-sections", "", "细则第18条")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		wantErr bool
	}{
		{"空说明书", nil, true},
		{"无章节", &SpecOutput{}, true},
		{"完整", &SpecOutput{Sections: []SpecSection{
			{Name: SecTechField}, {Name: SecBackground}, {Name: SecContent},
			{Name: SecDrawings}, {Name: SecEmbodiment},
		}}, false},
		{"缺少背景技术和实施方式", &SpecOutput{Sections: []SpecSection{
			{Name: SecTechField}, {Name: SecContent}, {Name: SecDrawings},
		}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, SpecInput{})) > 0
			if got != tt.wantErr {
				t.Errorf("Check() gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestStructureTitleLengthRule(t *testing.T) {
	rule := &structureTitleLengthRule{baseRule: newBaseRule("td", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		wantErr bool
	}{
		{"空标题", &SpecOutput{}, false},
		{"正常", &SpecOutput{Title: "一种挖掘机悬臂装置"}, false},
		{"超长", &SpecOutput{Title: "一种基于深度学习的用于自动化道路裂缝检测的图像识别系统和方法"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, SpecInput{})) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestStructureAbstractLengthRule(t *testing.T) {
	rule := &structureAbstractLengthRule{baseRule: newBaseRule("al", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		wantErr bool
	}{
		{"空", &SpecOutput{}, false},
		{"短", &SpecOutput{Abstract: "本发明涉及一种挖掘机。"}, false},
		{"超长", &SpecOutput{Abstract: genLongChinese(350)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, SpecInput{})) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestStructureContentTriadRule(t *testing.T) {
	rule := &structureContentTriadRule{baseRule: newBaseRule("ct", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		wantErr bool
	}{
		{"无内容", &SpecOutput{}, false},
		{"完整", &SpecOutput{Sections: []SpecSection{{
			Name:    SecContent,
			Content: "要解决的技术问题是提升效率。技术方案如下：包括一种新型结构。有益效果是提高了效率。",
		}}}, false},
		{"缺少技术方案", &SpecOutput{Sections: []SpecSection{{
			Name:    SecContent,
			Content: "要解决的技术问题是效率低下。有益效果是提高了效率。",
		}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, SpecInput{})) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestStructureEmbodimentDetailRule(t *testing.T) {
	rule := &structureEmbodimentDetailRule{baseRule: newBaseRule("ed", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		wantErr bool
	}{
		{"详尽", &SpecOutput{Sections: []SpecSection{{
			Name:    SecEmbodiment,
			Content: "下面结合附图详细说明。实施例1：包括悬臂主体和液压油缸，悬臂主体一端与机身铰接，另一端与液压油缸连接。液压油缸的伸缩控制悬臂的升降运动。",
		}}}, false},
		{"简略", &SpecOutput{Sections: []SpecSection{{
			Name:    SecEmbodiment,
			Content: "本发明的实施方式为上述技术方案。",
		}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, SpecInput{})) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// 清楚性规则测试
// =============================================================================

func TestClarityForbiddenWordsRule(t *testing.T) {
	rule := &clarityForbiddenWordsRule{baseRule: newBaseRule("fw", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		wantErr bool
	}{
		{"干净", &SpecOutput{Sections: []SpecSection{{Content: "本发明提供了一种高效节能的装置。"}}}, false},
		{"含禁止词", &SpecOutput{Sections: []SpecSection{{Content: "本发明性能卓越，市场广阔。"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, SpecInput{})) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestClarityPFEConsistencyRule(t *testing.T) {
	rule := &clarityPFEConsistencyRule{baseRule: newBaseRule("pfec", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		input   SpecInput
		wantErr bool
	}{
		{"一致", &SpecOutput{Sections: []SpecSection{{
			Name:    SecContent,
			Content: "要解决的技术问题是效率低下。有益效果是提高了效率。",
		}}}, SpecInput{Problems: []string{"效率低下"}, Effects: []string{"提高了效率"}}, false},
		{"问题未反映", &SpecOutput{Sections: []SpecSection{{
			Name:    SecContent,
			Content: "有益效果是提高了效率。",
		}}}, SpecInput{Problems: []string{"效率低下"}, Effects: []string{"提高了效率"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// 领域规则测试
// =============================================================================

func TestDomainMechanicalRule(t *testing.T) {
	rule := &domainMechanicalRule{baseRule: newBaseRule("dm", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		input   SpecInput
		wantErr bool
	}{
		{"非机械不触发", &SpecOutput{}, SpecInput{TechDomain: DomainSoftware}, false},
		{"机械含结构", &SpecOutput{Sections: []SpecSection{{Content: "壳体内部设置有连杆与齿轮连接。"}}},
			SpecInput{TechDomain: DomainMechanical}, false},
		{"机械缺结构", &SpecOutput{Sections: []SpecSection{{Content: "本发明涉及一种数据处理方法。"}}},
			SpecInput{TechDomain: DomainMechanical}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestDomainChemicalRule(t *testing.T) {
	rule := &domainChemicalRule{baseRule: newBaseRule("dc", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		input   SpecInput
		wantErr bool
	}{
		{"非化学不触发", &SpecOutput{}, SpecInput{TechDomain: DomainMechanical}, false},
		{"含组分数据", &SpecOutput{Sections: []SpecSection{{Content: "组分A含量50%，实验结果表明效果显著。"}}},
			SpecInput{TechDomain: DomainChemical}, false},
		{"缺组分数据", &SpecOutput{Sections: []SpecSection{{Content: "本发明提供了一种新材料。"}}},
			SpecInput{TechDomain: DomainChemical}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestDomainSoftwareRule(t *testing.T) {
	rule := &domainSoftwareRule{baseRule: newBaseRule("ds", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		input   SpecInput
		wantErr bool
	}{
		{"非软件不触发", &SpecOutput{}, SpecInput{TechDomain: DomainElectrical}, false},
		{"含方法步骤", &SpecOutput{Sections: []SpecSection{{Content: "步骤1：接收数据；步骤2：执行核心算法。"}}},
			SpecInput{TechDomain: DomainSoftware}, false},
		{"缺方法步骤", &SpecOutput{Sections: []SpecSection{{Content: "本发明是一种新型连接结构。"}}},
			SpecInput{TechDomain: DomainSoftware}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// 实用新型规则测试
// =============================================================================

func TestUtilityDrawingsRequiredRule(t *testing.T) {
	rule := &utilityDrawingsRequiredRule{baseRule: newBaseRule("udr", "", "")}
	tests := []struct {
		name    string
		spec    *SpecOutput
		input   SpecInput
		wantErr bool
	}{
		{"发明不触发", &SpecOutput{}, SpecInput{PatentType: PatentTypeInvention}, false},
		{"新型有图", &SpecOutput{Sections: []SpecSection{{Name: SecDrawings, Content: "图1为本实用新型结构示意图。"}}},
			SpecInput{PatentType: PatentTypeUtilityModel, HasDrawings: true}, false},
		{"新型无图", &SpecOutput{}, SpecInput{PatentType: PatentTypeUtilityModel, HasDrawings: false}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(tt.spec, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestUtilityProductOnlyRule(t *testing.T) {
	rule := &utilityProductOnlyRule{baseRule: newBaseRule("upo", "", "")}
	tests := []struct {
		name    string
		input   SpecInput
		wantErr bool
	}{
		{"发明不触发", SpecInput{PatentType: PatentTypeInvention}, false},
		{"新型无方法", SpecInput{PatentType: PatentTypeUtilityModel, Features: []SpecFeature{{Category: "structure"}}}, false},
		{"新型含方法特征", SpecInput{PatentType: PatentTypeUtilityModel, Features: []SpecFeature{{Category: "method"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(nil, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

func TestUtilitySingleIndependentRule(t *testing.T) {
	rule := &utilitySingleIndependentRule{baseRule: newBaseRule("usi", "", "")}
	tests := []struct {
		name    string
		input   SpecInput
		wantErr bool
	}{
		{"发明不触发", SpecInput{PatentType: PatentTypeInvention}, false},
		{"新型一个独权", SpecInput{PatentType: PatentTypeUtilityModel, Claims: []string{"一种挖掘机，其特征在于，包括壳体。"}}, false},
		{"新型多个独权", SpecInput{PatentType: PatentTypeUtilityModel, Claims: []string{
			"一种挖掘机，其特征在于，包括壳体。",
			"一种挖掘机控制方法，其特征在于，包括以下步骤。",
		}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rule.Check(nil, tt.input)) > 0
			if got != tt.wantErr {
				t.Errorf("gotErr=%v, wantErr=%v", got, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// 引擎集成测试
// =============================================================================

func TestRuleEngine_Validate(t *testing.T) {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	rules := engine.Rules()
	if len(rules) == 0 {
		t.Error("规则列表不应为空")
	}
	t.Logf("规则总数: %d", len(rules))

	spec := &SpecOutput{
		Title:    "一种挖掘机悬臂装置",
		Abstract: "本发明涉及工程机械技术领域，解决了效率低下的问题。",
		Sections: []SpecSection{
			{Name: SecTechField, Content: "本发明涉及工程机械技术领域。"},
			{Name: SecBackground, Content: "现有技术中，挖掘机悬臂存在效率低下的问题。"},
			{Name: SecContent, Content: "要解决的技术问题是提高挖掘效率。技术方案如下：包括悬臂主体和液压油缸。有益效果是提高了工作效率。"},
			{Name: SecDrawings, Content: "图1为本发明的结构示意图。"},
			{Name: SecEmbodiment, Content: "下面结合附图详细说明。实施例1：包括悬臂主体和液压油缸。"},
		},
	}
	violations := engine.Validate(spec, SpecInput{PatentType: PatentTypeInvention, TechDomain: DomainMechanical})
	t.Logf("违规数: %d", len(violations))
	if len(violations) > 5 {
		t.Errorf("预期少量违规，got=%d", len(violations))
	}
}

func TestSpecScorer_Score(t *testing.T) {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	scorer := NewSpecScorer(engine)
	spec := &SpecOutput{
		Title: "一种挖掘机悬臂装置",
		Sections: []SpecSection{
			{Name: SecTechField, Content: "本发明涉及工程机械技术领域。"},
			{Name: SecBackground, Content: "现有技术存在效率低下的问题。"},
			{Name: SecContent, Content: "要解决的技术问题是提高效率。技术方案如下：包括悬臂主体。有益效果是提高了效率。"},
			{Name: SecDrawings, Content: "图1为本发明的结构示意图。"},
			{Name: SecEmbodiment, Content: "实施例1：包括悬臂主体和液压油缸，一端与机身连接。"},
		},
	}
	report := scorer.Score(spec, SpecInput{PatentType: PatentTypeInvention, TechDomain: DomainMechanical})
	if report.OverallScore < 0 || report.OverallScore > 100 {
		t.Errorf("score out of range: %v", report.OverallScore)
	}
	t.Logf("评分: %.1f, 等级: %s", report.OverallScore, report.Grade)
}

func genLongChinese(n int) string {
	chars := []rune("本发明提供了一种挖掘机悬臂装置涉及工程机械技术领域。")
	r := make([]rune, n)
	for i := 0; i < n; i++ {
		r[i] = chars[i%len(chars)]
	}
	return string(r)
}
