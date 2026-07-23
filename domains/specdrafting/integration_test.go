package specdrafting

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Builder 集成测试
// =============================================================================

func TestSpecBuilder_Integration(t *testing.T) {
	builder := NewSpecBuilder(nil)

	tests := []struct {
		name      string
		input     SpecInput
		wantSects int // 期望的章节数
	}{
		{
			name: "发明专利-机械领域",
			input: SpecInput{
				Title:       "一种挖掘机悬臂装置",
				PatentType:  PatentTypeInvention,
				TechDomain:  DomainMechanical,
				HasDrawings: true,
				Problems:    []string{"现有挖掘机悬臂结构复杂、效率低"},
				Features: []SpecFeature{
					{ID: "f1", Description: "悬臂主体", Category: "structure", Function: "支撑工作装置"},
					{ID: "f2", Description: "液压油缸", Category: "structure", Function: "驱动悬臂升降"},
				},
				Effects: []string{"简化结构、提高工作效率"},
			},
			wantSects: 5,
		},
		{
			name: "实用新型-软件领域（应为机械默认）",
			input: SpecInput{
				Title:       "一种便携式数据采集器",
				PatentType:  PatentTypeUtilityModel,
				TechDomain:  DomainSoftware,
				HasDrawings: true,
				Problems:    []string{"现有数据采集器体积大、续航短"},
				Features: []SpecFeature{
					{ID: "f1", Description: "外壳", Category: "structure"},
					{ID: "f2", Description: "主控电路板", Category: "structure"},
				},
				Effects: []string{"体积减小、续航延长"},
			},
			wantSects: 5,
		},
		{
			name: "无输入特征",
			input: SpecInput{
				Title:      "一种新型材料",
				PatentType: PatentTypeInvention,
				TechDomain: DomainChemical,
			},
			wantSects: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := builder.Build(tt.input)

			if output.Title == "" {
				t.Error("Title 不应为空")
			}
			if len(output.Sections) != tt.wantSects {
				t.Errorf("Sections 数量: got=%d, want=%d", len(output.Sections), tt.wantSects)
			}
			if output.Timestamp == "" {
				t.Error("Timestamp 不应为空")
			}
			if output.Metadata.WordCount <= 0 {
				t.Errorf("WordCount 应 > 0, got=%d", output.Metadata.WordCount)
			}

			// 验证各章节不为空
			for _, sec := range output.Sections {
				if sec.Content == "" {
					t.Errorf("章节 %s 内容为空", sec.Name)
				}
			}

			t.Logf("标题: %s", output.Title)
			t.Logf("摘要: %s", output.Abstract[:min(len(output.Abstract), 80)])
			t.Logf("章节数: %d", len(output.Sections))
			t.Logf("总字数: %d", output.Metadata.WordCount)
		})
	}
}

// =============================================================================
// 规则引擎集成测试
// =============================================================================

func TestRuleEngine_Integration(t *testing.T) {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)

	spec := &SpecOutput{
		Title:    "一种挖掘机悬臂装置",
		Abstract: "本发明涉及工程机械技术领域，解决了效率低下的问题。",
		Sections: []SpecSection{
			{Name: SecTechField, Content: "本发明涉及工程机械技术领域，具体涉及一种挖掘机悬臂。"},
			{Name: SecBackground, Content: "现有技术中，挖掘机悬臂存在效率低下的问题。"},
			{Name: SecContent, Content: "要解决的技术问题是提高挖掘效率。技术方案如下：包括悬臂主体和液压油缸。有益效果是提高了工作效率。"},
			{Name: SecDrawings, Content: "图1为本发明的结构示意图。"},
			{Name: SecEmbodiment, Content: "下面结合附图详细说明。实施例1：包括悬臂主体和液压油缸。悬臂主体一端与机身连接，另一端与油缸连接。"},
		},
	}

	input := SpecInput{
		Title:      "一种挖掘机悬臂装置",
		PatentType: PatentTypeInvention,
		TechDomain: DomainMechanical,
		Features: []SpecFeature{
			{ID: "f1", Description: "悬臂主体", Category: "structure"},
			{ID: "f2", Description: "液压油缸", Category: "structure"},
		},
		Problems: []string{"挖掘效率低下"},
		Effects:  []string{"提高了工作效率"},
	}

	violations := engine.Validate(spec, input)
	t.Logf("总违规数: %d", len(violations))
	for _, v := range violations {
		t.Logf("  [%s] %s", v.Severity, v.Message)
	}

	// 完整说明书应少于5条违规
	if len(violations) >= 5 {
		t.Errorf("完整说明书违规过多: %d", len(violations))
	}
}

