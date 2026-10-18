package domains

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// handleListCases 列出案件。空参数表示不过滤；参数非法时返回错误而非
// 静默按零值查询——零值查询会返回全部案件，被模型误读为"筛选命中"。
func (e *CaseExtension) handleListCases(ctx context.Context, args json.RawMessage) (any, error) {
	var input listCasesInput
	if raw := bytes.TrimSpace(args); len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return errorResult("list_cases", fmt.Errorf("解析参数: %w", err)), nil
		}
	}

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
