package workercontract

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
)

// NewPatentTeamResolveTool 创建 patent_team_resolve 工具：按场景解析
// 立场配对的角色编排，输出各角色的 persona 段落与装配顺序，供多角色
// 对抗评审/团队协作场景装配子代理使用。
func NewPatentTeamResolveTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name: "patent_team_resolve",
		Description: "按场景解析专利团队的角色编排（立场配对 + 中立裁判）。" +
			"返回有序角色清单：每角色的 persona 段落（职责/越界禁止/HITL 标记）与立场。" +
			"对抗性场景（撰写对立评审/无效/OA 答复/侵权比对）应按此装配多角色评审，裁判角色禁止参与任一方策略起草。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenario": map[string]any{
					"type":        "string",
					"enum":        TeamScenarios(),
					"description": "场景（与专利工作流 case_type 对应）",
				},
			},
			"required": []string{"scenario"},
		},
		ReadOnly: true,
		Func:     handleTeamResolve,
	}
}

func handleTeamResolve(_ context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Scenario string `json:"scenario"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return agentcore.NewFailureResult("参数解析失败", "patent_team_resolve 参数格式错误"), nil
	}
	if p.Scenario == "" {
		return agentcore.NewFailureResult("缺少参数", "scenario 不能为空"), nil
	}

	team, err := ResolveTeamComposition(p.Scenario)
	if err != nil {
		return agentcore.NewFailureResult("解析失败", err.Error()), nil
	}

	roles := make([]map[string]any, 0, len(team.Roles))
	for _, r := range team.Roles {
		roles = append(roles, map[string]any{
			"id":                r.ID,
			"title":             r.Title,
			"stance":            string(r.Stance),
			"strategy_drafting": r.StrategyDrafting,
			"hitl":              r.HITL,
			"persona":           r.PersonaSegment(),
		})
	}

	out := map[string]any{
		"ok":       true,
		"scenario": team.Scenario,
		"roles":    roles,
		"note":     "裁判角色禁止参与任一方策略起草；HITL 角色的产出须经人工确认后才能进入交付物",
	}
	data, err := json.Marshal(out)
	if err != nil {
		return agentcore.NewFailureResult("序列化失败", err.Error()), nil
	}
	return string(data), nil
}
