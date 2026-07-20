package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/domains"
)

func TestSQLiteApprovalStore_SaveAndList(t *testing.T) {
	dir, err := os.MkdirTemp("", "approval_test")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewApprovalStore(dir + "/approval.db")
	if err != nil {
		t.Fatalf("NewApprovalStore: %v", err)
	}
	defer store.Close()

	rec := domains.ApprovalRecord{
		ID:             "appr_001",
		SessionID:      "session_1",
		CaseID:         "case_001",
		TriggerKeyword: "专利结论",
		OriginalOutput: "该权利要求具有新颖性",
		Decision:       domains.DecisionAdopted,
	}
	ctx := context.Background()
	if err := store.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	records, err := store.List(ctx, "session_1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("List returned %d records, want 1", len(records))
	}
	if records[0].ID != "appr_001" {
		t.Errorf("record ID=%q, want appr_001", records[0].ID)
	}
	if records[0].Decision != domains.DecisionAdopted {
		t.Errorf("decision=%q, want adopted", records[0].Decision)
	}
}

// TestSQLiteApprovalStore_StateRoundTrip 验证审批状态机的 State 随记录持久化，
// 且旧格式（无 state 字段）记录读取时能从 Decision 推导重建。
func TestSQLiteApprovalStore_StateRoundTrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "approval_test")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewApprovalStore(dir + "/approval.db")
	if err != nil {
		t.Fatalf("NewApprovalStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	rec := domains.ApprovalRecord{
		ID:        "appr_state",
		SessionID: "session_state",
		Decision:  domains.DecisionModified,
		State:     domains.StateModified,
	}
	if err := store.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	records, err := store.List(ctx, "session_state")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("List returned %d records, want 1", len(records))
	}
	if records[0].State != domains.StateModified {
		t.Errorf("state=%q, want %q", records[0].State, domains.StateModified)
	}

	// 旧格式数据（无 state 字段）：读取时应按 Decision 重建状态。
	legacy := `{"id":"appr_legacy","session_id":"session_state","decision":"rejected"}`
	got, err := unmarshalRecord([]byte(legacy))
	if err != nil {
		t.Fatalf("unmarshalRecord legacy: %v", err)
	}
	if got.State != domains.StateRejected {
		t.Errorf("legacy state=%q, want %q (derived from decision)", got.State, domains.StateRejected)
	}
}

func TestSQLiteApprovalStore_ListByCase(t *testing.T) {
	dir, err := os.MkdirTemp("", "approval_test")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewApprovalStore(dir + "/approval.db")
	if err != nil {
		t.Fatalf("NewApprovalStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		rec := domains.ApprovalRecord{
			ID:       fmt.Sprintf("appr_%03d", i+1),
			CaseID:   "case_abc",
			Decision: domains.DecisionAdopted,
		}
		if err := store.Save(ctx, rec); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	// ListByCase returns all 3 records
	records, err := store.ListByCase(ctx, "case_abc")
	if err != nil {
		t.Fatalf("ListByCase: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("ListByCase returned %d records, want 3", len(records))
	}
}

func TestSQLiteApprovalStore_Delete(t *testing.T) {
	dir, err := os.MkdirTemp("", "approval_test")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewApprovalStore(dir + "/approval.db")
	if err != nil {
		t.Fatalf("NewApprovalStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	rec := domains.ApprovalRecord{
		ID:       "appr_del",
		CaseID:   "case_del",
		Decision: domains.DecisionRejected,
	}
	if err := store.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete(ctx, "appr_del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	records, _ := store.ListByCase(ctx, "case_del")
	if len(records) != 0 {
		t.Errorf("expected 0 records after delete, got %d", len(records))
	}
}

func TestNewApprovalStore_FailsReadonlyDatabase(t *testing.T) {
	dir, err := os.MkdirTemp("", "approval_test")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "approval.db")
	store, err := NewApprovalStore(dbPath)
	if err != nil {
		t.Fatalf("NewApprovalStore initial create: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := os.Chmod(dbPath, 0o444); err != nil {
		t.Fatalf("Chmod readonly: %v", err)
	}
	defer func() { _ = os.Chmod(dbPath, 0o644) }()

	_, err = NewApprovalStore(dbPath)
	if err == nil {
		t.Fatal("NewApprovalStore should fail for readonly database")
	}
	if !strings.Contains(err.Error(), "readonly") && !strings.Contains(err.Error(), "write probe") {
		t.Fatalf("unexpected error: %v", err)
	}
}
