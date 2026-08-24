package plantask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// TestFlexiblePlanStore_RejectTraversalCaseID 验证不可信的 case_id（含路径分隔符/
// 相对路径）不会被拼进文件路径，从而杜绝逃逸 baseDir 的读写。
func TestFlexiblePlanStore_RejectTraversalCaseID(t *testing.T) {
	dir := t.TempDir()
	store := NewFlexiblePlanStore(dir)

	for _, bad := range []string{"../evil", "..\\evil", "/etc/passwd", ".", "..", "a/b/c", ""} {
		if _, err := store.Load(bad); err == nil {
			t.Errorf("Load(%q) should reject unsafe case_id", bad)
		}
		if err := store.Save(&FlexiblePlan{CaseID: bad}); err == nil {
			t.Errorf("Save(%q) should reject unsafe case_id", bad)
		}
		// 逃逸文件不应落盘
		joined := filepath.Join(dir, filepath.FromSlash(bad+".json"))
		if _, err := os.Stat(joined); err == nil {
			t.Errorf("unsafe case_id %q persisted file at %s", bad, joined)
		}
	}

	// 合法 case_id 往返读写正常。
	if err := store.Save(&FlexiblePlan{CaseID: "case-1", Status: "active"}); err != nil {
		t.Fatalf("save valid: %v", err)
	}
	if _, err := store.Load("case-1"); err != nil {
		t.Fatalf("load valid: %v", err)
	}
}

// TestFlexiblePlanTool_RejectTraversalCaseID 验证工具层对穿越 case_id 返回失败结果，
// 且不会在 baseDir 外写出文件。
func TestFlexiblePlanTool_RejectTraversalCaseID(t *testing.T) {
	dir := t.TempDir()
	store := NewFlexiblePlanStore(dir)
	tool := NewFlexiblePlanTool(store)

	args := json.RawMessage(`{"action":"create","case_id":"../../.ssh/id_rsa","case_type":"invalidation","input_text":"x","stages":[{"id":"s1","goal":"g","strategy":"none"}]}`)
	res, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hr, ok := res.(agentcore.HandoffResult); ok && hr.Success {
		t.Error("expected failure result for traversal case_id")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "..", ".ssh", "id_rsa.json")); err == nil {
		t.Error("traversal case_id wrote file outside baseDir")
	}
}

// TestSafeCaseID 覆盖净化规则的边界情况。
func TestSafeCaseID(t *testing.T) {
	valid := []string{"case-1", "CN20260001", "a_b.c", "123"}
	for _, v := range valid {
		if !isSafeCaseID(v) {
			t.Errorf("isSafeCaseID(%q) = false, want true", v)
		}
	}
	invalid := []string{"", ".", "..", "../x", "/abs", "a/b", "a\\b", "x/../y"}
	for _, v := range invalid {
		if isSafeCaseID(v) {
			t.Errorf("isSafeCaseID(%q) = true, want false", v)
		}
	}
}
