package domains

import (
	"context"
	"testing"
)

func TestCaseIndex_CRUD(t *testing.T) {
	ci := newTestCaseIndex(t)
	ctx := context.Background()

	// Create
	rec := CaseRecord{
		CaseID:        "case-001",
		IdentityStage: StageDrafting,
		ClientName:    "张三科技",
		PatentTitle:   "锂电池正极材料",
		PatentType:    "发明专利",
		Year:          2024,
		Domain:        DomainPatent,
		Status:        CaseStatusActive,
		PrimaryPath:   "/tmp/cases/001",
	}
	if err := ci.CreateCase(ctx, rec); err != nil {
		t.Fatalf("CreateCase: %v", err)
	}

	// Get
	got, err := ci.GetCase(ctx, "case-001")
	if err != nil {
		t.Fatalf("GetCase: %v", err)
	}
	if got.ClientName != "张三科技" {
		t.Errorf("ClientName = %q, want 张三科技", got.ClientName)
	}
	if got.IdentityStage != StageDrafting {
		t.Errorf("IdentityStage = %q, want %q", got.IdentityStage, StageDrafting)
	}

	// Update
	got.PatentTitle = "锂电池正极材料改进"
	if err := ci.UpdateCase(ctx, *got); err != nil {
		t.Fatalf("UpdateCase: %v", err)
	}
	got2, _ := ci.GetCase(ctx, "case-001")
	if got2.PatentTitle != "锂电池正极材料改进" {
		t.Errorf("PatentTitle after update = %q", got2.PatentTitle)
	}

	// Delete
	if err := ci.DeleteCase(ctx, "case-001"); err != nil {
		t.Fatalf("DeleteCase: %v", err)
	}
	if _, err := ci.GetCase(ctx, "case-001"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestCaseIndex_UpgradeToFiled(t *testing.T) {
	ci := newTestCaseIndex(t)
	ctx := context.Background()

	rec := CaseRecord{
		CaseID:        "case-002",
		IdentityStage: StageDrafting,
		ClientName:    "李四公司",
		PatentTitle:   "伸缩支架",
		PatentType:    "实用新型",
		Year:          2024,
		Domain:        DomainPatent,
		PrimaryPath:   "/tmp/cases/002",
	}
	ci.CreateCase(ctx, rec)

	// Upgrade to filed
	if err := ci.UpgradeToFiled(ctx, "case-002", "202420123456.7"); err != nil {
		t.Fatalf("UpgradeToFiled: %v", err)
	}

	got, _ := ci.GetCase(ctx, "case-002")
	if got.IdentityStage != StageFiled {
		t.Errorf("IdentityStage = %q, want %q", got.IdentityStage, StageFiled)
	}
	if got.FilingNumber != "202420123456.7" {
		t.Errorf("FilingNumber = %q", got.FilingNumber)
	}

	// Duplicate filing number should fail
	rec2 := CaseRecord{
		CaseID:        "case-003",
		IdentityStage: StageDrafting,
		ClientName:    "王五",
		PatentTitle:   "另一个",
		Year:          2024,
		PrimaryPath:   "/tmp/cases/003",
	}
	ci.CreateCase(ctx, rec2)
	err := ci.UpgradeToFiled(ctx, "case-003", "202420123456.7")
	if err == nil {
		t.Error("expected duplicate filing number error")
	}
}

func TestCaseIndex_FindByPath(t *testing.T) {
	ci := newTestCaseIndex(t)
	ctx := context.Background()

	rec := CaseRecord{
		CaseID:      "case-path",
		ClientName:  "测试客户",
		PatentTitle: "测试专利",
		PrimaryPath: "/tmp/cases/path-test",
	}
	ci.CreateCase(ctx, rec)
	ci.AddPath(ctx, "case-path", "/tmp/cases/path-test", "")

	// Exact path match
	cases, err := ci.FindByPath(ctx, "/tmp/cases/path-test")
	if err != nil {
		t.Fatalf("FindByPath: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}

	// Subdirectory match
	cases, _ = ci.FindByPath(ctx, "/tmp/cases/path-test/subdir")
	if len(cases) != 1 {
		t.Errorf("subdir match: expected 1, got %d", len(cases))
	}

	// Non-matching path
	cases, _ = ci.FindByPath(ctx, "/tmp/other")
	if len(cases) != 0 {
		t.Errorf("non-matching: expected 0, got %d", len(cases))
	}
}

func TestCaseIndex_SearchCases(t *testing.T) {
	ci := newTestCaseIndex(t)
	ctx := context.Background()

	ci.CreateCase(ctx, CaseRecord{
		CaseID: "s1", ClientName: "张三科技", PatentTitle: "锂电池",
		PatentType: "发明专利", Year: 2024, PrimaryPath: "/tmp/s1",
	})
	ci.CreateCase(ctx, CaseRecord{
		CaseID: "s2", ClientName: "李四公司", PatentTitle: "锂电池改进",
		PatentType: "实用新型", Year: 2024, PrimaryPath: "/tmp/s2",
	})
	ci.CreateCase(ctx, CaseRecord{
		CaseID: "s3", ClientName: "张三科技", PatentTitle: "伸缩支架",
		PatentType: "实用新型", Year: 2023, PrimaryPath: "/tmp/s3",
	})

	// Search by client name
	cases, _ := ci.SearchCases(ctx, CaseSearchQuery{ClientName: "张三"})
	if len(cases) != 2 {
		t.Errorf("client search: expected 2, got %d", len(cases))
	}

	// Search by patent type
	cases, _ = ci.SearchCases(ctx, CaseSearchQuery{PatentType: "实用新型"})
	if len(cases) != 2 {
		t.Errorf("type search: expected 2, got %d", len(cases))
	}

	// Search by year
	cases, _ = ci.SearchCases(ctx, CaseSearchQuery{Year: 2023})
	if len(cases) != 1 {
		t.Errorf("year search: expected 1, got %d", len(cases))
	}

	// Full-text search
	cases, _ = ci.SearchCases(ctx, CaseSearchQuery{Text: "锂电池"})
	if len(cases) != 2 {
		t.Errorf("FTS search: expected 2, got %d", len(cases))
	}
}

func TestCaseRecord_PrimaryIdentity(t *testing.T) {
	drafting := CaseRecord{
		IdentityStage: StageDrafting,
		ClientName:    "客户A",
		PatentTitle:   "专利X",
		PatentType:    "发明专利",
		Year:          2024,
	}
	id := drafting.PrimaryIdentity()
	if id != "客户A-专利X（发明专利·2024）" {
		t.Errorf("drafting identity = %q", id)
	}

	filed := CaseRecord{
		IdentityStage: StageFiled,
		FilingNumber:  "CN202410123456.7",
	}
	if filed.PrimaryIdentity() != "CN202410123456.7" {
		t.Errorf("filed identity = %q", filed.PrimaryIdentity())
	}
}

func newTestCaseIndex(t *testing.T) *CaseIndex {
	t.Helper()
	ci, err := NewCaseIndex(":memory:")
	if err != nil {
		t.Fatalf("NewCaseIndex: %v", err)
	}
	t.Cleanup(func() { ci.Close() })
	return ci
}
