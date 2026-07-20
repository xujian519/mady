package acp

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/xujian519/mady/domains"
)

// newRecordingTestServer 构造一个仅用于留痕测试的 Server：挂内存 ApprovalStore，
// 不启动任何读写循环（recordPermissionDecision 不依赖 session manager）。
func newRecordingTestServer(t *testing.T, store domains.ApprovalStore) *Server {
	t.Helper()
	return NewServer(ServerConfig{
		ApprovalStore: store,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// TestPermissionDecisionFor 验证 allow/deny 到审批决策枚举的映射。
func TestPermissionDecisionFor(t *testing.T) {
	if got := permissionDecisionFor(true); got != domains.DecisionAdopted {
		t.Errorf("permissionDecisionFor(true) = %q, want adopted", got)
	}
	if got := permissionDecisionFor(false); got != domains.DecisionRejected {
		t.Errorf("permissionDecisionFor(false) = %q, want rejected", got)
	}
}

// TestRecordPermissionDecision_Persists 验证工具授权决策按 RecordDecision 模式
// 留痕：allow → adopted、deny → rejected，触发关键字标记触点来源，授权入参
// 序列化为 OriginalOutput 供回溯。
func TestRecordPermissionDecision_Persists(t *testing.T) {
	store := domains.NewMemoryApprovalStore()
	srv := newRecordingTestServer(t, store)
	ctx := context.Background()

	srv.recordPermissionDecision(ctx, "sess-1", "bash", map[string]any{"command": "ls"}, domains.DecisionAdopted, "allow_once")
	srv.recordPermissionDecision(ctx, "sess-1", "write_file", map[string]any{"path": "/tmp/x"}, domains.DecisionRejected, "reject_once")
	srv.recordPermissionDecision(ctx, "sess-1", "bash", nil, domains.DecisionRejected, "canceled_or_error")

	records, err := store.List(ctx, "sess-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	if records[0].Decision != domains.DecisionAdopted || records[0].State != domains.StateApproved {
		t.Errorf("record[0] = (%q, %q), want (adopted, approved)", records[0].Decision, records[0].State)
	}
	if records[0].TriggerKeyword != "tool_permission:bash" {
		t.Errorf("record[0] trigger = %q, want tool_permission:bash", records[0].TriggerKeyword)
	}
	if records[0].OriginalOutput == "" {
		t.Error("record[0] original output should carry the serialized tool input")
	}
	if records[0].Feedback != "allow_once" {
		t.Errorf("record[0] feedback = %q, want allow_once", records[0].Feedback)
	}

	if records[1].Decision != domains.DecisionRejected || records[1].State != domains.StateRejected {
		t.Errorf("record[1] = (%q, %q), want (rejected, rejected)", records[1].Decision, records[1].State)
	}
	// rawInput 为 nil 时 OriginalOutput 允许为空，但记录必须存在。
	if records[2].Feedback != "canceled_or_error" {
		t.Errorf("record[2] feedback = %q, want canceled_or_error", records[2].Feedback)
	}
}

// TestRecordPermissionDecision_NoStore 验证未配置 store 时为安全 no-op。
func TestRecordPermissionDecision_NoStore(t *testing.T) {
	srv := newRecordingTestServer(t, nil)
	ctx := context.Background()
	// 不应 panic，也不产生任何副作用。
	srv.recordPermissionDecision(ctx, "sess-x", "bash", nil, domains.DecisionAdopted, "allow_once")
}
