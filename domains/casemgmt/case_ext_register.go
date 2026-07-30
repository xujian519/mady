package casemgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/xujian519/mady/domains"
)

func (e *CaseExtension) handleRegisterCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input registerCaseInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("register_case: %w", err)
	}

	stage := stageForInfo(ExtractedCaseInfo{FilingNumber: input.FilingNumber})

	primaryPath := input.Path
	if primaryPath == "" {
		primaryPath = e.cwd
	}

	rec := CaseRecord{
		CaseID:        uuid.New().String(),
		IdentityStage: stage,
		FilingNumber:  input.FilingNumber,
		ClientName:    input.ClientName,
		PatentTitle:   input.PatentTitle,
		PatentType:    input.PatentType,
		Year:          time.Now().Year(),
		Domain:        domains.DomainPatent,
		Status:        CaseStatusActive,
		PrimaryPath:   primaryPath,
	}
	if err := e.index.CreateCase(ctx, rec); err != nil {
		return errorResult("register_case", err), nil
	}

	if primaryPath != "" {
		_, _ = EnsureCaseWorkspace(primaryPath)
	}

	return map[string]any{
		"case_id":  rec.CaseID,
		"identity": rec.PrimaryIdentity(),
		"stage":    rec.IdentityStage,
		"message":  fmt.Sprintf("已创建案件: %s", rec.DisplayLabel()),
	}, nil
}

// applyIdentityUpgrade upgrades case identity based on extracted info.
// Logs warnings on failure instead of silently swallowing errors.
func (e *CaseExtension) applyIdentityUpgrade(ctx context.Context, rec *CaseRecord, merged ExtractedCaseInfo) {
	if merged.FilingNumber != "" && rec.IdentityStage == StageDrafting {
		if err := e.index.UpgradeToFiled(ctx, rec.CaseID, merged.FilingNumber); err != nil {
			slog.Warn("case_extension: identity upgrade to filed failed",
				"case_id", rec.CaseID, "err", err)
		}
	}
	if merged.PublicationNumber != "" && rec.IdentityStage != StagePublished {
		if err := e.index.UpgradeToPublished(ctx, rec.CaseID, merged.PublicationNumber); err != nil {
			slog.Warn("case_extension: identity upgrade to published failed",
				"case_id", rec.CaseID, "err", err)
		}
	}
}
