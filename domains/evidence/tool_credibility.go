package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// CredibilityToolName is the agent-visible name of the assess_credibility tool.
//
//nolint:gosec // 工具名称和描述不是硬编码凭据
const CredibilityToolName = "assess_credibility"

//nolint:gosec // 工具描述不是硬编码凭据
const credibilityToolDesc = `综合评估电子证据/互联网公开证据的平台可信度。

根据来源 URI 判定平台类型（政府官网/学术数据库/新闻媒体/社交平台等），
输出平台可信度等级和 0-1 分数；当提供内容哈希或验证副本时，综合评估可靠性。`

type credibilityTool struct{}

func newCredibilityTool() *agentcore.Tool {
	t := &credibilityTool{}
	return &agentcore.Tool{
		Name:        CredibilityToolName,
		Description: credibilityToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"uri": map[string]any{
					"type":        "string",
					"description": "证据来源 URI（如 https://www.gov.cn/xxx.pdf）",
				},
				"content_hash": map[string]any{
					"type":        "string",
					"description": "内容摘要哈希值（可选），用于完整性验证",
				},
				"is_verified_copy": map[string]any{
					"type":        "boolean",
					"description": "是否为经过公证或认证的副本（可选）",
				},
			},
			"required":             []string{"uri"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type credibilityArgs struct {
	URI            string `json:"uri"`
	ContentHash    string `json:"content_hash,omitempty"`
	IsVerifiedCopy bool   `json:"is_verified_copy,omitempty"`
}

func (t *credibilityTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p credibilityArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.URI == "" {
		return nil, fmt.Errorf("缺少必填字段 'uri'")
	}

	platformCred := PlatformCredibility(p.URI)
	platformScore := CredibilityToScore(platformCred)
	combined := AssessElectronicEvidence(platformCred, p.ContentHash, p.IsVerifiedCopy)
	combinedScore := CredibilityToScore(combined)

	result := map[string]any{
		"uri":              p.URI,
		"platform_level":   string(platformCred),
		"platform_score":   platformScore,
		"combined_level":   string(combined),
		"combined_score":   combinedScore,
		"has_content_hash": p.ContentHash != "",
		"is_verified_copy": p.IsVerifiedCopy,
	}
	return result, nil
}
