package doctmpl

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
)

// NewListDocTemplatesTool creates the list_doc_templates agent tool.
// It exposes the template catalog so the agent can browse available
// templates by category and domain.
func NewListDocTemplatesTool(store *TemplateStore) *agentcore.Tool {
	return &agentcore.Tool{
		Name:     "list_doc_templates",
		ReadOnly: true,
		Description: "列出可用的文档模板，支持按 category（claims/specification/oa-response/disclosure/legal）" +
			"和 domain（patent/legal）筛选。返回模板名、标题、描述、支持格式。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category": map[string]any{
					"type":        "string",
					"description": "可选，按类别筛选（claims/specification/oa-response/disclosure/legal）",
				},
				"domain": map[string]any{
					"type":        "string",
					"description": "可选，按领域筛选（patent/legal）",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "可选，按语言筛选（zh-CN/en-US）",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "可选，按模板名/标题/描述模糊搜索",
				},
			},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Category string `json:"category"`
				Domain   string `json:"domain"`
				Language string `json:"language"`
				Query    string `json:"query"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return agentcore.NewFailureResult("参数错误", "无法解析 list_doc_templates 参数"), nil
				}
			}

			templates := store.List(ListOptions{
				Category: p.Category,
				Domain:   p.Domain,
				Language: p.Language,
				Query:    p.Query,
			})

			if len(templates) == 0 {
				return agentcore.NewHandoffResult(
					"list_doc_templates",
					"没有匹配的模板。可用类别: claims, specification, oa-response, disclosure, legal。",
				), nil
			}

			summaries := store.toSummaries(templates)
			result := listResult{
				Templates: summaries,
				Count:     len(summaries),
			}
			return agentcore.NewHandoffResult(
				"list_doc_templates",
				toJSON(result),
			), nil
		},
	}
}

// NewRenderDocTemplateTool creates the render_doc_template agent tool.
// It resolves template variables and renders the document in the
// requested output format.
func NewRenderDocTemplateTool(store *TemplateStore) *agentcore.Tool {
	return &agentcore.Tool{
		Name:     "render_doc_template",
		ReadOnly: false, // may write to file
		Description: "使用文档模板生成最终文档。指定模板名（从 list_doc_templates 获取）、" +
			"变量键值对、输出格式（markdown/docx/pdf/html/email，默认 markdown），" +
			"输出文件名（可选，不含扩展名）。返回渲染后的文档内容。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"template_name": map[string]any{
					"type":        "string",
					"description": "模板名（从 list_doc_templates 获取）",
				},
				"variables": map[string]any{
					"type":        "object",
					"description": "模板变量键值对，key 不含 {{}} 包裹",
				},
				"output_format": map[string]any{
					"type":        "string",
					"description": "输出格式: markdown/docx/pdf/html/email（默认 markdown）",
				},
				"output_filename": map[string]any{
					"type":        "string",
					"description": "输出文件名（不含扩展名，可选）",
				},
			},
			"required": []string{"template_name", "variables"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				TemplateName string            `json:"template_name"`
				Variables    map[string]string `json:"variables"`
				OutputFormat string            `json:"output_format"`
				OutputFile   string            `json:"output_filename"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数错误", "无法解析 render_doc_template 参数"), nil
			}
			if p.TemplateName == "" {
				return agentcore.NewFailureResult("参数缺失", "template_name 不能为空"), nil
			}

			// Default format.
			format := FormatMarkdown
			if p.OutputFormat != "" {
				f := OutputFormat(p.OutputFormat)
				if !f.IsValid() {
					return agentcore.NewFailureResult("格式不支持",
						"output_format 必须是 markdown/docx/pdf/html/email 之一"), nil
				}
				format = f
			}

			meta := RenderMeta{Filename: p.OutputFile}
			output, err := store.Render(p.TemplateName, p.Variables, format, meta)
			if err != nil {
				return agentcore.NewFailureResult("渲染失败", err.Error()), nil
			}

			result := renderResult{
				Template: p.TemplateName,
				Format:   string(format),
				Content:  string(output),
			}
			return agentcore.NewHandoffResult(
				"render_doc_template",
				toJSON(result),
			), nil
		},
	}
}