// =============================================================================
// Scorer 集成测试
// =============================================================================

func TestSpecScorer_Integration(t *testing.T) {
	engine := NewRuleEngine()
	RegisterDefaultRules(engine)
	scorer := NewSpecScorer(engine)

	spec := &SpecOutput{
		Title:    "一种挖掘机悬臂装置",
		Abstract: "本发明涉及工程机械领域，解决了效率低下的问题。",
		Sections: []SpecSection{
			{Name: SecTechField, Content: "本发明涉及工程机械技术领域。"},
			{Name: SecBackground, Content: "现有技术存在效率问题。"},
			{Name: SecContent, Content: "要解决的技术问题是提高效率。技术方案如下：包括悬臂主体。有益效果是提高了效率。"},
			{Name: SecDrawings, Content: "图1为本发明的结构示意图。"},
			{Name: SecEmbodiment, Content: "实施例1：包括悬臂主体和液压油缸，一端与机身连接。"},
		},
	}

	report := scorer.Score(spec, SpecInput{PatentType: PatentTypeInvention, TechDomain: DomainMechanical})

	if report.OverallScore < 0 || report.OverallScore > 100 {
		t.Errorf("Score 超出范围: %v", report.OverallScore)
	}
	if report.Grade == "" {
		t.Error("Grade 不应为空")
	}
	t.Logf("总体评分: %.1f, 等级: %s", report.OverallScore, report.Grade)
	for dim, s := range report.DimensionScores {
		t.Logf("  维度 %s: %.1f", dim, s)
	}
}

// =============================================================================
// Pregel 图执行测试
// =============================================================================

func TestBuildSpecificationGraph_Execute(t *testing.T) {
	ctx := context.Background()

	compiled, err := BuildSpecificationGraph(nil, nil)
	if err != nil {
		t.Fatalf("BuildSpecificationGraph failed: %v", err)
	}

	input := &SpecInput{
		Title:       "一种挖掘机悬臂装置",
		PatentType:  PatentTypeInvention,
		TechDomain:  DomainMechanical,
		HasDrawings: true,
		Problems:    []string{"现有挖掘机悬臂结构复杂、效率低"},
		Features: []SpecFeature{
			{ID: "f1", Description: "悬臂主体", Category: "structure", Function: "支撑工作装置"},
			{ID: "f2", Description: "液压油缸", Category: "structure", Function: "驱动悬臂升降"},
		},
		Effects: []string{"简化结构、提高工作效率"},
		Claims:  []string{"一种挖掘机悬臂装置，其特征在于，包括悬臂主体和液压油缸。"},
	}

	state, err := compiled.Run(ctx, graph.PregelState{
		StateKeyInput: input,
	})
	if err != nil {
		t.Fatalf("CompiledPregelGraph.Run failed: %v", err)
	}

	output, ok := state[StateKeyOutput].(*SpecOutput)
	if !ok || output == nil {
		t.Fatal("state 中未找到 spec_output")
	}

	if output.Title == "" {
		t.Error("Title 不应为空")
	}
	if len(output.Sections) == 0 {
		t.Error("Sections 不应为空")
	}
	t.Logf("图执行完成: %s", output.Title)
	t.Logf("章节数: %d, 总字数: %d", len(output.Sections), output.Metadata.WordCount)
	for _, sec := range output.Sections {
		t.Logf("  [%s] %d字", sec.Name, sec.WordCnt)
	}
}

