//go:build integration

// PlanTask HCL 端到端测试：规划 → 批准 → 执行 → 打断 → 反馈 → 重规划 → 续跑。
//
// 用 recorder 测试替身（03-design §6 / 04-tasks 2.4-2.5）记录每次实际执行的
// 步骤，断言"已完成步骤不重跑"与"重跑:语法生效"。
package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/agentcore/tasklist"
	"github.com/xujian519/mady/bootstrap"
)

// execRecorder 记录实际执行的步骤描述（测试替身执行日志）。
type execRecorder struct {
	executed []string // 每次执行的步骤描述（含重复）
}

func (r *execRecorder) log(desc string) { r.executed = append(r.executed, desc) }

func (r *execRecorder) count(desc string) int {
	n := 0
	for _, e := range r.executed {
		if e == desc {
			n++
		}
	}
	return n
}

// fakeExecutor 模拟外部执行器：遍历会话 Plan，跳过 CompletedIDs 中的步骤，
// 每执行一步记录并标记完成。maxSteps>0 时最多执行 maxSteps 步（模拟打断时机）。
// 返回本轮执行的步骤描述列表。
func fakeExecutor(ctx context.Context, ext *plantask.Extension, sid string, rec *execRecorder, maxSteps int) ([]string, error) {
	sess, err := ext.Session(ctx, sid)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(sess.CompletedIDs))
	for _, h := range sess.CompletedIDs {
		done[h] = true
	}
	var ran []string
	n := 0
	for _, step := range sess.Plan.Steps {
		if maxSteps > 0 && n >= maxSteps {
			break
		}
		if done[step.Hash] {
			continue // 已完成步骤不重跑
		}
		rec.log(step.Description)
		ran = append(ran, step.Description)
		sess.MarkCompleted(step.Hash)
		n++
	}
	return ran, ext.Persist(ctx, sess)
}

