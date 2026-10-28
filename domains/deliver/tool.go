package deliver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// ToolName 是工具名。
const ToolName = "deliver_register"

// artifactArg 是工具入参中的一件交付物。
type artifactArg struct {
	Role          string `json:"role"`
	CanonicalPath string `json:"canonical_path"`
	DeliveredPath string `json:"delivered_path"`
}

// requirementArg 是工具入参中的一项要求。
type requirementArg struct {
	Requirement string   `json:"requirement"`
	Evidence    []string `json:"evidence"`
}

// toolArgs 是登记工具入参。
type toolArgs struct {
	TemplateID   string           `json:"template_id"`
	Artifacts    []artifactArg    `json:"artifacts"`
	Figures      []string         `json:"figures"`
	Rendered     string           `json:"rendered"`
	Requirements []requirementArg `json:"requirements"`
	SelfCheck    string           `json:"self_check"`
}

// NewTool 构造交付件登记工具。
func NewTool(cfg Config) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        ToolName,
		Description: "登记本次交付的成品文件并跑确定性核验：主路径与交付副本是否一致、渲染件是否早于输入、要求是否有证据；HTML 渲染件另做合规检查，PDF 等非 HTML 交付件记入未执行项。artifacts 与 rendered 至少给一项；什么都没给会判失败。返回文件清单（路径/摘要/大小/时间）、结论（pass / fail / inconclusive）与核验明细，并与你的自述并列。",
		ReadOnly:    true,
		Parameters:  toolParameters(),
		Func: func(_ context.Context, raw json.RawMessage) (any, error) {
			var args toolArgs
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, fmt.Errorf("deliver: %s 参数解析失败: %w", ToolName, err)
			}
			return Register(toRegisterInput(args), cfg)
		},
	}
}

// toolParameters 返回工具参数 schema。
func toolParameters() map[string]any {
	// artifacts 与 rendered 至少给一项即可：只登记渲染件是合法交付，不应被 schema 拒之门外。
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"template_id": map[string]any{
				"type":        "string",
				"description": "本次交付使用的模板 id（如 claims-spec）；不来自模板时省略。",
			},
			"artifacts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"role", "canonical_path", "delivered_path"},
					"properties": map[string]any{
						"role":           map[string]any{"type": "string", "description": "文件角色，如「权利要求书」。"},
						"canonical_path": map[string]any{"type": "string", "description": "读者最先打开的案卷主路径。"},
						"delivered_path": map[string]any{"type": "string", "description": "本次交付发出的副本路径。"},
					},
				},
				"description": "待核对的交付物（主路径与交付副本）。",
			},
			"figures":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "本次交付的附图路径。"},
			"rendered": map[string]any{"type": "string", "description": "渲染出的交付件路径（html/pdf）；PDF 不做 HTML 合规检查，会在未执行项中说明。"},
			"requirements": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"requirement"},
					"properties": map[string]any{
						"requirement": map[string]any{"type": "string", "description": "指示方的原话要求。"},
						"evidence":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "支撑该要求的文件。"},
					},
				},
				"description": "本次交付必须交代的要求及证据。",
			},
			"self_check": map[string]any{
				"type":        "string",
				"description": "你对本次交付质量门的自述结论（将与确定性核验并列呈现）。",
			},
		},
	}
}

// toRegisterInput 把工具入参转成登记输入。
func toRegisterInput(args toolArgs) RegisterInput {
	input := RegisterInput{
		TemplateID: args.TemplateID,
		Figures:    args.Figures,
		Rendered:   args.Rendered,
		SelfCheck:  args.SelfCheck,
	}
	for _, artifact := range args.Artifacts {
		// 字段名、类型、顺序与 Artifact 严格一致，改动任一侧都会编译失败。
		input.Artifacts = append(input.Artifacts, Artifact(artifact))
	}
	for _, requirement := range args.Requirements {
		input.Requirements = append(input.Requirements, Requirement(requirement))
	}
	return input
}
