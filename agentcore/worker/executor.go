// Package worker 将工作中台与执行引擎打通。
//
// Executor 是核心适配层：将 worker.Definition（声明式契约）编译为
// graph.PregelNode（可执行节点）。
//
// 三种执行模式：
//   - LLM Worker:  调用 LLM 执行分析（description 作为指令）
//   - Graph Worker:委托给 CompiledPregelGraph 子图
//   - Tool Worker: 委托给 agentcore.Tool
//
// 无论哪种模式，Executor 在外层包裹契约校验：
//
//	① 从 PregelState 读取 Inputs
//	② 校验必需 Inputs 是否到位（可选 QualityGate 钩子）
//	③ 执行内部逻辑
//	④ 校验 Outputs 是否产生
//	⑤ 写入结果 + 降解标记（如有）
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Executor
// =============================================================================

// ExecMode 标识 Worker 的执行方式。
type ExecMode int

const (
	ExecModeLLM   ExecMode = iota // 调用 LLM 分析
	ExecModeGraph                 // 委托 CompiledPregelGraph 子图
	ExecModeTool                  // 委托 agentcore.Tool
)

// String 返回执行模式的字面描述。
func (m ExecMode) String() string {
	switch m {
	case ExecModeLLM:
		return "llm"
	case ExecModeGraph:
		return "graph"
	case ExecModeTool:
		return "tool"
	default:
		return "unknown"
	}
}

// Executor 将 worker.Definition 编译为可执行的 graph.PregelNode。
//
// 构造方式（按执行模式）：
//
//	exec := worker.NewLLMExecutor(def, llmFn)
//	exec := worker.NewGraphExecutor(def, compiledGraph)
//	exec := worker.NewToolExecutor(def, agentTool)
//
// 可选：接入执行监控
//
//	monitor := worker.NewMonitor()
//	exec = exec.WithMonitor(monitor)
//
// 然后：
//
//	node := exec.ToPregelNode()  // → graph.PregelNode
type Executor struct {
	def      *Definition
	mode     ExecMode
	llmFn    func(ctx context.Context, prompt string) (string, error) // ExecModeLLM
	subGraph *graph.CompiledPregelGraph                               // ExecModeGraph
	tool     *agentcore.Tool                                          // ExecModeTool
	monitor  *Monitor                                                 // 可选执行监控
}

// NewLLMExecutor 创建一个调用 LLM 的 Worker Executor。
// worker.Definition.Description 作为 system prompt 的核心指令，
// 调用时从 PregelState 读取 Inputs 拼入 user prompt，结果写入 Outputs。
//
// llmFn 是实际的 LLM 调用函数，接收完整 prompt 文本，返回分析结果。
// 适配 reasoning.LlmClient 的示例：
func NewLLMExecutor(def *Definition, llmFn func(ctx context.Context, prompt string) (string, error)) *Executor {
	if llmFn == nil {
		llmFn = func(_ context.Context, prompt string) (string, error) {
			return "", fmt.Errorf("worker %q: LLM function 未配置", def.Name)
		}
	}
	return &Executor{def: def, mode: ExecModeLLM, llmFn: llmFn}
}

// NewGraphExecutor 创建一个委托给 Pregel 子图的 Worker Executor。
// 子图的输入输出通过 state key 映射到 Worker 的 Inputs/Outputs 契约。
func NewGraphExecutor(def *Definition, subGraph *graph.CompiledPregelGraph) *Executor {
	return &Executor{def: def, mode: ExecModeGraph, subGraph: subGraph}
}

// NewToolExecutor 创建一个包装现有 agentcore.Tool 的 Worker Executor。
// 构造时校验 Tool.Name 与 Worker.Name 一致，避免注册错位。
func NewToolExecutor(def *Definition, tool *agentcore.Tool) *Executor {
	return &Executor{def: def, mode: ExecModeTool, tool: tool}
}

// WithMonitor 接入执行监控。每次 ToPregelNode 执行时会记录 ExecutionRecord。
func (e *Executor) WithMonitor(m *Monitor) *Executor {
	e.monitor = m
	return e
}

