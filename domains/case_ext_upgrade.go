package domains

import (
	"context"
	"encoding/json"
	"fmt"
)

func (e *CaseExtension) handleUpgradeCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input upgradeCaseInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("upgrade_case_identity: %w", err)
	}

	if input.FilingNumber != "" {
		if err := e.index.UpgradeToFiled(ctx, input.CaseID, input.FilingNumber); err != nil {
			return errorResult("upgrade_case_identity", err), nil
		}
	}
	if input.PublicationNumber != "" {
		if err := e.index.UpgradeToPublished(ctx, input.CaseID, input.PublicationNumber); err != nil {
			return errorResult("upgrade_case_identity", err), nil
		}
	}

	rec, _ := e.index.GetCase(ctx, input.CaseID)
	stage := ""
	label := ""
	if rec != nil {
		stage = rec.IdentityStage
		label = rec.DisplayLabel()
	}

	return map[string]any{
		"case_id": input.CaseID,
		"stage":   stage,
		"label":   label,
		"message": fmt.Sprintf("案件标识已升级: %s（阶段: %s）", label, stageLabel(stage)),
	}, nil
}
