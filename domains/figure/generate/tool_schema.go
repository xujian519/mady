package generate

func stepsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "流程图步骤（kind=flowchart）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
				"shape": map[string]any{"type": "string", "enum": []string{"box", "ellipse", "diamond", "parallelogram", "cylinder"}},
				"next": map[string]any{
					"type":        "array",
					"description": "后继：字符串 id，或 {\"id\":…,\"label\":…}（决策分支必须带标签）",
					"items":       map[string]any{"type": []string{"string", "object"}},
				},
			},
			"required": []string{"id", "label"},
		},
	}
}

func blocksSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "框图块（kind=block）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
				"type":  map[string]any{"type": "string", "enum": []string{"input", "output", "process", "storage", "decision", "default"}},
			},
			"required": []string{"id", "label"},
		},
	}
}

func connectionsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "框图连接（kind=block）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from":  map[string]any{"type": "string"},
				"to":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
			},
			"required": []string{"from", "to"},
		},
	}
}

func nodesSchema() map[string]any {
	node := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":    map[string]any{"type": "string"},
			"label": map[string]any{"type": "string"},
		},
		"required": []string{"id", "label"},
	}
	node["properties"].(map[string]any)["children"] = map[string]any{
		"type":        "array",
		"description": "子节点（任意深度）",
		"items":       node,
	}
	return map[string]any{
		"type":        "array",
		"description": "层级图根节点（kind=hierarchy）",
		"items":       node,
	}
}

func statesSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "状态（kind=state）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
				"kind":  map[string]any{"type": "string", "enum": []string{"normal", "initial", "final"}},
			},
			"required": []string{"id", "label"},
		},
	}
}

func transitionsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "状态转移（kind=state）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from":  map[string]any{"type": "string"},
				"to":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
			},
			"required": []string{"from", "to"},
		},
	}
}

func referencesSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "渲染后追加的图外标号（仅 svg）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label":   map[string]any{"type": "string", "description": "匹配图内文本（子串，大小写不敏感）"},
				"numeral": map[string]any{"type": "string", "description": "追加的标号"},
			},
			"required": []string{"label", "numeral"},
		},
	}
}

func pointSchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "number"},
		"minItems":    2,
		"maxItems":    2,
	}
}

func participantsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "时序图参与者（kind=sequence）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
			},
			"required": []string{"id", "label"},
		},
	}
}

func messagesSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "时序图消息（kind=sequence）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from":     map[string]any{"type": "string"},
				"to":       map[string]any{"type": "string"},
				"label":    map[string]any{"type": "string"},
				"kind":     map[string]any{"type": "string", "enum": []string{"sync", "return", "async"}},
				"activate": map[string]any{"type": "boolean", "description": "是否在目标生命线上画激活条"},
			},
			"required": []string{"from", "to", "label"},
		},
	}
}

func sequenceOptionsSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "时序图尺寸选项（毫米）",
		"properties": map[string]any{
			"box_width_mm":       map[string]any{"type": "number"},
			"message_spacing_mm": map[string]any{"type": "number"},
			"padding_mm":         map[string]any{"type": "number"},
		},
		"additionalProperties": false,
	}
}

func sectionSchema() map[string]any {
	part := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"label":   map[string]any{"type": "string", "description": "零件标号；落在轮廓右上方并画引线"},
			"outline": map[string]any{"type": "array", "items": pointSchema("顶点 [x, y]"), "description": "闭合轮廓（至少 3 个顶点）"},
			"holes":   map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": pointSchema("顶点 [x, y]")}, "description": "孔环；须同时给 hatch_segments"},
			"hatch_segments": map[string]any{
				"type":        "array",
				"description": "已算好的剖面线段",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"from": pointSchema("起点 [x, y]"),
						"to":   pointSchema("终点 [x, y]"),
					},
					"required": []string{"from", "to"},
				},
			},
			"hatch": map[string]any{
				"description": `剖面线参数对象，或 "none" 表示只画轮廓`,
			},
			"stroke_width_mm": map[string]any{"type": "number"},
		},
		"required": []string{"outline"},
	}
	return map[string]any{
		"type":        "object",
		"description": "剖视图输入（kind=section）",
		"properties": map[string]any{
			"parts": map[string]any{"type": "array", "items": part, "description": "被剖切零件（至少一个）"},
			"labels": map[string]any{
				"type":        "array",
				"description": "显式引线标号",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
						"at":   pointSchema("数字视觉中心 [x, y]"),
						"from": pointSchema("引线起点 [x, y]（可选）"),
					},
					"required": []string{"text", "at"},
				},
			},
			"centerlines": map[string]any{
				"type":        "array",
				"description": "中心线（细点划线）",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"from": pointSchema("起点 [x, y]"),
						"to":   pointSchema("终点 [x, y]"),
					},
					"required": []string{"from", "to"},
				},
			},
			"options": map[string]any{
				"type":        "object",
				"description": "尺寸与线宽选项（毫米）",
				"properties": map[string]any{
					"font_size_mm":         map[string]any{"type": "number"},
					"stroke_width_mm":      map[string]any{"type": "number"},
					"thin_stroke_width_mm": map[string]any{"type": "number"},
					"padding_mm":           map[string]any{"type": "number"},
				},
				"additionalProperties": false,
			},
		},
		"required": []string{"parts"},
	}
}

func templateNames() []string {
	out := make([]string, 0, len(AllTemplates()))
	for _, name := range AllTemplates() {
		out = append(out, string(name))
	}
	return out
}
