package domains

import (
	"context"
	"encoding/json"
	"fmt"
)

func (e *CaseExtension) handleFocusCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input focusCaseInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("focus_case: %w", err)
	}

	// 先按 CaseID 查，再按申请号查
	rec, err := e.index.GetCase(ctx, input.CaseID)
	if err != nil {
		// 按 CaseID 未命中→回退按申请号查找，不是漏处理错误。
		rec, err = e.index.FindByFilingNumber(ctx, input.CaseID)
		if err != nil {
			// 两种键都未命中→以「未找到」业务结果返回，不构造 Go error。
			return focusResult{Found: false, Message: fmt.Sprintf("未找到案件: %s", input.CaseID)}, nil
		}
	}

	paths, _ := e.index.GetPaths(ctx, rec.CaseID)
	docs, _ := e.index.GetDocuments(ctx, rec.CaseID)

	return focusResult{
		Found:    true,
		CaseID:   rec.CaseID,
		Identity: rec.PrimaryIdentity(),
		Label:    rec.DisplayLabel(),
		Stage:    rec.IdentityStage,
		Status:   rec.Status,
		Paths:    pathStrings(paths),
		DocCount: len(docs),
		Message:  fmt.Sprintf("已聚焦案件: %s", rec.DisplayLabel()),
	}, nil
}
