package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// 测试 AsTool
// =============================================================================

func TestAsTool_LLMWorker(t *testing.T) {
	llm := &mockLLM{response: "新颖性分析完成"}
	def := &Definition{
		Name:         "test-as-tool-llm",
		Tier:         TierReasoning,
		Description:  "测试 AsTool 的 LLM Worker",
		Inputs:       []Input{{Path: "claims.md"}},
		Outputs:      []Output{{Path: "output.md", ContractLevel: ContractHard}},
		TriggersHITL: true,
	}

	exec := NewLLMExecutor(def, llm.call)
	tool := AsTool(exec)

	// 校验 Tool 元数据
	if tool.Name != "test-as-tool-llm" {
		t.Errorf("Tool.Name = %q", tool.Name)
	}
	if tool.Description != "测试 AsTool 的 LLM Worker" {
		t.Errorf("Tool.Description = %q", tool.Description)
	}
	if tool.ReadOnly {
		t.Error("Worker 默认应非只读")
	}

	// 调用 Tool
	args, _ := json.Marshal(map[string]any{
		"claims": "一种智能终端，包括处理器和存储器",
	})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Tool.Func 返回 error: %v", err)
	}

	hr, ok := result.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("Tool.Func 返回类型 = %T, want HandoffResult", result)
	}
	if !hr.Success {
		t.Errorf("HandoffResult.Success = false, want true")
	}
	if !strings.Contains(hr.Result, "新颖性分析完成") {
		t.Errorf("HandoffResult.Result = %q, 应包含 LLM 响应", hr.Result)
	}
	if !strings.Contains(hr.Action, "test-as-tool-llm") {
		t.Errorf("HandoffResult.Action = %q, 应包含 Worker 名", hr.Action)
	}
}

func TestAsTool_GraphWorker(t *testing.T) {
	// 构建测试子图：输入大写输出
	g := graph.NewPregelGraph()
	_ = g.AddNode("upper", func(_ context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := state.GetString("input")
		return graph.PregelState{"output": strings.ToUpper(input)}, nil
	})
	_ = g.AddEdge("upper", graph.PregelEnd)
	compiled, err := g.Compile("upper", 5)
	if err != nil {
		t.Fatalf("编译子图失败: %v", err)
	}

	def := &Definition{
		Name:        "test-as-tool-graph",
		Tier:        TierWork,
		Description: "测试 AsTool 的 Graph Worker",
		Outputs:     []Output{{Path: "output.md", ContractLevel: ContractHard}},
	}

	exec := NewGraphExecutor(def, compiled)
	tool := AsTool(exec)

	args, _ := json.Marshal(map[string]any{
		"input": "hello world",
	})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Tool.Func error: %v", err)
	}

	hr, ok := result.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("Tool.Func 返回类型 = %T", result)
	}
	if !strings.Contains(hr.Result, "HELLO WORLD") {
		t.Errorf("输出应包含大写转换结果, got %q", hr.Result)
	}
}

func TestAsTool_ToolWorker(t *testing.T) {
	toolImpl := newEchoTool("test-as-tool-wrapper")
	def := &Definition{
		Name:        "test-as-tool-wrapper",
		Tier:        TierDomain,
		Description: "测试 AsTool 的 Tool Worker",
		Inputs:      []Input{{Path: "query_text"}},
		Outputs:     []Output{{Path: "output.md"}},
	}

	exec := NewToolExecutor(def, toolImpl)
	tool := AsTool(exec)

	args, _ := json.Marshal(map[string]any{
		"query_text": "hello tool",
	})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Tool.Func error: %v", err)
	}

	hr, ok := result.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("Tool.Func 返回类型 = %T", result)
	}
	if !strings.Contains(hr.Result, "hello tool") {
		t.Errorf("输出应包含输入参数, got %q", hr.Result)
	}
}

func TestAsTool_EmptyArgs(t *testing.T) {
	llm := &mockLLM{response: "结果"}
	def := &Definition{
		Name:        "test-empty-args",
		Tier:        TierChecker,
		Description: "空参数测试",
		Inputs:      []Input{},
		Outputs:     []Output{{Path: "output.md"}},
	}

	exec := NewLLMExecutor(def, llm.call)
	tool := AsTool(exec)

	// 空参数
	result, err := tool.Func(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("Tool.Func error: %v", err)
	}

	hr, ok := result.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("Tool.Func 返回类型 = %T", result)
	}
	if !strings.Contains(hr.Result, "结果") {
		t.Errorf("输出不正确: %q", hr.Result)
	}
}

