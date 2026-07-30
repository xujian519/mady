package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Mock LLM
// =============================================================================

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) call(_ context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// =============================================================================
// Mock Tool
// =============================================================================

func newEchoTool(name string) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        name,
		Description: "echo tool for testing",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			return fmt.Sprintf("echo: %s", string(args)), nil
		},
	}
}

// =============================================================================
// 测试 stateKeyFromPath
// =============================================================================

func TestStateKeyFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"data/cases/{caseId}/outputs/claims.md", "claims"},
		{"data/cases/{caseId}/outputs/*-cleaned.md", "cleaned"},
		{"data/cases/{caseId}/disclosure/*.{md,txt,pdf}", "disclosure"},
		{"search-request.md", "search_request"},
		{"state:custom_key", "custom_key"},
		{"", "_fallback"},
	}
	for _, tt := range tests {
		got := stateKeyFromPath(tt.path)
		if got != tt.want {
			t.Errorf("stateKeyFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// =============================================================================
// 测试 LLM Executor
// =============================================================================

func TestLLMExecutor_ToPregelNode(t *testing.T) {
	llm := &mockLLM{response: "分析完成：具有新颖性"}
	def := &Definition{
		Name:        "test-llm-worker",
		Tier:        TierReasoning,
		Description: "测试 LLM Worker：分析技术方案的新颖性",
		Inputs:      []Input{{Path: "data/cases/{caseId}/outputs/claims.md"}},
		Outputs:     []Output{{Path: "data/cases/{caseId}/outputs/novelty-analysis.md", ContractLevel: ContractHard}},
	}

	exec := NewLLMExecutor(def, llm.call)
	node := exec.ToPregelNode()

	state := graph.PregelState{
		"claims": "一种智能终端，其特征在于包括处理器和存储器",
	}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("LLM Executor 执行返回错误: %v", err)
	}

	// 校验输出已写入 state
	out := result.GetString("novelty_analysis")
	if out == "" {
		t.Fatal("LLM Executor 未写入输出 novelty_analysis")
	}
	if !strings.Contains(out, "分析完成") {
		t.Errorf("输出内容不匹配: got %q", out)
	}

	// 不应有降解标记
	if graph.HasDegradation(result) {
		t.Errorf("不应有降解标记，但发现: %+v", graph.DegradationSummary(result))
	}
}

func TestLLMExecutor_MissingInput_Degraded(t *testing.T) {
	llm := &mockLLM{response: "分析结果"}
	def := &Definition{
		Name:        "test-worker-input-missing",
		Tier:        TierWork,
		Description: "测试输入缺失降级",
		Inputs:      []Input{{Path: "data/cases/{caseId}/disclosure/disclosure.md", Optional: false}},
		Outputs:     []Output{{Path: "data/cases/{caseId}/outputs/analysis.md", ContractLevel: ContractHard}},
	}

	exec := NewLLMExecutor(def, llm.call)
	node := exec.ToPregelNode()

	// state 中不提供 disclosure key
	state := graph.PregelState{}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("输入缺失不应返回 error，应写入 degradation: %v", err)
	}

	if !graph.HasDegradation(result) {
		t.Error("输入缺失时应产生降解标记")
	}

	// LLM 仍应执行，输出应写入
	out := result.GetString("analysis")
	if out == "" {
		t.Error("即使输入缺失，输出也应写入")
	}
}

func TestLLMExecutor_NilLLM(t *testing.T) {
	def := &Definition{
		Name:        "test-nil-llm",
		Tier:        TierChecker,
		Description: "nil LLM test",
		Outputs:     []Output{{Path: "output.md", ContractLevel: ContractSoft}},
	}

	exec := NewLLMExecutor(def, nil)
	node := exec.ToPregelNode()

	state := graph.PregelState{}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("nil LLM 不应返回 error: %v", err)
	}

	// 应产生 critical degradation
	if !graph.HasDegradation(result) {
		t.Error("nil LLM 时应产生降解标记")
	}
	marks := graph.DegradationSummary(result)
	found := false
	for _, m := range marks {
		if m.Severity == "critical" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("nil LLM 应产生 critical 降解，但得到: %+v", marks)
	}
}

