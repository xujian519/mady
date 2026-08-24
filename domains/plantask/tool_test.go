package plantask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestPatentPlanTask_Transition(t *testing.T) {
	args := json.RawMessage(`{"action":"transition","current_state":"planning","to":"awaiting_approval"}`)
	res, err := handlePlanTask(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := res.(string); !ok || !contains(s, "planning → awaiting_approval") {
		t.Errorf("unexpected result: %v", res)
	}
}

func TestPatentPlanTask_InvalidTransition(t *testing.T) {
	args := json.RawMessage(`{"action":"transition","current_state":"planning","to":"finished"}`)
	res, err := handlePlanTask(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hr, ok := res.(agentcore.HandoffResult); ok && hr.Success {
		t.Error("expected failure for illegal transition")
	}
}

func TestPatentPlanTask_Sync(t *testing.T) {
	args := json.RawMessage(`{"action":"sync","plan_steps":["检索","分析","撰写"]}`)
	res, err := handlePlanTask(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := res.(string); !ok || !contains(s, "task_count") {
		t.Errorf("unexpected result: %v", res)
	}
}

func TestFlexiblePlanTool_CreateGet(t *testing.T) {
	dir := t.TempDir()
	store := NewFlexiblePlanStore(dir)
	tool := NewFlexiblePlanTool(store)

	args := json.RawMessage(`{"action":"create","case_id":"c1","case_type":"invalidation","input_text":"test","stages":[{"id":"s1","name":"检索","goal":"完成检索","strategy":"chain"}]}`)
	res, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if s, ok := res.(string); !ok || !contains(s, "active") {
		t.Errorf("unexpected create result: %v", res)
	}

	args = json.RawMessage(`{"action":"get","case_id":"c1"}`)
	res, err = tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if s, ok := res.(string); !ok || !contains(s, "s1") {
		t.Errorf("unexpected get result: %v", res)
	}

	if _, err := os.Stat(filepath.Join(dir, "c1.json")); err != nil {
		t.Errorf("expected persisted plan file: %v", err)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
