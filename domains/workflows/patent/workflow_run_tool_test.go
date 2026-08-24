package patent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestWorkflowRunTool_PlanAndTrace(t *testing.T) {
	tool := NewPatentWorkflowRunTool(nil)
	out, err := tool.Func(context.Background(), json.RawMessage(`{"workflow_id":"novelty-analysis"}`))
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("expected string output, got %T", out)
	}
	if !strings.Contains(s, `"ok":true`) || !strings.Contains(s, "novelty-analysis") {
		t.Errorf("unexpected output: %s", s)
	}
	if !strings.Contains(s, "BuildNoveltyGraph") {
		t.Errorf("步骤应解析 entry_label 到既有入口，got: %s", s)
	}
}

func TestWorkflowRunTool_UnknownWorkflow(t *testing.T) {
	tool := NewPatentWorkflowRunTool(nil)
	out, err := tool.Func(context.Background(), json.RawMessage(`{"workflow_id":"does-not-exist"}`))
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	hr, ok := out.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("expected HandoffResult for failure, got %T", out)
	}
	if hr.Action == "" {
		t.Errorf("expected a failure action label, got empty")
	}
}

func TestWorkflowRunTool_MissingID(t *testing.T) {
	tool := NewPatentWorkflowRunTool(nil)
	if _, err := tool.Func(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Func: %v", err)
	}
}