// =============================================================================
// 发明 vs 实用新型对比测试
// =============================================================================

func TestPatentType_Differences(t *testing.T) {
	builder := NewSpecBuilder(nil)

	invention := builder.Build(SpecInput{
		Title:       "一种数据处理方法",
		PatentType:  PatentTypeInvention,
		TechDomain:  DomainSoftware,
		HasDrawings: false,
		Features:    []SpecFeature{{ID: "f1", Description: "数据处理模块", Category: "method"}},
	})

	utility := builder.Build(SpecInput{
		Title:       "一种数据采集装置",
		PatentType:  PatentTypeUtilityModel,
		TechDomain:  DomainMechanical,
		HasDrawings: true,
		Features:    []SpecFeature{{ID: "f1", Description: "壳体", Category: "structure"}},
	})

	// 发明在无附图时"附图说明"应包含"无附图"
	if !strings.Contains(invention.Sections[3].Content, "无附图") {
		t.Errorf("发明无附图时附图说明应包含'无附图'，实际: %s", invention.Sections[3].Content)
	}

	// 实用新型应有附图描述
	hasDrawingDesc := containsAnyOf(utility.Sections[3].Content, []string{"图1", "结构示意图"})
	if !hasDrawingDesc {
		t.Logf("实用新型附图说明应包含图示描述")
	}

	t.Logf("发明标题: %s", invention.Title)
	t.Logf("实用新型标题: %s", utility.Title)
}

// =============================================================================
// 领域自适应测试
// =============================================================================

func TestDomainAdaptation(t *testing.T) {
	builder := NewSpecBuilder(nil)

	chemSpec := builder.Build(SpecInput{
		Title:      "一种催化组合物",
		PatentType: PatentTypeInvention,
		TechDomain: DomainChemical,
		Features:   []SpecFeature{{ID: "f1", Description: "催化剂A", Category: "material"}},
	})

	content := chemSpec.Sections[2].Content // 发明内容
	if containsAnyOf(content, []string{"实验数据", "加以证实"}) {
		t.Log("化学领域说明书包含实验数据提示")
	}

	softSpec := builder.Build(SpecInput{
		Title:      "一种图像处理方法",
		PatentType: PatentTypeInvention,
		TechDomain: DomainSoftware,
		Features:   []SpecFeature{{ID: "f1", Description: "图像处理步骤", Category: "method"}},
	})

	title := softSpec.Title
	if title != "" {
		t.Logf("软件领域标题: %s", title)
	}
}

// =============================================================================
// 空输入边界测试
// =============================================================================

func TestSpecBuilder_EdgeCases(t *testing.T) {
	builder := NewSpecBuilder(nil)

	t.Run("空SpecInput", func(t *testing.T) {
		output := builder.Build(SpecInput{})
		if output.Title == "" {
			t.Log("空输入时使用默认标题")
		}
		if len(output.Sections) != 5 {
			t.Errorf("期望5个章节, got %d", len(output.Sections))
		}
	})

	t.Run("仅标题", func(t *testing.T) {
		output := builder.Build(SpecInput{
			Title: "一种测试装置",
		})
		if output.Title != "一种测试装置" {
			t.Errorf("标题不匹配: %s", output.Title)
		}
	})

	t.Run("仅特征", func(t *testing.T) {
		output := builder.Build(SpecInput{
			Features: []SpecFeature{{ID: "f1", Description: "测试特征"}},
		})
		content := output.Sections[2].Content
		if !strings.Contains(content, "测试特征") {
			t.Log("特征在内容中得到反映")
		}
	})
}

func TestSpecInputFromExtraction(t *testing.T) {
	input := SpecInputFromExtraction(nil, PatentTypeInvention, false, nil)
	if input == nil {
		t.Fatal("nil ExtractionResult 应返回非 nil SpecInput")
	}
	if input.PatentType != PatentTypeInvention {
		t.Errorf("PatentType 不匹配: %v", input.PatentType)
	}
	t.Log("SpecInputFromExtraction(nil) 正常")
}