// =============================================================================
// 测试 Graph Executor
// =============================================================================

func TestGraphExecutor_ToPregelNode(t *testing.T) {
	// 构建一个简单的子图：输入 → 大写转换 → 输出
	subGraph := graph.NewPregelGraph()
	_ = subGraph.AddNode("upper", func(_ context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := state.GetString("input")
		return graph.PregelState{"output": strings.ToUpper(input)}, nil
	})
	_ = subGraph.AddEdge("upper", graph.PregelEnd)

	compiled, err := subGraph.Compile("upper", 5)
	if err != nil {
		t.Fatalf("编译子图失败: %v", err)
	}

	def := &Definition{
		Name:        "test-graph-worker",
		Tier:        TierWork,
		Description: "测试 Graph Worker",
		Outputs:     []Output{{Path: "data/cases/{caseId}/outputs/result.md"}},
	}

	exec := NewGraphExecutor(def, compiled)
	node := exec.ToPregelNode()

	state := graph.PregelState{
		"input": "hello world",
	}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("Graph Executor 执行返回 error: %v", err)
	}

	out := result.GetString("result")
	if out != "HELLO WORLD" {
		t.Errorf("Graph Executor 输出 = %q, want %q", out, "HELLO WORLD")
	}
}

func TestGraphExecutor_NilGraph(t *testing.T) {
	def := &Definition{
		Name:    "test-nil-graph",
		Tier:    TierDomain,
		Outputs: []Output{{Path: "out.md"}},
	}

	exec := NewGraphExecutor(def, nil)
	node := exec.ToPregelNode()

	state := graph.PregelState{}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatal("nil 子图不应返回 error")
	}
	if !graph.HasDegradation(result) {
		t.Error("nil 子图时应产生降解标记")
	}
}

// =============================================================================
// 测试 Tool Executor
// =============================================================================

func TestToolExecutor_ToPregelNode(t *testing.T) {
	tool := newEchoTool("test-tool-worker")
	def := &Definition{
		Name:        "test-tool-worker",
		Tier:        TierWork,
		Description: "测试 Tool Worker",
		Inputs:      []Input{{Path: "query_text"}},
		Outputs:     []Output{{Path: "data/cases/{caseId}/outputs/result.md"}},
	}

	exec := NewToolExecutor(def, tool)
	node := exec.ToPregelNode()

	state := graph.PregelState{
		"query_text": "新颖性分析",
	}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("Tool Executor 执行返回 error: %v", err)
	}

	out := result.GetString("result")
	if out == "" {
		t.Fatal("Tool Executor 未写入输出")
	}
	if !strings.Contains(out, "新颖性分析") {
		t.Errorf("Tool Executor 输出不包含输入: %q", out)
	}
}

func TestToolExecutor_NilTool(t *testing.T) {
	def := &Definition{
		Name:    "test-nil-tool",
		Tier:    TierChecker,
		Outputs: []Output{{Path: "out.md"}},
	}

	exec := NewToolExecutor(def, nil)
	node := exec.ToPregelNode()

	state := graph.PregelState{}
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatal("nil tool 不应返回 error")
	}
	if !graph.HasDegradation(result) {
		t.Error("nil tool 时应产生降解标记")
	}
}

// =============================================================================
// 测试契约校验
// =============================================================================

