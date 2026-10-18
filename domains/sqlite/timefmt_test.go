package sqlite

// timefmt_test.go 覆盖时间列格式统一的回归用例：历史缺陷是同一天内
// `expires_at < datetime('now')` 的字符串比较恒为假（RFC3339 的 'T' 大于
// 空格的 ' '），导致过期判定被推迟到次日，且读取侧 Parse 失败使时间字段
// 恒为零值。

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/graph"
)

func TestParseTimeText(t *testing.T) {
	utc := time.Date(2026, 10, 18, 15, 4, 5, 0, time.UTC)
	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{"RFC3339 UTC", "2026-10-18T15:04:05Z", utc},
		{"legacy datetime", "2026-10-18 15:04:05", utc},
		{"empty", "", time.Time{}},
		{"garbage", "not-a-time", time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTimeText(c.in)
			if !got.Equal(c.want) {
				t.Fatalf("parseTimeText(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

// TestExpirePending_SameDay 验证同一天内到期的 pending 会被立即标记过期。
// 旧实现用 datetime('now') 与 RFC3339 文本比较，同一天内永远判为未过期。
func TestExpirePending_SameDay(t *testing.T) {
	store := newTestApprovalStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	for _, p := range []domains.PendingApproval{
		{ID: "p_expired", SessionID: "s1", Status: domains.PendingStatusPending, CreatedAt: now.Add(-2 * time.Minute), ExpiresAt: &past},
		{ID: "p_alive", SessionID: "s1", Status: domains.PendingStatusPending, CreatedAt: now, ExpiresAt: &future},
		{ID: "p_no_expiry", SessionID: "s1", Status: domains.PendingStatusPending, CreatedAt: now},
	} {
		if err := store.SavePending(ctx, p); err != nil {
			t.Fatalf("SavePending(%s): %v", p.ID, err)
		}
	}

	n, err := store.ExpirePending(ctx)
	if err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if n != 1 {
		t.Fatalf("ExpirePending affected %d rows, want 1", n)
	}

	got, err := store.LoadPending(ctx, "p_expired")
	if err != nil {
		t.Fatalf("LoadPending(expired): %v", err)
	}
	if got.Status != domains.PendingStatusExpired {
		t.Errorf("status = %q, want %q", got.Status, domains.PendingStatusExpired)
	}
	if got.RespondedAt == nil || got.RespondedAt.IsZero() {
		t.Errorf("RespondedAt = %v, want non-zero", got.RespondedAt)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt = zero, want parsed creation time")
	}

	alive, err := store.LoadPending(ctx, "p_alive")
	if err != nil {
		t.Fatalf("LoadPending(alive): %v", err)
	}
	if alive.Status != domains.PendingStatusPending {
		t.Errorf("status = %q, want %q", alive.Status, domains.PendingStatusPending)
	}
}

// TestPendingTimestampsRoundTrip 验证 CreatedAt/ExpiresAt 写入后可无损读回。
func TestPendingTimestampsRoundTrip(t *testing.T) {
	store := newTestApprovalStore(t)
	ctx := context.Background()

	created := time.Date(2026, 10, 18, 15, 4, 5, 0, time.UTC)
	expires := created.Add(30 * time.Minute)
	want := domains.PendingApproval{
		ID:        "p_roundtrip",
		SessionID: "s_rt",
		Status:    domains.PendingStatusPending,
		CreatedAt: created,
		ExpiresAt: &expires,
	}
	if err := store.SavePending(ctx, want); err != nil {
		t.Fatalf("SavePending: %v", err)
	}

	got, err := store.LoadPending(ctx, want.ID)
	if err != nil {
		t.Fatalf("LoadPending: %v", err)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %s, want %s", got.CreatedAt, created)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %s", got.ExpiresAt, expires)
	}
}

// TestPrune_LegacyRows 验证删除按时间值比较：历史遗留的 "YYYY-MM-DD HH:MM:SS"
// 行不会被 RFC3339 cutoff 的字符串比较误判为"更早"。
func TestPrune_LegacyRows(t *testing.T) {
	dir := t.TempDir()
	store, err := NewEventStore(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	now := time.Now().UTC()
	// 两小时前的事件，按旧格式写入（模拟历史数据）。
	legacyOld := now.Add(-2 * time.Hour).Format(legacyTimeLayout)
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO workflow_events (event_type, session_id, payload, created_at) VALUES ('legacy_old', 's1', '{}', ?)`,
		legacyOld); err != nil {
		t.Fatalf("insert legacy old: %v", err)
	}
	// 半小时前的事件，按旧格式写入，必须保留。
	legacyRecent := now.Add(-30 * time.Minute).Format(legacyTimeLayout)
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO workflow_events (event_type, session_id, payload, created_at) VALUES ('legacy_recent', 's1', '{}', ?)`,
		legacyRecent); err != nil {
		t.Fatalf("insert legacy recent: %v", err)
	}
	if _, err := store.Append(ctx, "new_event", "s1", "agent", []byte(`{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	n, err := store.Prune(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("Prune deleted %d rows, want 1", n)
	}

	events, err := store.ListBySession(ctx, "s1", 10)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("remaining events = %d, want 2", len(events))
	}
	if events[0].CreatedAt.IsZero() || events[1].CreatedAt.IsZero() {
		t.Error("CreatedAt = zero, want parsed timestamps")
	}
}

// TestLoadLatest_ScansTimestamp 覆盖 created_at 以 TEXT 存储却被 Scan 到
// time.Time 导致的读取失败（checkpoint_store 长期无测试覆盖）。
func TestLoadLatest_ScansTimestamp(t *testing.T) {
	dir := t.TempDir()
	store, err := NewGraphCheckpointStore(filepath.Join(dir, "cp.db"))
	if err != nil {
		t.Fatalf("NewGraphCheckpointStore: %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	for i, id := range []string{"cp_1", "cp_2"} {
		if err := store.Save(ctx, graph.Checkpoint{
			ID:        id,
			GraphID:   "g1",
			NodeName:  "node",
			StepIndex: int64(i),
			State:     []byte(`{}`),
		}); err != nil {
			t.Fatalf("Save(%s): %v", id, err)
		}
	}

	latest, err := store.LoadLatest(ctx, "g1")
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if latest == nil {
		t.Fatal("LoadLatest returned nil")
	}
	if latest.ID != "cp_2" {
		t.Errorf("LoadLatest ID = %q, want cp_2", latest.ID)
	}
	if latest.CreatedAt.IsZero() {
		t.Error("CreatedAt = zero, want parsed creation time")
	}

	list, err := store.List(ctx, "g1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].CreatedAt.IsZero() {
		t.Fatalf("List = %d items, first CreatedAt = %v", len(list), list[0].CreatedAt)
	}
}

func newTestApprovalStore(t *testing.T) *SQLiteApprovalStore {
	t.Helper()
	store, err := NewApprovalStore(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatalf("NewApprovalStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
