package domains

import (
	"context"
	"encoding/json"
)

func (e *CaseExtension) handleListCases(ctx context.Context, args json.RawMessage) (any, error) {
	var input listCasesInput
	_ = json.Unmarshal(args, &input)

	cases, err := e.index.SearchCases(ctx, CaseSearchQuery{
		ClientName: input.ClientName,
		Year:       input.Year,
		PatentType: input.PatentType,
		Status:     input.Status,
	})
	if err != nil {
		return errorResult("list_cases", err), nil
	}
	return caseListResult(cases), nil
}
