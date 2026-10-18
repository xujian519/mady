package caseindex

// timefmt_test.go 覆盖案件索引库时间列格式统一的回归用例。
// 历史缺陷：case_paths.created_at / case_events.event_date 依赖建表默认值
// datetime('now')（"YYYY-MM-DD HH:MM:SS"），读取侧按 RFC3339 解析失败后
// 静默退化为零值时间。

import (
	"context"
	"testing"
	"time"
)

// TestCaseIndex_TimestampsNotZero 验证主要写入路径读回的时间字段非零。
func TestCaseIndex_TimestampsNotZero(t *testing.T) {
	ci := newTestCaseIndex(t)
	ctx := context.Background()

	if err := ci.CreateCase(ctx, CaseRecord{
		CaseID:      "case-ts",
		ClientName:  "张三科技",
		PatentTitle: "锂电池正极材料",
		PatentType:  "发明专利",
		Year:        2024,
		PrimaryPath: "/tmp/cases/ts",
	}); err != nil {
		t.Fatalf("CreateCase: %v", err)
	}

	rec, err := ci.GetCase(ctx, "case-ts")
	if err != nil {
		t.Fatalf("GetCase: %v", err)
	}
	if rec.CreatedAt.IsZero() || rec.UpdatedAt.IsZero() {
		t.Errorf("case timestamps = (%v, %v), want both non-zero", rec.CreatedAt, rec.UpdatedAt)
	}

	paths, err := ci.GetPaths(ctx, "case-ts")
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("GetPaths returned %d paths, want 1", len(paths))
	}
	if paths[0].CreatedAt.IsZero() {
		t.Error("path CreatedAt = zero, want parsed creation time")
	}

	events, err := ci.GetEvents(ctx, "case-ts")
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("GetEvents returned %d events, want 1", len(events))
	}
	if events[0].EventDate.IsZero() {
		t.Error("event EventDate = zero, want parsed event time")
	}

	if err := ci.AddEvent(ctx, "case-ts", "note", `{"k":"v"}`); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	events, err = ci.GetEvents(ctx, "case-ts")
	if err != nil {
		t.Fatalf("GetEvents after AddEvent: %v", err)
	}
	if len(events) != 2 || events[1].EventDate.IsZero() {
		t.Fatalf("events after AddEvent = %d, last EventDate = %v", len(events), events[1].EventDate)
	}
	if events[1].EventDate.Before(events[0].EventDate) {
		t.Errorf("events out of order: %v then %v", events[0].EventDate, events[1].EventDate)
	}

	docs := []CaseDocument{{CaseID: "case-ts", DocType: DocFiling, DocPath: "/tmp/f.docx"}}
	if err := ci.RecordDocument(ctx, docs[0]); err != nil {
		t.Fatalf("RecordDocument: %v", err)
	}
	doc, err := ci.GetDocument(ctx, "case-ts", DocFiling)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.ParsedAt.IsZero() {
		t.Error("doc ParsedAt = zero, want parsed time")
	}
}

// TestCaseIndex_LegacyTimestampsReadable 验证历史遗留的 datetime('now')
// 文本（空格分隔、无时区）仍可被正确解析，避免旧库升级后时间字段归零。
func TestCaseIndex_LegacyTimestampsReadable(t *testing.T) {
	ci := newTestCaseIndex(t)
	ctx := context.Background()

	if err := ci.CreateCase(ctx, CaseRecord{
		CaseID: "case-legacy", ClientName: "客户L", PatentTitle: "旧库案件",
		PatentType: "发明专利", Year: 2023, PrimaryPath: "/tmp/legacy",
	}); err != nil {
		t.Fatalf("CreateCase: %v", err)
	}

	legacy := "2026-10-18 15:04:05"
	want := time.Date(2026, 10, 18, 15, 4, 5, 0, time.UTC)
	if _, err := ci.db.ExecContext(ctx,
		`UPDATE cases SET created_at = ?, updated_at = ? WHERE case_id = ?`,
		legacy, legacy, "case-legacy"); err != nil {
		t.Fatalf("update legacy timestamps: %v", err)
	}

	rec, err := ci.GetCase(ctx, "case-legacy")
	if err != nil {
		t.Fatalf("GetCase: %v", err)
	}
	if !rec.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %s, want %s", rec.CreatedAt, want)
	}
}
