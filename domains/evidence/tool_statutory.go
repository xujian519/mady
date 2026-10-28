package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

const statutoryToolDesc = `按《民事诉讼法》第 66 条的八类法定证据，对单条证据做二维三性评估。

两个维度彼此独立：
- kind（法定种类）：书证、物证、视听资料、电子数据、证人证言、当事人陈述、鉴定意见、勘验笔录。
- acquisition_context（取证场景）：复用既有 15 类证据类型，如 internet_publication（互联网公开）、
  public_use（使用公开）、overseas（域外取得）、foreign_language（外文形成）、notarial_certificate（公证）。

返回关联性、合法性、真实性三项评分（0-1）与综合分，以及逐条规则检查结果（findings）。
未声明的事实按未通过记录但不影响分数；要得到完整结论，请尽量填写相关字段，
例如评估书证时给出 has_original、signature_verified、formation_time_clear。

结论以已声明事实为限，评分用于辅助判断，不替代律师对证据的实质审查。`

type statutoryTool struct{}

func newStatutoryTool() *agentcore.Tool {
	t := &statutoryTool{}
	return &agentcore.Tool{
		Name:        StatutoryToolName,
		Description: statutoryToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"evidence_id": map[string]any{
					"type":        "string",
					"description": "证据标识，缺省时自动生成",
				},
				"kind": map[string]any{
					"type":        "string",
					"enum":        evidenceKindNames(),
					"description": "法定证据种类（民事诉讼法第 66 条）",
				},
				"acquisition_context": map[string]any{
					"type":        "string",
					"enum":        evidenceTypeNames(),
					"description": "取证场景，缺省为 general",
				},
				"fact_to_prove": map[string]any{
					"type":        "string",
					"description": "证据拟证明的事实",
				},
				"content": map[string]any{"type": "string", "description": "证据内容摘要或原文"},
				"source":  map[string]any{"type": "string", "description": "证据来源描述"},
				"has_original": map[string]any{
					"type":        "boolean",
					"description": "是否提供了原件/原物（书证、物证）",
				},
				"original_absence_reason": map[string]any{
					"type":        "string",
					"description": "未提供原件/原物的原因说明",
				},
				"signature_verified": map[string]any{
					"type":        "boolean",
					"description": "书证签章是否已核验",
				},
				"formation_time_clear": map[string]any{
					"type":        "boolean",
					"description": "书证形成时间是否明确",
				},
				"content_hash": map[string]any{
					"type":        "string",
					"description": "内容哈希（电子数据、视听资料）",
				},
				"hash_verified": map[string]any{
					"type":        "boolean",
					"description": "内容哈希是否已独立核验",
				},
				"timestamp": map[string]any{
					"type":        "string",
					"description": "时间戳或时间信息（电子数据、视听资料）",
				},
				"storage_integrity_verified": map[string]any{
					"type":        "boolean",
					"description": "存储完整性/保管链是否已核验",
				},
				"witness_has_conflict": map[string]any{
					"type":        "boolean",
					"description": "证人与案件是否有利害关系",
				},
				"conflict_disclosed": map[string]any{
					"type":        "boolean",
					"description": "利害关系是否已披露",
				},
				"corroborated": map[string]any{
					"type":        "boolean",
					"description": "是否有其他证据佐证（证人证言、当事人陈述）",
				},
				"expert_qualified": map[string]any{
					"type":        "boolean",
					"description": "鉴定机构/鉴定人资质是否已核验",
				},
				"expert_procedure_legal": map[string]any{
					"type":        "boolean",
					"description": "鉴定程序是否合法",
				},
				"expert_conclusion_clear": map[string]any{
					"type":        "boolean",
					"description": "鉴定结论是否明确",
				},
				"inspection_has_witness": map[string]any{
					"type":        "boolean",
					"description": "勘验是否有见证人在场",
				},
				"inspection_record_complete": map[string]any{
					"type":        "boolean",
					"description": "勘验记录是否完整",
				},
				"collection_legal": map[string]any{
					"type":        "boolean",
					"description": "证据收集是否合法",
				},
				"form_legal": map[string]any{
					"type":        "boolean",
					"description": "证据形式是否合法",
				},
				"notarized": map[string]any{
					"type":        "boolean",
					"description": "是否已办理公证/公证认证（域外取得、公证场景）",
				},
				"translated": map[string]any{
					"type":        "boolean",
					"description": "外文证据是否附有中文译本",
				},
				"public_date_clear": map[string]any{
					"type":        "boolean",
					"description": "互联网公开时间是否明确",
				},
				"publicly_accessible": map[string]any{
					"type":        "boolean",
					"description": "互联网内容是否处于公众可获取状态",
				},
				"public_use_four_elements": map[string]any{
					"type":        "boolean",
					"description": "使用公开的四要件（时间/地点/方式/公众可获取）是否齐备",
				},
			},
			"required":             []string{"kind"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

func (t *statutoryTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var in StatutoryEvidenceInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if in.EvidenceID == "" {
		in.EvidenceID = fmt.Sprintf("legal_evi_%d", time.Now().UnixNano())
	}
	assessment, err := AssessStatutoryEvidence(in)
	if err != nil {
		return nil, err
	}
	return assessment, nil
}