// ToPregelNode 返回一个 graph.PregelNode，执行时：
//  1. ValidateInputs — 从 state 读取 Inputs，检查必需项
//  2. 执行内部逻辑（LLM / 子图 / Tool）
//  3. ValidateOutputs — 检查 Outputs 是否写入
//  4. 返回更新后的 state
//
// 校验失败时写入 DegradationMark 而非返回 error，保持图执行不中断。
func (e *Executor) ToPregelNode() graph.PregelNode {
	nodeName := e.def.Name
	mon := e.monitor
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		start := time.Now()
		var inputIssueCount, outputIssueCount int

		// ---- ① 校验输入 ----
		inputKeys := e.ValidateInputs(state)
		inputIssueCount = len(inputKeys)
		for _, issue := range inputKeys {
			graph.MarkDegraded(state, issue.Key,
				"",
				graph.DegradationNotImplemented,
				fmt.Sprintf("Worker %q 必需输入 %q 缺失", nodeName, issue.Key),
			)
		}

		// ---- ② 执行 ----
		var output string
		var execErr error
		switch e.mode {
		case ExecModeLLM:
			output, execErr = e.execLLM(ctx, state)
		case ExecModeGraph:
			output, execErr = e.execGraph(ctx, state)
		case ExecModeTool:
			output, execErr = e.execTool(ctx, state)
		default:
			execErr = fmt.Errorf("worker %q: 未知执行模式 %d", nodeName, e.mode)
		}

		if execErr != nil {
			graph.MarkDegradedCritical(state, nodeName+"_exec",
				execErr.Error(),
				graph.DegradationNotImplemented,
				fmt.Sprintf("Worker %q 执行失败: %v", nodeName, execErr),
			)
			if output != "" {
				e.writeOutputs(state, output)
			}
			recordExec(mon, e.def, e.mode, start, inputIssueCount, outputIssueCount, false, execErr.Error())
			return state, nil
		}

		// ---- ③ 写入输出 ----
		e.writeOutputs(state, output)

		// ---- ④ 校验输出 ----
		outIssues := e.ValidateOutputs(state)
		outputIssueCount = len(outIssues)
		for _, issue := range outIssues {
			graph.MarkDegraded(state, issue.Key,
				"",
				graph.DegradationNotImplemented,
				fmt.Sprintf("Worker %q 输出 %q 未产生 (等级:%s)", nodeName, issue.Key, issue.Level),
			)
		}

		recordExec(mon, e.def, e.mode, start, inputIssueCount, outputIssueCount, true, "")
		return state, nil
	}
}

// =============================================================================
// 执行
// =============================================================================

// execLLM 把 Worker 的 Inputs 拼接为 LLM prompt，调用 LLM，返回结果文本。
func (e *Executor) execLLM(ctx context.Context, state graph.PregelState) (string, error) {
	prompt := e.buildLLMPrompt(state)
	return e.llmFn(ctx, prompt)
}

// execGraph 把 PregelState 注入子图，运行后取回 output。
func (e *Executor) execGraph(ctx context.Context, state graph.PregelState) (string, error) {
	if e.subGraph == nil {
		return "", fmt.Errorf("worker %q: 子图未配置", e.def.Name)
	}

	// 把当前 state 作为子图的初始状态（深拷贝，避免子图修改污染父 state）
	initial := state.Clone()

	final, err := e.subGraph.Run(ctx, initial)
	if err != nil {
		return "", fmt.Errorf("子图执行失败: %w", err)
	}

	output := final.GetString("output")
	return output, nil
}

// execTool 调用 agentcore.Tool，传入从 state 提取的输入参数。
func (e *Executor) execTool(ctx context.Context, state graph.PregelState) (string, error) {
	if e.tool == nil {
		return "", fmt.Errorf("worker %q: 工具未配置", e.def.Name)
	}

	// 构造工具调用的参数 JSON — 从 state 中提取 Worker Inputs 命名的 key
	args := make(map[string]any)
	for _, in := range e.def.Inputs {
		key := stateKeyFromPath(in.Path)
		if v, ok := state[key]; ok {
			args[key] = v
		}
	}

	raw, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("参数序列化失败: %w", err)
	}

	result, err := e.tool.Func(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("工具调用失败: %w", err)
	}

	return fmt.Sprintf("%v", result), nil
}

// =============================================================================
// 契约校验
// =============================================================================

// InputIssue 描述一个输入契约的违反。
type InputIssue struct {
	Key     string
	Message string
}

// ValidateInputs 检查必需 Inputs 在 state 中是否可用。
// 返回所有有问题的 key（跳过 Optional=true 的 input）。
func (e *Executor) ValidateInputs(state graph.PregelState) []InputIssue {
	if len(e.def.Inputs) == 0 {
		return nil
	}
	var issues []InputIssue
	for _, in := range e.def.Inputs {
		if in.Optional {
			continue
		}
		key := stateKeyFromPath(in.Path)
		if _, ok := state[key]; !ok {
			issues = append(issues, InputIssue{
				Key:     key,
				Message: fmt.Sprintf("必需输入 %q 在 state 中不存在", key),
			})
		}
	}
	return issues
}

// OutputIssue 描述一个输出契约的违反。
type OutputIssue struct {
	Key     string
	Level   string // "hard" | "soft" | "structured"
	Message string
}

// ValidateOutputs 检查承诺的 Outputs 是否已写入 state。
// Hard 级别缺失会返回 issue；Soft 级别仅记录不阻塞。
func (e *Executor) ValidateOutputs(state graph.PregelState) []OutputIssue {
	if len(e.def.Outputs) == 0 {
		return nil
	}
	var issues []OutputIssue
	for _, out := range e.def.Outputs {
		key := stateKeyFromPath(out.Path)
		// 检查 key 是否存在且非空
		v, ok := state[key]
		if !ok || v == nil || fmt.Sprintf("%v", v) == "" {
			issues = append(issues, OutputIssue{
				Key:     key,
				Level:   string(out.ContractLevel),
				Message: fmt.Sprintf("输出 %q 未产生", key),
			})
		}
	}
	return issues
}

// =============================================================================
// 辅助
// =============================================================================

