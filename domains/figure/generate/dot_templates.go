package generate

// TemplateName 是内置模板名。
type TemplateName string

// Template constants.
const (
	TemplateSimpleFlowchart    TemplateName = "simple_flowchart"
	TemplateSystemBlock        TemplateName = "system_block"
	TemplateMethodSteps        TemplateName = "method_steps"
	TemplateComponentHierarchy TemplateName = "component_hierarchy"
)

// AllTemplates 返回全部内置模板名。
func AllTemplates() []TemplateName {
	return []TemplateName{
		TemplateSimpleFlowchart, TemplateSystemBlock,
		TemplateMethodSteps, TemplateComponentHierarchy,
	}
}

// Template 构建内置模板 DOT。
func Template(name TemplateName, opts Options) (string, error) {
	switch name {
	case TemplateSimpleFlowchart:
		return BuildFlowchart([]Step{
			{ID: "start", Label: "开始", Shape: "ellipse", Next: []Next{{ID: "step1"}}},
			{ID: "step1", Label: "初始化", Next: []Next{{ID: "decision"}}},
			{ID: "decision", Label: "成功？", Shape: "diamond", Next: []Next{
				{ID: "step3", Label: "是"}, {ID: "step1", Label: "否"},
			}},
			{ID: "step3", Label: "处理", Next: []Next{{ID: "end"}}},
			{ID: "end", Label: "结束", Shape: "ellipse"},
		}, opts)
	case TemplateSystemBlock:
		return BuildBlockDiagram([]Block{
			{ID: "input", Label: "输入设备", Type: "input"},
			{ID: "controller", Label: "控制器", Type: "process"},
			{ID: "processor", Label: "处理模块", Type: "process"},
			{ID: "memory", Label: "存储", Type: "storage"},
			{ID: "output", Label: "输出设备", Type: "output"},
		}, []Connection{
			{From: "input", To: "controller", Label: "信号"},
			{From: "controller", To: "processor", Label: "指令"},
			{From: "processor", To: "memory", Label: "数据"},
			{From: "memory", To: "processor", Label: "数据"},
			{From: "processor", To: "output", Label: "结果"},
		}, opts)
	case TemplateMethodSteps:
		fixed := Options{
			FigureNumber:  opts.FigureNumber,
			Style:         opts.Style,
			FontName:      opts.FontName,
			FontPt:        opts.FontPt,
			NumeralStep:   opts.NumeralStep,
			Page:          opts.Page,
			EmbedNumerals: opts.EmbedNumerals,
			Numerals: map[string]string{
				"step101": "101", "step102": "102", "step103": "103",
				"step104": "104", "step105": "105",
			},
		}
		return BuildFlowchart([]Step{
			{ID: "step101", Label: "接收输入数据", Next: []Next{{ID: "step102"}}},
			{ID: "step102", Label: "校验数据格式", Next: []Next{{ID: "step103"}}},
			{ID: "step103", Label: "处理数据", Next: []Next{{ID: "step104"}}},
			{ID: "step104", Label: "生成输出", Next: []Next{{ID: "step105"}}},
			{ID: "step105", Label: "发送结果"},
		}, fixed)
	case TemplateComponentHierarchy:
		return BuildHierarchy([]HierarchyNode{
			{
				ID: "system", Label: "系统",
				Children: []HierarchyNode{
					{ID: "sub1", Label: "子系统", Children: []HierarchyNode{
						{ID: "c1", Label: "组件A"}, {ID: "c2", Label: "组件B"},
					}},
					{ID: "sub2", Label: "子系统"},
				},
			},
		}, opts)
	default:
		return "", newError(ErrInvalidTemplate, "未知模板：%s", name)
	}
}
