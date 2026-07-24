package prompt

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestNewListPromptsTool(t *testing.T) {
	store, err := NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}

	tool := NewListPromptsTool(store)
	if tool == nil {
		t.Fatal("expected tool")
	}
	if tool.Name != "list_prompts" {
		t.Fatalf("expected tool name list_prompts, got %q", tool.Name)
	}
	if !tool.ReadOnly {
		t.Fatal("expected list_prompts to be read-only")
	}

	// Call without args.
	res, err := tool.Func(context.Background(), nil)
	if err != nil {
		t.Fatalf("tool.Func: %v", err)
	}
	handoff, ok := res.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("expected handoff result, got %T", res)
	}

	var parsed listResult
	if err := json.Unmarshal([]byte(handoff.Result), &parsed); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if parsed.Count == 0 {
		t.Fatal("expected some templates")
	}
}
