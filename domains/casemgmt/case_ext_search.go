package casemgmt

import (
	"context"
	"encoding/json"
	"fmt"
)

func (e *CaseExtension) handleSearchCases(ctx context.Context, args json.RawMessage) (any, error) {
	var input searchCasesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("search_cases: %w", err)
	}

	cases, err := e.index.SearchCases(ctx, CaseSearchQuery{Text: input.Query})
	if err != nil {
		return errorResult("search_cases", err), nil
	}
	return caseListResult(cases), nil
}