// buildPlantaskHarness 构造完整 HCL 测试环境（真实 bootstrap bridge + 内存存储）。
func buildPlantaskHarness(t *testing.T) (*plantask.Extension, *bootstrap.PlantaskBridge, *fakeGate) {
	t.Helper()
	gate := &fakeGate{}
	taskStore := tasklist.NewMemoryStore()
	bridge := bootstrap.NewPlantaskBridge(taskStore)
	// 确定性生成器：反馈含"加一步"时追加一步。
	bridge.GenerateSteps = func(_ context.Context, s *plantask.PlanTaskSession, feedback string) ([]plantask.StepSnapshot, error) {
		steps := s.Plan.Steps
		if !stringsContains(feedback, "加一步") {
			return steps, nil
		}
		next := plantask.StepSnapshot{
			Order:       len(steps) + 1,
			Strategy:    "chain",
			Description: "补充法律依据",
		}
		next.Hash = plantask.StepHash(next.Order, next.Strategy, next.Description)
		return append(append([]plantask.StepSnapshot(nil), steps...), next), nil
	}
	ext, err := plantask.NewExtension(plantask.Config{
		Store:        plantask.NewMemoryStore(),
		TaskStore:    taskStore,
		Gate:         gate,
		Replanner:    bridge,
		NewSessionID: func(caseID string) string { return caseID + "_sess" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext, bridge, gate
}

func stringsContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// fakeGate 是 planmode 门控的测试替身。
type fakeGate struct{ active bool }

func (g *fakeGate) Activate()      { g.active = true }
func (g *fakeGate) Deactivate()    { g.active = false }
func (g *fakeGate) IsActive() bool { return g.active }

// callTool 调用工具的 Func。
func callTool[TArgs, TResult any](t *testing.T, tool *agentcore.Tool, args TArgs) (TResult, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var zero TResult
	res, err := tool.Func(context.Background(), raw)
	if err != nil {
		return zero, err
	}
	switch r := res.(type) {
	case string:
		if err := json.Unmarshal([]byte(r), &zero); err != nil {
			t.Fatalf("unmarshal %q: %v", r, err)
		}
		return zero, nil
	default:
		out, ok := r.(TResult)
		if !ok {
			t.Fatalf("unexpected type %T", res)
		}
		return out, nil
	}
}

// TestPlantask_HCLFullLoop 验证完整人机协作闭环。
func TestPlantask_HCLFullLoop(t *testing.T) {
	ext, _, _ := buildPlantaskHarness(t)
	ctx := context.Background()
	rec := &execRecorder{}

	steps := []plantask.PlanStepInput{
		{Order: 1, Strategy: "chain", Description: "检索现有技术"},
		{Order: 2, Strategy: "chain", Description: "比对权利要求"},
		{Order: 3, Strategy: "chain", Description: "撰写分析结论"},
	}

	// ① 规划提交 → 等待批准。
	sub, err := callTool[plantask.PlanSubmitArgs, plantask.PlanSubmitResult](t, ext.Tools()[0], plantask.PlanSubmitArgs{
		CaseID: "case-hcl", CaseType: "invalidity",
		Steps: steps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != string(plantask.StatusAwaitingApproval) {
		t.Fatalf("expected awaiting_approval, got %s", sub.Status)
	}

	// ② 用户批准 → 执行。
	if _, err := callTool[plantask.PlanApproveArgs, plantask.PlanApproveResult](t, ext.Tools()[1], plantask.PlanApproveArgs{SessionID: sub.SessionID}); err != nil {
		t.Fatal(err)
	}

	// ③ 执行前两步（recorder 记录），第三步未执行。
	ran, err := fakeExecutor(ctx, ext, sub.SessionID, rec, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 2 {
		t.Fatalf("expected 2 steps executed, got %v", ran)
	}

	// ④ 用户打断。
	if _, err := callTool[plantask.WorkflowInterruptArgs, plantask.WorkflowInterruptResult](t, ext.Tools()[4], plantask.WorkflowInterruptArgs{
		SessionID: sub.SessionID,
		Reason:    "需要补充检索范围",
	}); !errors.Is(err, agentcore.ErrInterrupt) {
		t.Fatalf("expected interrupt, got %v", err)
	}

	// ⑤ 用户反馈：重跑步骤 1 + 新增一步。
	res, err := callTool[plantask.WorkflowFeedbackArgs, plantask.WorkflowFeedbackResult](t, ext.Tools()[6], plantask.WorkflowFeedbackArgs{
		SessionID: sub.SessionID,
		Feedback:  "检索范围太窄\n重跑:检索现有技术\n加一步",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(plantask.StatusExecuting) {
		t.Fatalf("expected executing after replan, got %s", res.Status)
	}

	// ⑥ 续跑：步骤 1 重跑、步骤 2 跳过、步骤 3 + 新步骤执行。
	ran2, err := fakeExecutor(ctx, ext, sub.SessionID, rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran2) != 3 {
		t.Fatalf("expected 3 steps executed in resume, got %v", ran2)
	}

	// 断言（recorder 执行日志）：
	//   - 步骤 1（显式重跑）执行 2 次
	//   - 步骤 2（未改动）执行 1 次 → 不重跑
	//   - 步骤 3 与新步骤各执行 1 次
	if got := rec.count("检索现有技术"); got != 2 {
		t.Errorf("步骤 1 应执行 2 次（显式重跑），实际 %d", got)
	}
	if got := rec.count("比对权利要求"); got != 1 {
		t.Errorf("步骤 2 应执行 1 次（不重跑），实际 %d", got)
	}
	if got := rec.count("撰写分析结论"); got != 1 {
		t.Errorf("步骤 3 应执行 1 次，实际 %d", got)
	}
	if got := rec.count("补充法律依据"); got != 1 {
		t.Errorf("新步骤应执行 1 次，实际 %d", got)
	}

	// ⑦ 会话最终状态可查询。
	sess, err := ext.Session(ctx, sub.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Plan.Steps) != 4 {
		t.Errorf("replan 后应有 4 步，实际 %d", len(sess.Plan.Steps))
	}
}
