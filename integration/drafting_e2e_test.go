//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
)

// TestDrafting_WorkflowTool verifies that the CaseDrafting WorkflowManifest
// produces substantive output when run through AsWorkflowTool.
//
// This test validates:
//   - The drafting manifest loads correctly (5 stages, 5 plan steps)
//   - The LLMNodeBuilder produces real analysis text per step
//   - formatResult aggregates all step outputs into a coherent final result
func TestDrafting_WorkflowTool(t *testing.T) {
	mockLLM := &draftMockLlm{}

	runner := reasoning.NewWorkflowRunner(
		"test-draft-001",
		reasoning.CaseDrafting,
		"数据处理",
		nil, // retriever: nil → Stage ② skipped
		mockLLM,
	)
	if runner == nil {
		t.Fatal("NewWorkflowRunner returned nil")
	}

	tool := reasoning.AsWorkflowTool(runner)
	if tool == nil {
		t.Fatal("AsWorkflowTool returned nil")
	}
	if tool.Name != "run_five_step_workflow" {
		t.Errorf("expected tool name 'run_five_step_workflow', got %q", tool.Name)
	}

	result, err := tool.Func(context.Background(), mustJSON(t, map[string]string{
		"query":     "一种基于区块链的数据存证方法，其特征在于：步骤1：接收用户上传的电子文件；步骤2：计算文件哈希值；步骤3：将哈希值写入区块链；步骤4：返回存证凭证。",
		"case_type": "drafting",
	}))
	if err != nil {
		t.Fatalf("workflow tool execution failed: %v", err)
	}

	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T: %v", result, result)
	}

	// Verify result contains step analysis output.
	for _, want := range []string{"步骤1", "步骤2", "步骤3", "步骤4", "步骤5"} {
		if !strings.Contains(resultStr, want) {
			t.Errorf("result missing %q\ngot:\n%s", want, resultStr)
		}
	}
}

// TestDrafting_PatentAgentConfig verifies that PatentAgentConfig registers
// the run_five_step_workflow tool when SetupPatentDraftingEngine has been called.
//
// This test validates the Sprint 1.2 fix: the FiveStepRunner must be available
// in PatentAgentConfig's tool list so Router Handoff sub-agents can use it.
func TestDrafting_PatentAgentConfig(t *testing.T) {
	// Simulate the startup wiring: inject a drafting engine before creating
	// the Patent Agent config.
	retriever := buildTestRetriever(t)
	mockLLM := &draftMockLlm{}
	domains.SetupPatentDraftingEngine(retriever, mockLLM)

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "test",
			Model: "mock",
		},
		WorkspaceDir: t.TempDir(),
	}
	cfg := domains.PatentAgentConfig(base)

	// Verify that run_five_step_workflow appears in the tool list.
	found := false
	for _, tool := range cfg.Tools {
		if tool.Name == "run_five_step_workflow" {
			found = true
			break
		}
	}
	if !found {
		// Print all tool names for debugging.
		var names []string
		for _, tool := range cfg.Tools {
			names = append(names, tool.Name)
		}
		t.Fatalf("PatentAgentConfig missing run_five_step_workflow tool — Sprint 1.2 fix not applied\ntools: %v", names)
	}
}

// TestDrafting_NoComputerUse verifies that PatentAgentConfig does NOT include
// the computer_use tool (Sprint 2.1 fix: patent agents don't need desktop control).
func TestDrafting_NoComputerUse(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "test",
			Model: "mock",
		},
		WorkspaceDir: t.TempDir(),
	}
	cfg := domains.PatentAgentConfig(base)

	for _, tool := range cfg.Tools {
		if tool.Name == "computer_use" {
			t.Error("PatentAgentConfig should NOT include computer_use tool — Sprint 2.1 fix not applied")
		}
	}
}

// TestDrafting_ManifestSteps verifies the drafting manifest has 5 plan steps.
func TestDrafting_ManifestSteps(t *testing.T) {
	manifests := reasoning.DefaultManifests()
	found := false
	for _, m := range manifests {
		if m.ID == "patent_drafting_default" {
			found = true
			if len(m.Stage4.Steps) != 5 {
				t.Errorf("expected 5 plan steps, got %d", len(m.Stage4.Steps))
			}
			// Verify step descriptions mention drafting-specific terms.
			step1 := m.Stage4.Steps[0].Description
			if !strings.Contains(step1, "技术特征") {
				t.Errorf("step 1 should mention '技术特征'\ngot: %s", step1)
			}
			break
		}
	}
	if !found {
		t.Fatal("patent_drafting_default manifest not found in DefaultManifests()")
	}
}

// ───── helpers ─────

// draftMockLlm returns canned analysis for each turn to simulate a real LLM.
type draftMockLlm struct {
	turn int
}

func (m *draftMockLlm) Chat(_ context.Context, msgs []reasoning.LlmMessage) (string, error) {
	m.turn++
	replies := []string{
		"本发明的技术方案涉及区块链技术在数据存证领域的应用。",
		"根据专利法第22条第2款，本发明的技术方案具备新颖性。",
		"独立权利要求应涵盖：存证请求接收、哈希计算、上链存储、凭证返回四个必要特征。",
		"从属权利要求应限定：哈希算法类型、区块链网络类型、存证凭证格式。",
		"单一性分析：各权利要求之间具有相同或相应的特定技术特征，可以合案申请。",
		"特征对照表：权1与D1的区别在于区块链存证，授权概率高。",
	}
	idx := m.turn - 1
	if idx >= len(replies) {
		return "分析完成。", nil
	}
	return replies[idx], nil
}

// buildTestRetriever creates an empty retriever for testing.
func buildTestRetriever(t *testing.T) *reasoning.MultiSourceRetriever {
	t.Helper()
	// nil retriever is acceptable — Stage ② skips gracefully.
	return nil
}

// mustJSON serializes v to JSON bytes.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
