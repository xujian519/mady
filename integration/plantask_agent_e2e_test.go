//go:build integration

// PlanTask HCL agent 级端到端测试：通过真实 agentcore.Agent 调用 plantask
// 工具（InvokeTool 走 executor 全链路），验证：
//   - 7 个工具正确注册进 Agent 工具注册表
//   - plan_submit → plan_approve → workflow_interrupt 状态机经 Agent 正常流转
//   - 状态迁移事件经 Agent 事件总线发射（TUI 订阅链路的数据源）
package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/agentcore/tasklist"
	"github.com/xujian519/mady/bootstrap"
)

// TestPlantask_AgentToolRegistry 验证工具经 Agent 注册且名称正确。
func TestPlantask_AgentToolRegistry(t *testing.T) {
	ext := buildAgentHarness(t)
	agent := agentcore.New(agentcore.Config{ModelConfig: agentcore.ModelConfig{Name: "pt-e2e", Model: "stub", Provider: stubProvider{}}})
	if err := ext.Init(context.Background(), agent); err != nil { // 真实装配路径：Extension.Init 绑定 agent
		t.Fatal(err)
	}
	agent.RegisterTools(ext.Tools()...)

	names := agent.ToolNames()
	want := []string{
		"plan_submit", "plan_approve", "plan_reject", "plan_revise",
		"workflow_interrupt", "workflow_resume", "workflow_feedback",
	}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("工具 %s 未注册进 Agent（已注册: %v）", w, names)
		}
	}
	if _, ok := agent.GetTool("plan_submit"); !ok {
		t.Error("GetTool(plan_submit) 应命中")
	}
}

// TestPlantask_AgentFullLoop 通过 Agent.InvokeTool 走完整 HCL 闭环。
func TestPlantask_AgentFullLoop(t *testing.T) {
	ext := buildAgentHarness(t)
	agent := agentcore.New(agentcore.Config{ModelConfig: agentcore.ModelConfig{Name: "pt-e2e", Model: "stub", Provider: stubProvider{}}})
	if err := ext.Init(context.Background(), agent); err != nil { // 真实装配路径：Extension.Init 绑定 agent
		t.Fatal(err)
	}
	agent.RegisterTools(ext.Tools()...)

	// 事件捕获：验证状态迁移事件经 Agent 总线发射（Emit 为异步 broker，用 channel 等待）。
	statusEvents := make(chan agentcore.Event, 8)
	agent.On(agentcore.EventPlanTaskStatusChanged, func(e agentcore.Event) { statusEvents <- e })

	ctx := context.Background()

	// ① plan_submit（经 Agent executor 调用）。
	sub := invokeAgentTool[plantask.PlanSubmitResult](t, agent, "plan_submit", plantask.PlanSubmitArgs{
		CaseID: "agent-e2e", CaseType: "invalidity",
		Steps: []plantask.PlanStepInput{
			{Order: 1, Strategy: "chain", Description: "检索"},
			{Order: 2, Strategy: "chain", Description: "比对"},
		},
	})
	if sub.Status != string(plantask.StatusAwaitingApproval) {
		t.Fatalf("expected awaiting_approval, got %s", sub.Status)
	}

	// ② plan_approve。
	appr := invokeAgentTool[plantask.PlanApproveResult](t, agent, "plan_approve", plantask.PlanApproveArgs{SessionID: sub.SessionID})
	if appr.Status != string(plantask.StatusExecuting) {
		t.Fatalf("expected executing, got %s", appr.Status)
	}

	// ③ workflow_interrupt（返回 ErrInterrupt，agent 层原样透传）。
	_, err := invokeAgentToolErr(t, agent, "workflow_interrupt", plantask.WorkflowInterruptArgs{SessionID: sub.SessionID})
	if err == nil || !errorsIsInterrupt(err) {
		t.Fatalf("expected interrupt error through agent, got %v", err)
	}

	// ④ workflow_feedback（recorder replanner 合并 → 回 Executing）。
	fb := invokeAgentTool[plantask.WorkflowFeedbackResult](t, agent, "workflow_feedback", plantask.WorkflowFeedbackArgs{
		SessionID: sub.SessionID,
		Feedback:  "重跑:检索",
	})
	if fb.Status != string(plantask.StatusExecuting) {
		t.Fatalf("expected executing after replan, got %s", fb.Status)
	}

	// 事件断言：submit + approve + interrupt + feedback 至少 4 次状态迁移（异步等待）。
	for i := 0; i < 4; i++ {
		select {
		case <-statusEvents:
		case <-time.After(2 * time.Second):
			t.Fatalf("期望 >=4 次状态迁移事件，仅收到 %d", i)
		}
	}

	// ⑤ 最终会话状态可经扩展查询。
	sess, err := ext.Session(ctx, sub.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != plantask.StatusExecuting || len(sess.Plan.Steps) != 2 {
		t.Errorf("final session mismatch: status=%s steps=%d", sess.Status, len(sess.Plan.Steps))
	}
}

// stubProvider 是最小 Provider 实现（仅满足 config 校验，InvokeTool 不触发模型调用）。
type stubProvider struct{}

func (stubProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{Content: "stub"}, nil
}
func (stubProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta)
	close(ch)
	return ch, nil
}

// buildAgentHarness 构造带 recorder replanner 的扩展（agent 级测试用）。
func buildAgentHarness(t *testing.T) *plantask.Extension {
	t.Helper()
	taskStore := tasklist.NewMemoryStore()
	bridge := bootstrap.NewPlantaskBridge(taskStore)
	ext, err := plantask.NewExtension(plantask.Config{
		Store:        plantask.NewMemoryStore(),
		TaskStore:    taskStore,
		Gate:         &fakeGate{},
		Replanner:    bridge,
		NewSessionID: func(caseID string) string { return caseID + "_sess" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext
}

// invokeAgentTool 经 Agent.InvokeTool 调用工具并反序列化结果。
func invokeAgentTool[TResult any](t *testing.T, agent *agentcore.Agent, name string, args any) TResult {
	t.Helper()
	var zero TResult
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	res, err := agent.InvokeTool(context.Background(), name, raw)
	if err != nil {
		t.Fatalf("InvokeTool(%s): %v", name, err)
	}
	if err := json.Unmarshal([]byte(res), &zero); err != nil {
		t.Fatalf("unmarshal %s result %q: %v", name, res, err)
	}
	return zero
}

// invokeAgentToolErr 经 Agent.InvokeTool 调用并返回错误。
func invokeAgentToolErr(t *testing.T, agent *agentcore.Agent, name string, args any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return agent.InvokeTool(context.Background(), name, raw)
}

func errorsIsInterrupt(err error) bool {
	return errors.Is(err, agentcore.ErrInterrupt)
}
