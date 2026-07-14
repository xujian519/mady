package sqlite

import (
	"context"
	"fmt"
	"os"
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
