package doomloop

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// 建议性防循环提醒（reminder）测试
// =============================================================================

type callSet struct {
	hook agentcore.LifecycleHook
	dl   *DoomLoop
	arc  *agentcore.AgentRunContext
	ctx  context.Context
}

func newCallSet(t *testing.T, opts ...Option) *callSet {
	t.Helper()
	dl := New(opts...)
	cs := &callSet{
		hook: dl.AsHook(),
		dl:   dl,
		arc:  &agentcore.AgentRunContext{},
		ctx:  context.Background(),
	}
	if err := cs.hook.BeforeAgentRun(cs.ctx, cs.arc); err != nil {
		t.Fatal(err)
	}
	return cs
}

// runToolTurns 模拟 n 轮"工具执行→模型调用"，返回最后一轮的请求（含注入结果）。
func (cs *callSet) runToolTurns(t *testing.T, userMsg string, n int, call agentcore.ToolCall) *agentcore.ProviderRequest {
	t.Helper()
	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: userMsg}},
	}
	for i := 0; i < n; i++ {
		cs.hook.AfterToolExecution(cs.ctx, cs.arc, &agentcore.ToolExecutionContext{ToolCalls: []agentcore.ToolCall{call}})
		mcc := &agentcore.ModelCallContext{Request: req}
		if err := cs.hook.BeforeModelCall(cs.ctx, cs.arc, mcc); err != nil {
			t.Fatal(err)
		}
	}
	return req
}

func lastMessage(req *agentcore.ProviderRequest) agentcore.Message {
	return req.Messages[len(req.Messages)-1]
}

func TestReminder_InjectsAtThirdRepeat(t *testing.T) {
	cs := newCallSet(t)
	call := agentcore.ToolCall{Name: "patent_search", Arguments: `{"query":"无人机","limit":10}`}

	// 前两轮：不注入。
	req := cs.runToolTurns(t, "帮我检索无人机专利", 2, call)
	if got := len(req.Messages); got != 1 {
		t.Fatalf("no reminder expected after 2 repeats, got %d messages", got)
	}

	// 第三轮：注入提醒。
	req = cs.runToolTurns(t, "帮我检索无人机专利", 1, call)
	last := lastMessage(req)
	if last.Role != agentcore.RoleUser || !strings.HasPrefix(last.Content, reminderMarker) {
		t.Fatalf("expected reminder injection, got role=%q content=%q", last.Role, last.Content)
	}
	if !strings.Contains(last.Content, "patent_search") || !strings.Contains(last.Content, "3") {
		t.Errorf("reminder should name tool and count, got %q", last.Content)
	}
}

func TestReminder_ArgOrderNormalized(t *testing.T) {
	cs := newCallSet(t)
	// 参数书写顺序不同，语义相同：应累计为连续同参调用。
	cs.runToolTurns(t, "检索", 1, agentcore.ToolCall{Name: "search", Arguments: `{"a":1,"b":2}`})
	cs.runToolTurns(t, "检索", 1, agentcore.ToolCall{Name: "search", Arguments: `{"b":2,"a":1}`})
	req := cs.runToolTurns(t, "检索", 1, agentcore.ToolCall{Name: "search", Arguments: `{"a": 1, "b": 2}`})

	if !strings.HasPrefix(lastMessage(req).Content, reminderMarker) {
		t.Errorf("arg-order-insensitive repeats should trigger reminder, got %q", lastMessage(req).Content)
	}
}

func TestReminder_DifferentCallResetsCount(t *testing.T) {
	cs := newCallSet(t)
	a := agentcore.ToolCall{Name: "search", Arguments: `{"q":"x"}`}
	b := agentcore.ToolCall{Name: "other_tool", Arguments: `{"q":"x"}`}

	cs.runToolTurns(t, "任务", 1, a)
	cs.runToolTurns(t, "任务", 1, a)
	cs.runToolTurns(t, "任务", 1, b) // 打断连续性
	req := cs.runToolTurns(t, "任务", 1, a)

	if len(req.Messages) != 1 {
		t.Errorf("interrupted repeats must not trigger reminder, got %d messages", len(req.Messages))
	}
}

func TestReminder_NewUserMessageResets(t *testing.T) {
	cs := newCallSet(t)
	call := agentcore.ToolCall{Name: "search", Arguments: `{"q":"x"}`}

	// 连打 3 次触发提醒注入。
	req := cs.runToolTurns(t, "第一轮任务", 3, call)
	if !strings.HasPrefix(lastMessage(req).Content, reminderMarker) {
		t.Fatal("expected reminder at 3rd repeat")
	}

	// 新的真实用户消息开启新一轮：提醒被消费且计数清零。
	cs.runToolTurns(t, "第二轮任务", 2, call)
	// 此时连击只有 2，不应再注入。用第 3 轮消息验证。
	req = cs.runToolTurns(t, "第二轮任务", 0, call)
	if len(req.Messages) != 1 {
		t.Errorf("count should reset on new user turn, got %d messages", len(req.Messages))
	}
}

func TestReminder_ReminderDoesNotResetItself(t *testing.T) {
	cs := newCallSet(t)
	call := agentcore.ToolCall{Name: "search", Arguments: `{"q":"x"}`}

	// 触发注入后，继续用相同参数调用：注入的提醒消息不应被误判为新用户轮次，
	// 计数应继续累加并在第 5 次再次提醒。
	cs.runToolTurns(t, "任务", 3, call)        // 注入 #1（count=3）
	req := cs.runToolTurns(t, "任务", 2, call) // count=4,5 → 注入 #2
	if !strings.Contains(lastMessage(req).Content, "5") {
		t.Errorf("expected second reminder at count 5, got %q", lastMessage(req).Content)
	}
}

func TestReminder_Disabled(t *testing.T) {
	cs := newCallSet(t, WithoutReminder())
	call := agentcore.ToolCall{Name: "search", Arguments: `{"q":"x"}`}
	req := cs.runToolTurns(t, "任务", 8, call)
	if len(req.Messages) != 1 {
		t.Errorf("disabled reminder must never inject, got %d messages", len(req.Messages))
	}
}

func TestReminder_CustomThresholds(t *testing.T) {
	cs := newCallSet(t, WithReminderThresholds(2))
	call := agentcore.ToolCall{Name: "search", Arguments: `{"q":"x"}`}
	req := cs.runToolTurns(t, "任务", 2, call)
	if !strings.HasPrefix(lastMessage(req).Content, reminderMarker) {
		t.Errorf("custom threshold 2 should fire, got %q", lastMessage(req).Content)
	}
}

func TestNormalizeToolArgs_NonJSON(t *testing.T) {
	if got := normalizeToolArgs("not-json"); got != "not-json" {
		t.Errorf("non-JSON args should pass through, got %q", got)
	}
}

func TestLastRealUserContent_SkipsReminder(t *testing.T) {
	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "真实问题"},
		{Role: agentcore.RoleAssistant, Content: "回答"},
		{Role: agentcore.RoleUser, Content: reminderMarker + " 提醒"},
	}
	if got := lastRealUserContent(msgs); got != "真实问题" {
		t.Errorf("lastRealUserContent = %q, want 真实问题", got)
	}
}