func TestValidateInputs_AllPresent(t *testing.T) {
	def := &Definition{
		Name: "test",
		Inputs: []Input{
			{Path: "claims.md"},
			{Path: "description.md"},
		},
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{
		"claims":      "权利要求内容",
		"description": "说明书内容",
	}
	issues := exec.ValidateInputs(state)
	if len(issues) != 0 {
		t.Errorf("所有输入都存在，不应有 issue: %+v", issues)
	}
}

func TestValidateInputs_Missing(t *testing.T) {
	def := &Definition{
		Name: "test",
		Inputs: []Input{
			{Path: "claims.md"},
			{Path: "optional-file.md", Optional: true},
			{Path: "description.md"},
		},
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{
		"claims": "权利要求内容",
	}
	issues := exec.ValidateInputs(state)
	if len(issues) != 1 {
		t.Fatalf("期望 1 个 input issue，得到 %d: %+v", len(issues), issues)
	}
	if issues[0].Key != "description" {
		t.Errorf("缺失的 key 应为 description，得到 %q", issues[0].Key)
	}
}

func TestValidateOutputs_AllPresent(t *testing.T) {
	def := &Definition{
		Name: "test",
		Outputs: []Output{
			{Path: "result.md", ContractLevel: ContractHard},
			{Path: "report.md", ContractLevel: ContractSoft},
		},
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{
		"result": "分析结果",
		"report": "报告内容",
	}
	issues := exec.ValidateOutputs(state)
	if len(issues) != 0 {
		t.Errorf("所有输出都存在，不应有 issue: %+v", issues)
	}
}

func TestValidateOutputs_MissingHard(t *testing.T) {
	def := &Definition{
		Name: "test",
		Outputs: []Output{
			{Path: "result.md", ContractLevel: ContractHard},
			{Path: "report.md", ContractLevel: ContractSoft},
		},
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{
		"result": "分析结果",
		// report 缺失
	}
	issues := exec.ValidateOutputs(state)
	if len(issues) != 1 {
		t.Fatalf("期望 1 个 output issue，得到 %d: %+v", len(issues), issues)
	}
	if issues[0].Key != "report" {
		t.Errorf("缺失的 key 应为 report，得到 %q", issues[0].Key)
	}
	if issues[0].Level != "soft" {
		t.Errorf("report 的 ContractLevel 应为 soft，得到 %q", issues[0].Level)
	}
}

// =============================================================================
// 测试写入输出
// =============================================================================

func TestWriteOutputs_Single(t *testing.T) {
	def := &Definition{
		Name:    "test-writer",
		Outputs: []Output{{Path: "output.md"}},
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{}
	exec.writeOutputs(state, "分析结果文本")
	if state["output"] != "分析结果文本" {
		t.Errorf("writeOutputs: state[output] = %q, want %q", state["output"], "分析结果文本")
	}
}

func TestWriteOutputs_Multiple(t *testing.T) {
	def := &Definition{
		Name: "test-writer-multi",
		Outputs: []Output{
			{Path: "full.md", ContractLevel: ContractHard},
			{Path: "summary.md", ContractLevel: ContractSoft},
		},
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{}
	longText := strings.Repeat("A", 500)
	exec.writeOutputs(state, longText)

	if state["full"] != longText {
		t.Error("首个 output 应写入完整文本")
	}
	summary, ok := state["summary"].(string)
	if !ok || len(summary) > 210 {
		t.Errorf("后续 output 应截断 <= 200 + …, 得到 %d 字符", len(summary))
	}
}

func TestWriteOutputs_NoOutputs(t *testing.T) {
	def := &Definition{
		Name: "test-no-output-contract",
	}
	exec := NewLLMExecutor(def, nil)

	state := graph.PregelState{}
	exec.writeOutputs(state, "结果")
	if state["test-no-output-contract"] != "结果" {
		t.Error("无输出契约时应写入 worker.Name 作为 key")
	}
}

// =============================================================================
// 测试多轮调用
// =============================================================================

func TestExecutor_SequentialChain(t *testing.T) {
	// 模拟 Pipeline 中的前后串联：Worker1 → state → Worker2
	llm1 := &mockLLM{response: "技术特征提取完成"}
	llm2 := &mockLLM{response: "创造性分析完成"}

	def1 := &Definition{
		Name:    "technical-analyzer",
		Tier:    TierWork,
		Inputs:  []Input{{Path: "disclosure.md"}},
		Outputs: []Output{{Path: "features.md"}},
	}
	def2 := &Definition{
		Name:    "inventiveness-analyzer",
		Tier:    TierReasoning,
		Inputs:  []Input{{Path: "features.md"}},
		Outputs: []Output{{Path: "conclusion.md"}},
	}

	node1 := NewLLMExecutor(def1, llm1.call).ToPregelNode()
	node2 := NewLLMExecutor(def2, llm2.call).ToPregelNode()

	ctx := context.Background()
	state := graph.PregelState{"disclosure": "一种新型装置..."}

	// Worker1 → Worker2
	state, err := node1(ctx, state)
	if err != nil {
		t.Fatalf("Worker1 执行失败: %v", err)
	}
	if graph.HasDegradation(state) {
		t.Fatalf("Worker1 不应降级: %+v", graph.DegradationSummary(state))
	}

	state, err = node2(ctx, state)
	if err != nil {
		t.Fatalf("Worker2 执行失败: %v", err)
	}
	if graph.HasDegradation(state) {
		t.Fatalf("Worker2 不应降级: %+v", graph.DegradationSummary(state))
	}

	if state.GetString("conclusion") == "" {
		t.Error("最终输出 conclusion 应为空")
	}
}

// =============================================================================
// 测试 Monitor 集成
// =============================================================================

func TestExecutor_WithMonitor(t *testing.T) {
	llm := &mockLLM{response: "monitored result"}
	def := &Definition{
		Name:        "monitored-worker",
		Tier:        TierWork,
		Description: "被监控的 Worker",
		Outputs:     []Output{{Path: "out.md"}},
	}

	mon := NewMonitor()
	exec := NewLLMExecutor(def, llm.call).WithMonitor(mon)
	node := exec.ToPregelNode()

	state := graph.PregelState{}
	_, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// Monitor 应包含记录
	records := mon.Records()
	if len(records) != 1 {
		t.Fatalf("期望 1 条记录, 得到 %d", len(records))
	}
	rec := records[0]
	if rec.WorkerName != "monitored-worker" {
		t.Errorf("WorkerName = %q", rec.WorkerName)
	}
	if !rec.Success {
		t.Error("执行应成功")
	}
	if rec.Duration <= 0 {
		t.Error("Duration 应 > 0")
	}
}

func TestExecutor_WithMonitor_Failure(t *testing.T) {
	def := &Definition{
		Name:    "failing-worker",
		Tier:    TierChecker,
		Outputs: []Output{{Path: "out.md"}},
	}

	mon := NewMonitor()
	exec := NewLLMExecutor(def, nil).WithMonitor(mon) // nil llmFn → 返回 error
	node := exec.ToPregelNode()

	state := graph.PregelState{}
	_, err := node(context.Background(), state)
	if err != nil {
		t.Fatal("nil llm 不应返回 error（应降级）")
	}

	records := mon.Records()
	if len(records) != 1 {
		t.Fatalf("期望 1 条记录, 得到 %d", len(records))
	}
	rec := records[0]
	if rec.Success {
		t.Error("执行应标记为失败")
	}
	if !rec.Degraded {
		t.Error("应标记为降级")
	}
}

func TestExecutor_WithoutMonitor(t *testing.T) {
	// 不接入 Monitor 不应 panic
	llm := &mockLLM{response: "no monitor"}
	def := &Definition{
		Name:    "no-monitor-worker",
		Tier:    TierWork,
		Outputs: []Output{{Path: "out.md"}},
	}

	exec := NewLLMExecutor(def, llm.call) // 未调用 WithMonitor
	node := exec.ToPregelNode()

	state := graph.PregelState{}
	_, err := node(context.Background(), state)
	if err != nil {
		t.Fatal("未接入 Monitor 不应影响执行")
	}
}
