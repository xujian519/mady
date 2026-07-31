package plantask

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/tasklist"
)

// autoEnterExt 构造开启自动进入的扩展（默认 N=2）。
func autoEnterExt(t *testing.T, rounds int) (*Extension, *fakeGate) {
	t.Helper()
	gate := &fakeGate{}
	ext, err := NewExtension(Config{
		Store:        NewMemoryStore(),
		TaskStore:    taskListMem(),
		Gate:         gate,
		NewSessionID: func(caseID string) string { return caseID + "_sess" },
		AutoEnter:    AutoEnterConfig{Rounds: rounds},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext, gate
}

func taskListMem() *tasklist.MemoryStore { return tasklist.NewMemoryStore() }

func highDecision(turn int64) agentcore.GatewayDecision {
	return agentcore.GatewayDecision{Turn: turn, Complexity: agentcore.ComplexityHigh}
}

// TestAutoEnter_ConsecutiveRounds 连续 N 轮 High → 激活门控 + 创建 Planning 会话。
func TestAutoEnter_ConsecutiveRounds(t *testing.T) {
	ext, gate := autoEnterExt(t, 2)
	// 第一轮：未达门槛。
	ext.AutoEnterPlanning(nil, highDecision(1))
	if gate.IsActive() {
		t.Fatal("gate must not activate before N rounds")
	}
	// 第二轮：达到门槛。
	ext.AutoEnterPlanning(nil, highDecision(2))
	if !gate.IsActive() {
		t.Fatal("gate must activate after N consecutive high turns")
	}
	pending, _ := ext.cfg.Store.ListPending(context.Background())
	if len(pending) != 1 || pending[0].Status != StatusPlanning {
		t.Errorf("expected 1 planning session, got %v", pending)
	}
}

// TestAutoEnter_SameTurnDedupe 同轮多次模型调用只计 1 次。
func TestAutoEnter_SameTurnDedupe(t *testing.T) {
	ext, gate := autoEnterExt(t, 3)
	// 同一用户输入（turn=1）内 5 次模型调用 → 只算 1 轮。
	for i := 0; i < 5; i++ {
		ext.AutoEnterPlanning(nil, highDecision(1))
	}
	if gate.IsActive() {
		t.Fatal("gate must not activate on same-turn dedupe")
	}
	// 后续两轮（turn 2、3）达到门槛。
	ext.AutoEnterPlanning(nil, highDecision(2))
	ext.AutoEnterPlanning(nil, highDecision(3))
	if !gate.IsActive() {
		t.Fatal("gate must activate after 3 distinct high turns")
	}
}

// TestAutoEnter_TurnGapResets 轮次缺口（中间非 High 轮）清零计数。
func TestAutoEnter_TurnGapResets(t *testing.T) {
	ext, gate := autoEnterExt(t, 2)
	ext.AutoEnterPlanning(nil, highDecision(1))
	// turn 3 而非 2 → 中间存在非 High 轮，计数清零。
	ext.AutoEnterPlanning(nil, highDecision(3))
	if gate.IsActive() {
		t.Fatal("gap between turns must reset the counter")
	}
	ext.AutoEnterPlanning(nil, highDecision(4))
	if !gate.IsActive() {
		t.Fatal("gate must activate after fresh 2-round streak")
	}
}

// TestAutoEnter_SkipWhenGateActive planmode 已激活（手动 /plan）时跳过。
func TestAutoEnter_SkipWhenGateActive(t *testing.T) {
	ext, gate := autoEnterExt(t, 1)
	gate.Activate()
	ext.AutoEnterPlanning(nil, highDecision(1))
	// 门控保持激活但不应创建新会话。
	pending, _ := ext.cfg.Store.ListPending(context.Background())
	if len(pending) != 0 {
		t.Errorf("no session should be created when gate already active, got %d", len(pending))
	}
}

// TestAutoEnter_SkipWhenSessionActive 已有活动会话时不重复创建。
func TestAutoEnter_SkipWhenSessionActive(t *testing.T) {
	ext, _ := autoEnterExt(t, 1)
	_ = ext.cfg.Store.Save(context.Background(), newTestSession("existing", "case1", StatusAwaitingApproval))
	ext.AutoEnterPlanning(nil, highDecision(1))
	pending, _ := ext.cfg.Store.ListPending(context.Background())
	if len(pending) != 1 {
		t.Errorf("must not create duplicate session, got %d pending", len(pending))
	}
}

// TestAutoEnter_Disabled Rounds=0 时完全关闭。
func TestAutoEnter_Disabled(t *testing.T) {
	ext, gate := autoEnterExt(t, 0)
	ext.AutoEnterPlanning(nil, highDecision(1))
	ext.AutoEnterPlanning(nil, highDecision(2))
	if gate.IsActive() {
		t.Fatal("auto enter must be disabled with Rounds=0")
	}
}
