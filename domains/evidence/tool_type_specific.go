package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

const TypeSpecificToolName = "judge_type_specific"

const typeSpecificToolDesc = `根据证据类型进行专门检查，包括：
- internet_publication: 平台可信度、内容完整性、公开意图
- public_use: 四要件检查（时间/地点/方式/公众可获取性）
- foreign_language: 翻译状态
- overseas: 公证认证状态
- electronic: 平台可信度
- notarial_certificate: 公证状态`

type typeSpecificTool struct {
	engine *DefaultEngine
}

func newTypeSpecificTool(engine *DefaultEngine) *agentcore.Tool {
	t := &typeSpecificTool{engine: engine}
	return &agentcore.Tool{
		Name:        TypeSpecificToolName,
		Description: typeSpecificToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source_uri":         map[string]any{"type": "string", "description": "证据来源 URI"},
				"evidence_type_hint": map[string]any{"type": "string", "description": "手动指定证据类型（可选）"},
			},
			"required":             []string{"source_uri"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type typeSpecificArgs struct {
	SourceURI        string `json:"source_uri"`
	EvidenceTypeHint string `json:"evidence_type_hint"`
}

func (t *typeSpecificTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p typeSpecificArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.SourceURI == "" {
		return nil, fmt.Errorf("缺少必填字段 'source_uri'")
	}

	span := agentcore_evidence.EvidenceSpan{
		ID:        fmt.Sprintf("type_specific_%d", time.Now().UnixNano()),
		SourceURI: p.SourceURI,
	}

	judgment, err := t.engine.Judge(span)
	if err != nil {
		return nil, fmt.Errorf("证据判断失败: %w", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		return map[string]any{
			"evidence_type": string(inferEvidenceType(p.SourceURI)),
			"note":          "无类型特定判断结果",
		}, nil
	}

	ts := judgment.TypeSpecificJudgment
	m := map[string]any{
		"evidence_type":     string(ts.EvidenceType),
		"platform_category": ts.PlatformCategory,
	}
	if ts.PlatformCredibility != nil {
		m["platform_credibility"] = string(*ts.PlatformCredibility)
	}
	if ts.ContentIntegrity != "" {
		m["content_integrity"] = string(ts.ContentIntegrity)
	}
	if ts.PublicIntent != "" {
		m["public_intent"] = string(ts.PublicIntent)
	}
	if ts.TranslationStatus != "" {
		m["translation_status"] = ts.TranslationStatus
	}
	if ts.NotarizationStatus != "" {
		m["notarization_status"] = ts.NotarizationStatus
	}
	if ts.FourElementsCheck != nil {
		fec := ts.FourElementsCheck
		m["four_elements_check"] = map[string]any{
			"time":          map[string]any{"met": fec.TimeElement.Met, "score": fec.TimeElement.Score, "detail": fec.TimeElement.Detail},
			"place":         map[string]any{"met": fec.PlaceElement.Met, "score": fec.PlaceElement.Score, "detail": fec.PlaceElement.Detail},
			"method":        map[string]any{"met": fec.MethodElement.Met, "score": fec.MethodElement.Score, "detail": fec.MethodElement.Detail},
			"accessibility": map[string]any{"met": fec.Accessibility.Met, "score": fec.Accessibility.Score, "detail": fec.Accessibility.Detail},
		}
	}
	return m, nil
}
