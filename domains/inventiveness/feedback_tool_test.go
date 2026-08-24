package inventiveness

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestInventivenessFeedbackTool_RejectsUnsafeInput(t *testing.T) {
	tool := NewInventivenessFeedbackTool()
	cases := []string{
		`{"case_id":"","action":"rejection"}`,        // 空 case_id
		`{"case_id":"../evil","action":"rejection"}`, // 路径穿越（不会写盘）
		`{"case_id":"case-1","action":"reject"}`,     // 非法 action
	}
	for _, raw := range cases {
		res, err := tool.Func(context.Background(), json.RawMessage(raw))
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", raw, err)
		}
		if hr, ok := res.(agentcore.HandoffResult); ok && hr.Success {
			t.Errorf("expected failure result for %s, got success", raw)
		}
	}
}
