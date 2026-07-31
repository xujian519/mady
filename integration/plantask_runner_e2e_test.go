//go:build integration

// PlanTask HCL × FiveStepRunner 真实链路端到端测试：
// replan 经 bootstrap.PlantaskBridge → reasoning.FiveStepRunner.GenerateReplanSteps
// （反馈注入 blackboard + 模板/KG/LLM 三路规划），再经 ReplanMerge 合并。
// 无 LLM 也成立：CaseDrafting 走 manifest 模板路径（确定性）。
package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/agentcore/tasklist"
	"github.com/xujian519/mady/bootstrap"
	"github.com/xujian519/mady/domains/reasoning"
)

// TestPlantask_RealRunnerReplan 验证真实 runner 规划链上的完整 HCL 闭环。
func TestPlantask_RealRunnerReplan(t *testing.T) {
	ctx := context.Background()

	// ① 真实五步推理引擎（无 retriever/无 LLM → manifest 模板路径，确定性）。
	runner := reasoning.NewWorkflowRunner("pt-e2e", reasoning.CaseDrafting, "", nil, nil)

	// ② 取规范步骤（与提交计划一致的基准：runner 模板计划的步骤）。
	canonical, err := runner.GenerateReplanSteps(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) < 3 {
		t.Fatalf("drafting manifest should yield >=3 steps, got %d", len(canonical))
	}

	// ③ plantask + bridge（runner 已接入，GenerateSteps 未覆盖）。
	taskStore := tasklist.NewMemoryStore()
	bridge := bootstrap.NewPlantaskBridge(taskStore)
	bridge.SetRunner(runner)
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

	// ④ 提交模板步骤（转换为 plan_submit 输入）。
	inputs := make([]plantask.PlanStepInput, len(canonical))
	for i, st := range canonical {
		inputs[i] = plantask.PlanStepInput{
			Order:       st.Order,
			Strategy:    string(st.Strategy),
			Description: st.Description,
		}
	}
	sub, err := callTool[plantask.PlanSubmitArgs, plantask.PlanSubmitResult](t, ext.Tools()[0], plantask.PlanSubmitArgs{
		CaseID: "case-runner", CaseType: "drafting", Steps: inputs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callTool[plantask.PlanApproveArgs, plantask.PlanApproveResult](t, ext.Tools()[1], plantask.PlanApproveArgs{SessionID: sub.SessionID}); err != nil {
		t.Fatal(err)
	}

	// ⑤ 执行前两步（recorder）。
	rec := &execRecorder{}
	if ran, err := fakeExecutor(ctx, ext, sub.SessionID, rec, 2); err != nil || len(ran) != 2 {
		t.Fatalf("executor phase 1: ran=%v err=%v", ran, err)
	}
	step1Desc := canonical[0].Description

	// ⑥ 打断 + 反馈（显式重跑步骤 1）。
	if _, err := callTool[plantask.WorkflowInterruptArgs, plantask.WorkflowInterruptResult](t, ext.Tools()[4], plantask.WorkflowInterruptArgs{
		SessionID: sub.SessionID,
	}); !errors.Is(err, agentcore.ErrInterrupt) {
		t.Fatalf("expected interrupt, got %v", err)
	}
	res, err := callTool[plantask.WorkflowFeedbackArgs, plantask.WorkflowFeedbackResult](t, ext.Tools()[6], plantask.WorkflowFeedbackArgs{
		SessionID: sub.SessionID,
		Feedback:  "第一步范围太窄\n重跑:" + step1Desc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(plantask.StatusExecuting) {
		t.Fatalf("expected executing after real-runner replan, got %s", res.Status)
	}

	// ⑦ 续跑：步骤 1 重跑，步骤 2 跳过，其余执行。
	if ran2, err := fakeExecutor(ctx, ext, sub.SessionID, rec, 0); err != nil {
		t.Fatal(err)
	} else if len(ran2) == 0 {
		t.Fatal("resume should execute pending steps")
	}

	// 断言：步骤 1 执行 2 次（重跑），步骤 2 只 1 次（不重跑）。
	if got := rec.count(step1Desc); got != 2 {
		t.Errorf("步骤 1（显式重跑）应执行 2 次，实际 %d", got)
	}
	if got := rec.count(canonical[1].Description); got != 1 {
		t.Errorf("步骤 2（未改动）应执行 1 次，实际 %d", got)
	}
	// 反馈已注入 runner blackboard（replan 后应可见用户反馈事实）。
	if !runnerHasFeedbackFact(t, runner) {
		t.Error("feedback fact not injected into runner blackboard")
	}
}

// runnerHasFeedbackFact 检查 runner blackboard 中是否存在用户反馈事实。
func runnerHasFeedbackFact(t *testing.T, runner *reasoning.FiveStepRunner) bool {
	t.Helper()
	return runner.FeedbackInjected()
}
