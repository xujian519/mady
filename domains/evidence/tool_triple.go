package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

// TripleToolName is the agent-visible name of the judge_triple tool.
const TripleToolName = "judge_triple"

const tripleToolDesc = `对单条证据进行三性审查（关联性、合法性、真实性），返回逐项评分和综合判断。

适用场景：
- 评估现有技术文献是否可以作为无效宣告证据
- 判断互联网公开内容的法律效力
- 核实域外证据的可采性

返回逐项评分（0-1）及推理说明，综合评分低于 0.5 的证据不建议使用。`

type tripleTool struct {
	engine *DefaultEngine
}

func newTripleTool(engine *DefaultEngine) *agentcore.Tool {
	t := &tripleTool{engine: engine}
	return &agentcore.Tool{
		Name:        TripleToolName,
		Description: tripleToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source_uri": map[string]any{
					"type":        "string",
					"description": "证据来源 URI，如 patent:CN12345678A 或 https://...",
				},
				"snippet": map[string]any{
					"type":        "string",
					"description": "证据原文摘录",
				},
			},
			"required":             []string{"snippet"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type tripleArgs struct {
	SourceURI string `json:"source_uri"`
	Snippet   string `json:"snippet"`
}

func (t *tripleTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p tripleArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Snippet == "" {
		return nil, fmt.Errorf("缺少必填字段 'snippet'")
	}

	span := agentcore_evidence.EvidenceSpan{
		ID:        fmt.Sprintf("triple_%d", time.Now().UnixNano()),
		SourceURI: p.SourceURI,
		Snippet:   p.Snippet,
	}

	judgment, err := t.engine.Judge(span)
	if err != nil {
		return nil, fmt.Errorf("证据判断失败: %w", err)
	}

	return judgmentToMap(judgment), nil
}

// judgmentToMap 将 EvidenceJudgment 转换为便于 LLM 消费的 map。
func judgmentToMap(j *EvidenceJudgment) map[string]any {
	m := map[string]any{
		"overall_score": j.OverallScore,
		"confidence":    j.Confidence,
		"reasoning":     j.Reasoning,
	}

	if j.RelevanceJudgment != nil {
		m["relevance"] = map[string]any{
			"score":     j.RelevanceJudgment.Score,
			"level":     j.RelevanceJudgment.Level,
			"reasoning": j.RelevanceJudgment.Reasoning,
		}
	}
	if j.LegalityJudgment != nil {
		m["legality"] = map[string]any{
			"score":     j.LegalityJudgment.Score,
			"level":     j.LegalityJudgment.Level,
			"reasoning": j.LegalityJudgment.Reasoning,
		}
	}
	if j.AuthenticityJudgment != nil {
		m["authenticity"] = map[string]any{
			"score":     j.AuthenticityJudgment.Score,
			"level":     j.AuthenticityJudgment.Level,
			"reasoning": j.AuthenticityJudgment.Reasoning,
		}
	}

	issues := make([]map[string]string, 0, len(j.FlaggedIssues))
	for _, issue := range j.FlaggedIssues {
		issues = append(issues, map[string]string{
			"type":        issue.Type,
			"description": issue.Description,
			"severity":    issue.Severity,
		})
	}
	if len(issues) > 0 {
		m["issues"] = issues
	}
	return m
}
