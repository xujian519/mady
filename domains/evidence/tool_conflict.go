package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

const ConflictToolName = "detect_conflict"

const conflictToolDesc = `检测多条证据之间的冲突关系，包括方向冲突（同一主张同时有支持和反对证据）和来源冲突（同一来源内含矛盾内容）。`

type conflictTool struct {
	engine *DefaultEngine
}

func newConflictTool(engine *DefaultEngine) *agentcore.Tool {
	t := &conflictTool{engine: engine}
	return &agentcore.Tool{
		Name:        ConflictToolName,
		Description: conflictToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claims": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"claim_id":      map[string]any{"type": "string"},
							"supporting":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"contradicting": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required": []string{"claim_id"},
					},
				},
			},
			"required":             []string{"claims"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type conflictClaimInput struct {
	ClaimID       string   `json:"claim_id"`
	Supporting    []string `json:"supporting"`
	Contradicting []string `json:"contradicting"`
}

type conflictArgs struct {
	Claims []conflictClaimInput `json:"claims"`
}

func (t *conflictTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p conflictArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if len(p.Claims) == 0 {
		return map[string]any{"conflicts": []map[string]any{}}, nil
	}

	cb := agentcore_evidence.NewClaimBinding()
	for _, c := range p.Claims {
		for _, sid := range c.Supporting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionSupporting, ClaimRefs: []string{c.ClaimID},
			})
		}
		for _, sid := range c.Contradicting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionContradicting, ClaimRefs: []string{c.ClaimID},
			})
		}
	}

	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()

	var out []map[string]any
	for _, c := range conflicts {
		out = append(out, map[string]any{
			"type":        string(c.Type),
			"description": c.Description,
			"span_ids":    c.SpanIDs,
		})
	}
	return map[string]any{"conflicts": out}, nil
}
