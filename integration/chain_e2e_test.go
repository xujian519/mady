package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/workflows/legal"
	"github.com/xujian519/mady/workflows/patent"
)

// ──────────────────────────────────────────────
// 链路 3: 专利分析全链路
// ──────────────────────────────────────────────

func TestChainE2E_PatentNoveltyAnalysis(t *testing.T) {
	compiled, err := patent.BuildNoveltyGraphWithRules()
	if err != nil {
		t.Fatalf("BuildNoveltyGraphWithRules: %v", err)
	}

	input := "一种智能节水灌溉装置，其特征在于包括土壤湿度传感器、中央控制器和电磁阀；土壤湿度传感器采集土壤水分数据，中央控制器根据预设阈值判断是否需要灌溉，电磁阀根据控制信号开启或关闭。本发明解决了现有灌溉系统水资源浪费的问题，提高了灌溉效率。"
	state, err := compiled.Run(context.Background(), graph.PregelState{
		patent.StateInput: input,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := state.GetString(patent.StateOutput)
	if output == "" {
		t.Fatal("output is empty")
	}

	checks := []string{"技术特征", "初步结论", "AI 辅助生成"}
	for _, c := range checks {
		if !strings.Contains(output, c) {
			t.Errorf("output missing %q", c)
		}
	}

	ruleCheck := state.GetString(patent.StateRuleCheck)
	if ruleCheck == "" {
		t.Error("rule check report should not be empty")
	}

	t.Logf("Patent analysis output length: %d", len(output))
}

// ──────────────────────────────────────────────
// 链路 4: 法律比较分析全链路
// ──────────────────────────────────────────────

func TestChainE2E_LegalCaseComparison(t *testing.T) {
	compiled, bb, err := legal.BuildComparisonGraphWithReasoning(
		"case-e2e-001", reasoning.CaseInvalidation,
	)
	if err != nil {
		t.Fatalf("BuildComparisonGraphWithReasoning: %v", err)
	}
	if bb == nil {
		t.Fatal("FactBlackboard is nil")
	}

	facts := "本案涉及一项发明专利的侵权纠纷。原告主张被告未经许可制造并销售了侵犯其专利权 ZL2024101234567 的产品。被告抗辩称其产品使用了不同的技术方案，不构成侵权。争议焦点为被告产品是否落入原告专利权的保护范围。"
	state, err := compiled.Run(context.Background(), graph.PregelState{
		legal.StateCaseFacts: facts,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := state.GetString(legal.StateConclusion)
	if output == "" {
		output = state.GetString(legal.StateOutput)
	}
	if output == "" {
		t.Fatal("output is empty")
	}

	for _, want := range []string{"大前提", "小前提", "结论"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}

	chains := bb.ReasoningChains()
	if len(chains) == 0 {
		t.Error("FactBlackboard has no reasoning chains")
	}
	t.Logf("Legal comparison output length: %d, chains: %d", len(output), len(chains))
}

// ──────────────────────────────────────────────
// Session 连续性: 多轮对话状态保持
// ──────────────────────────────────────────────

func TestChainE2E_SessionContinuity(t *testing.T) {
	provider := &handoffE2EProvider{
		tool:    "chat-agent",
		content: "回复内容",
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "session-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	// 轮 1
	out1, err := agent.Run(context.Background(), "第一轮消息")
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if out1 == "" {
		t.Error("turn 1 output empty")
	}

	// 轮 2
	out2, err := agent.Run(context.Background(), "第二轮消息")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if out2 == "" {
		t.Error("turn 2 output empty")
	}

	// 验证状态中有多条消息（system + user1 + asst1 + user2 + asst2）
	msgs := agent.State().Messages()
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 messages after 2 turns, got %d", len(msgs))
	}

	t.Logf("Session: %d messages after 2 turns", len(msgs))
}