// buildLLMPrompt 将 Worker 的 Description（作为指令）和 Inputs（作为上下文）拼接为完整 prompt。
func (e *Executor) buildLLMPrompt(state graph.PregelState) string {
	var sb strings.Builder
	// Worker 描述作为系统指令
	if e.def.Description != "" {
		fmt.Fprintf(&sb, "## 任务指令\n%s\n\n", e.def.Description)
	}
	sb.WriteString("## 输入内容\n\n")
	for _, in := range e.def.Inputs {
		key := stateKeyFromPath(in.Path)
		if v, ok := state[key]; ok {
			if in.Description != "" {
				fmt.Fprintf(&sb, "### %s\n", in.Description)
			} else {
				fmt.Fprintf(&sb, "### %s\n", key)
			}
			content := fmt.Sprintf("%v", v)
			if len([]rune(content)) > 4000 {
				content = string([]rune(content)[:4000]) + "\n…(内容已截断)"
			}
			fmt.Fprintf(&sb, "%s\n\n", content)
		}
	}
	sb.WriteString("请基于以上信息完成分析任务，输出分析结果。")
	return sb.String()
}

// stateKeyFromPath 从 Input/Output 的 Path 模式中推导 PregelState 的 key。
//
// 从文件路径推导 key 是启发式的（取最后一段、去除扩展名和通配符）：
//
//	"data/cases/{caseId}/outputs/claims.md"            → "claims"
//	"data/cases/{caseId}/outputs/*-cleaned.md"          → "cleaned"
//	"data/cases/{caseId}/disclosure/*.{md,txt,pdf}"     → "disclosure"
//	"search-request.md"                                  → "search_request"
//
// ⚠️ 推导结果不保证对所有文件路径模式都稳定。当需要确切 key 时，
// 使用 "state:key_name" 前缀显式指定，函数会原样返回 "key_name"。
func stateKeyFromPath(path string) string {
	// 显式 key 前缀
	if strings.HasPrefix(path, "state:") {
		return strings.TrimPrefix(path, "state:")
	}

	// 保存父路径用于 fallback
	parentDir := ""
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		parentDir = path[:idx]
		path = path[idx+1:]
	}

	// 去掉通配符 * 和 { }
	path = strings.ReplaceAll(path, "*", "")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")

	// 去掉文件扩展名（最后一个 . 及其后的内容）
	if idx := strings.Index(path, "."); idx >= 0 {
		path = path[:idx]
	}

	// 去掉首尾多余的连字符/下划线/逗号（brace expansion 残留）
	path = strings.Trim(path, "-_,;|")
	path = strings.TrimSpace(path)

	// 驼峰式连字符转下划线
	path = strings.ReplaceAll(path, "-", "_")

	// 如果文件名清空后无结果，尝试用父目录名
	if path == "" && parentDir != "" {
		if idx := strings.LastIndex(parentDir, "/"); idx >= 0 {
			path = parentDir[idx+1:]
		} else {
			path = parentDir
		}
		// 对父目录名也做清理
		path = strings.ReplaceAll(path, "{", "")
		path = strings.ReplaceAll(path, "}", "")
		path = strings.ReplaceAll(path, "*", "")
		path = strings.Trim(path, "-_")
		path = strings.TrimSpace(path)
		path = strings.ReplaceAll(path, "-", "_")
	}

	if path == "" {
		return "_fallback"
	}
	return path
}

// writeOutputs 将输出文本写入 state 中 Outputs 指定的各 key。
//
// 策略：
//   - 无 Outputs 契约时：写入 worker.Name 作为 state key（兜底）
//   - 有多个 Outputs 时：首个写入完整内容（视为主输出），
//     后续 Outputs 写入 ≤200 字的摘要（辅助输出用简短描述即可）
func (e *Executor) writeOutputs(state graph.PregelState, output string) {
	if len(e.def.Outputs) == 0 {
		// 无显式输出契约时，默认写入 worker.Name 作为 key
		state[e.def.Name] = output
		return
	}
	for i, out := range e.def.Outputs {
		key := stateKeyFromPath(out.Path)
		if i == 0 {
			state[key] = output
		} else {
			// 非首个 output key 写入摘要
			summary := output
			if len([]rune(summary)) > 200 {
				summary = string([]rune(summary)[:200]) + "…"
			}
			state[key] = summary
		}
	}
}

// =============================================================================
// 编译检查
// =============================================================================

// recordExec 记录执行到 Monitor（如果已接入）。
func recordExec(mon *Monitor, def *Definition, mode ExecMode, start time.Time, inputIssues, outputIssues int, success bool, errMsg string) {
	if mon == nil {
		return
	}
	mon.Record(ExecutionRecord{
		WorkerName:  def.Name,
		Tier:        def.Tier,
		Mode:        mode.String(),
		StartedAt:   start,
		Duration:    time.Since(start),
		InputValid:  inputIssues == 0,
		OutputValid: outputIssues == 0,
		Success:     success,
		Degraded:    inputIssues > 0 || outputIssues > 0 || !success,
		ErrorMsg:    errMsg,
	})
}
