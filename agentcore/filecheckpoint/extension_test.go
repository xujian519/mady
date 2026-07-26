package filecheckpoint

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// 本文件中的 hook 测试传递 nil 作为 *agentcore.AgentRunContext 参数是
// 有意的：当前 checkpointHook 的实现不依赖 ARC 中的大多数字段（它只
// 读取 arc.Turn、arc.Input 和 arc.Messages），相关测试提供了真实的 ARC。
// 传入 nil 的场景则验证了 nil-ARC 保护的可靠性。

// ---------------------------------------------------------------------------
// FileCheckpointExtension.LifecycleHook
// ---------------------------------------------------------------------------

func TestFileCheckpointExtension_LifecycleHook(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}
}

func TestFileCheckpointExtension_LifecycleHook_ReturnsCheckpointHook(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hook := ext.LifecycleHook()
	if _, ok := hook.(*checkpointHook); !ok {
		t.Fatalf("expected *checkpointHook, got %T", hook)
	}
}

// ---------------------------------------------------------------------------
// checkpointHook.BeforeTurn
// ---------------------------------------------------------------------------

func TestCheckpointHook_BeforeTurn_BeginsTurn(t *testing.T) {
	// Use memFS to avoid real file IO.
	fs := newMemFS()
	_ = fs.WriteFile("/tmp/test/dummy.go", []byte("content"))

	ext := NewExtensionWithFS(fs, "/tmp/test")
	hook := ext.LifecycleHook()

	arc := &agentcore.AgentRunContext{
		Turn:  1,
		Input: "test input",
	}

	hook.BeforeTurn(context.Background(), arc)

	// BeforeTurn should initialize the store's active turn checkpoint.
	// SnapshotFile should succeed (non-nil cur).
	if err := ext.store.SnapshotFile("/tmp/test/dummy.go"); err != nil {
		t.Fatalf("SnapshotFile should succeed after BeforeTurn: %v", err)
	}

	// The snapshotted path should appear in CurrentTurnPaths.
	paths := ext.store.CurrentTurnPaths()
	var found bool
	for _, p := range paths {
		if p == "/tmp/test/dummy.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /tmp/test/dummy.go in CurrentTurnPaths, got %v", paths)
	}

	// End the turn — must not panic.
	hook.AfterTurn(context.Background(), arc, agentcore.TurnInfo{})

	// After EndTurn, the checkpoint should appear in List.
	list := ext.store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 checkpoint in List after EndTurn, got %d", len(list))
	}
	if list[0].Turn != 1 {
		t.Fatalf("expected checkpoint turn=1, got %d", list[0].Turn)
	}
}

func TestCheckpointHook_BeforeTurn_AcceptsNilARC(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hook := ext.LifecycleHook()

	// Must not panic.
	hook.BeforeTurn(context.Background(), nil)
}

func TestCheckpointHook_BeforeTurn_EmptyMessages(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hook := ext.LifecycleHook()

	arc := &agentcore.AgentRunContext{
		Turn:     1,
		Input:    "test",
		Messages: []agentcore.Message{},
	}

	// Must not panic with empty messages slice.
	hook.BeforeTurn(context.Background(), arc)
}

// ---------------------------------------------------------------------------
// checkpointHook.AfterTurn
// ---------------------------------------------------------------------------

func TestCheckpointHook_AfterTurn_EndsTurn(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hook := ext.LifecycleHook()

	// Start a turn first.
	hook.BeforeTurn(context.Background(), &agentcore.AgentRunContext{Turn: 1, Input: "test"})

	// End the turn — must not panic.
	hook.AfterTurn(context.Background(), &agentcore.AgentRunContext{Turn: 1}, agentcore.TurnInfo{})
}

// ---------------------------------------------------------------------------
// FileCheckpointExtension.HookProvider
// ---------------------------------------------------------------------------

func TestFileCheckpointExtension_BeforeHooks_ReturnsHook(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hooks := ext.BeforeHooks()
	if len(hooks) == 0 {
		t.Fatal("expected at least one BeforeHook")
	}
}

func TestFileCheckpointExtension_AfterHooks_ReturnsNil(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hooks := ext.AfterHooks()
	if hooks != nil {
		t.Fatalf("expected nil AfterHooks, got %v", hooks)
	}
}

// ---------------------------------------------------------------------------
// checkpointHook lifecycle sequence: BeforeTurn → AfterTurn
// ---------------------------------------------------------------------------

func TestCheckpointHook_BeforeAfterTurn_Sequence(t *testing.T) {
	ext := NewExtension("/tmp/test")
	hook := ext.LifecycleHook()

	// Simulate two turns — must not panic.
	for turn := int64(1); turn <= 2; turn++ {
		arc := &agentcore.AgentRunContext{
			Turn:  turn,
			Input: "turn input",
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on turn %d: %v", turn, r)
				}
			}()
			hook.BeforeTurn(context.Background(), arc)
			hook.AfterTurn(context.Background(), arc, agentcore.TurnInfo{HadToolCalls: true})
		}()
	}
}