func TestAsTool_InvalidJSON(t *testing.T) {
	llm := &mockLLM{response: "结果"}
	def := &Definition{
		Name:        "test-invalid-json",
		Tier:        TierWork,
		Description: "非法 JSON 测试",
		Outputs:     []Output{{Path: "output.md"}},
	}

	exec := NewLLMExecutor(def, llm.call)
	tool := AsTool(exec)

	// 非法 JSON
	result, err := tool.Func(context.Background(), json.RawMessage("{invalid}"))
	if err != nil {
		t.Fatalf("Tool.Func should not return raw error: %v", err)
	}

	hr, ok := result.(agentcore.HandoffResult)
	if !ok {
		t.Fatalf("Tool.Func 返回类型 = %T", result)
	}
	if hr.Success {
		t.Errorf("非法 JSON 时应返回失败 HandoffResult, Success=%v", hr.Success)
	}
	if !strings.Contains(hr.Action, "参数解析失败") {
		t.Errorf("Action 应含'参数解析失败', got %q", hr.Action)
	}
}

func TestAsTool_HITLFlag(t *testing.T) {
	tests := []struct {
		name         string
		triggersHITL bool
		wantReadOnly bool
	}{
		{"HITL-enabled", true, false},
		{"HITL-disabled", false, false}, // Worker 默认非只读
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &Definition{
				Name:         "test-hitl-" + tt.name,
				Tier:         TierWork,
				Description:  "HITL 测试",
				TriggersHITL: tt.triggersHITL,
			}
			exec := NewLLMExecutor(def, func(_ context.Context, _ string) (string, error) { return "ok", nil })
			tool := AsTool(exec)
			if tool.ReadOnly != tt.wantReadOnly {
				t.Errorf("ReadOnly = %v, want %v", tool.ReadOnly, tt.wantReadOnly)
			}
		})
	}
}

// =============================================================================
// 测试 Tool 注册到 agentcore.Registry
// =============================================================================

func TestAsTool_RegistryIntegration(t *testing.T) {
	llm := &mockLLM{response: "registry test result"}
	def := &Definition{
		Name:        "registry-test-worker",
		Tier:        TierWork,
		Description: "Registry 集成测试",
		Inputs:      []Input{{Path: "query"}},
		Outputs:     []Output{{Path: "output.md", ContractLevel: ContractHard}},
	}

	exec := NewLLMExecutor(def, llm.call)
	tool := AsTool(exec)

	// 注册到 agentcore.Registry
	reg := agentcore.NewRegistry()
	reg.Register(tool)

	// 通过 Registry 获取并调用
	registered, ok := reg.Get("registry-test-worker")
	if !ok {
		t.Fatal("Registry.Get 返回 nil")
	}
	if registered.Name != tool.Name {
		t.Errorf("名称不匹配: %q vs %q", registered.Name, tool.Name)
	}

	// 验证 Definitions 排序包含该工具
	defs := reg.Definitions()
	found := false
	for _, d := range defs {
		if d.Name == "registry-test-worker" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Registry.Definitions 中未找到注册的工具")
	}
}

// =============================================================================
// 测试 Catalog 中新增的 Worker 定义
// =============================================================================

func TestDefaultWorkers_NewEntries(t *testing.T) {
	catalog := NewCatalog()
	for _, d := range DefaultWorkers() {
		if err := catalog.Register(d); err != nil {
			t.Errorf("注册 Worker %q 失败: %v", d.Name, err)
		}
	}

	issues := catalog.Verify()
	for _, iss := range issues {
		t.Errorf("验证问题: %s", iss)
	}

	// 验证新增的 Worker 存在
	expectedNew := []string{
		"patent-infringement-analyzer",
		"patent-invalidation-analyzer",
		"patent-debate-simulator",
		"patent-reexamination-drafter",
		"legal-case-comparator",
		"patent-claim-formality-checker",
		"patent-search-commander",
	}
	for _, name := range expectedNew {
		if catalog.Get(name) == nil {
			t.Errorf("新增 Worker %q 未在 DefaultWorkers 中找到", name)
		}
	}

	// 验证总数
	all := catalog.List()
	t.Logf("总 Worker 数: %d", len(all))
	if len(all) != 18 {
		t.Errorf("期望 18 个 Worker（11 原始 + 6 新增 + patent-search-commander），得到 %d", len(all))
	}

	// 按 Tier 列出
	for _, tier := range []WorkerTier{TierWork, TierProvision, TierReasoning, TierDomain, TierChecker} {
		workers := catalog.ListByTier(tier)
		t.Logf("  [%s] %d 个 Worker", tier, len(workers))
	}
}
