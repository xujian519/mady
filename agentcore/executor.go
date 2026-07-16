package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore/concurrency"
)

// ExecutionMode controls whether tool calls run serially or in parallel.
type ExecutionMode string

const (
	// ModeSerial runs tool calls one after the other (default).
	ModeSerial ExecutionMode = "serial"
	// ModeParallel runs independent tool calls concurrently.
	ModeParallel ExecutionMode = "parallel"
)

// DualToolOutput wraps a tool result with separate output for LLM and user.
type DualToolOutput struct {
	ForLLM    string `json:"for_llm"`
	ForUser   string `json:"for_user"`
	Silent    bool   `json:"silent,omitempty"`
	Terminate bool   `json:"terminate,omitempty"`
}

// NewToolResult creates a result visible to both LLM and user.
func NewToolResult(forLLM string) *DualToolOutput {
	return &DualToolOutput{ForLLM: forLLM}
}

// SilentResult creates a result visible only to the LLM (not shown to user).
func SilentResult(forLLM string) *DualToolOutput {
	return &DualToolOutput{ForLLM: forLLM, Silent: true}
}

// UserResult creates a result visible to both LLM and user.
func UserResult(content string) *DualToolOutput {
	return &DualToolOutput{ForLLM: content, ForUser: content}
}

// TerminateResult creates a result that ends the agent loop immediately:
// content becomes the final answer and no further LLM turn runs.
func TerminateResult(content string) *DualToolOutput {
	return &DualToolOutput{ForLLM: content, ForUser: content, Terminate: true}
}

// terminateKey carries a mutable early-exit marker through the middleware
// chain so that coreExecute can signal termination without changing the
// (string, error) ExecuteFunc contract.
type terminateKey struct{}

type terminateMarker struct {
	set     bool
	content string
}

// ExecutorConfig tunes how the executor dispatches tool calls.
type ExecutorConfig struct {
	Mode               ExecutionMode
	Concurrency        int64 // max parallel goroutines; 0 = unlimited
	Middleware         []Middleware
	Before             []BeforeHook       // global before hooks applied to every tool
	After              []AfterHook        // global after hooks applied to every tool
	ValidateArguments  bool               // enable JSON Schema validation of tool arguments
	UnknownToolHandler UnknownToolHandler // called when the model hallucinates a tool name
}

// ToolResult holds the outcome of a single tool call execution.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Result     string
	// ForLLM provides alternative content shown to the LLM.
	// When set, this replaces Result in the model context.
	ForLLM string
	// ForUser provides alternative content shown to the user.
	// When set, this replaces Result in the user display.
	ForUser string
	// Silent suppresses display output.
	Silent bool
	// Terminate ends the agent loop: Result becomes the final answer and no
	// further LLM turn is invoked. Set when a tool returns TerminateResult.
	Terminate bool
	Err       error
	Duration  time.Duration
}

// IsDualOutput returns true when LLM and user outputs differ.
func (r *ToolResult) IsDualOutput() bool {
	return r.ForLLM != "" && r.ForLLM != r.Result
}

// EffectiveResult returns the content for LLM context.
func (r *ToolResult) EffectiveResult() string {
	if r.ForLLM != "" {
		return r.ForLLM
	}
	return r.Result
}

// ExecuteCallbacks provides optional real-time notifications during ExecuteAll.
type ExecuteCallbacks struct {
	OnStart func(tc ToolCall)
	OnEnd   func(result ToolResult)
}

// Executor dispatches tool calls against a Registry with hooks and middleware.
type Executor struct {
	registry *Registry
	config   ExecutorConfig
	chain    ExecuteFunc
}

func NewExecutor(registry *Registry, cfg ...ExecutorConfig) *Executor {
	var config ExecutorConfig
	if len(cfg) > 0 {
		config = cfg[0]
	}
	if config.Mode == "" {
		config.Mode = ModeSerial
	}

	e := &Executor{
		registry: registry,
		config:   config,
	}
	e.chain = e.buildChain()
	return e
}

func (e *Executor) buildChain() ExecuteFunc {
	var core ExecuteFunc = e.coreExecute
	for i := len(e.config.Middleware) - 1; i >= 0; i-- {
		core = e.config.Middleware[i](core)
	}
	return core
}

// coreExecute looks up the tool, optionally validates arguments, and invokes its Func.
func (e *Executor) coreExecute(ctx context.Context, tc ToolCall) (string, error) {
	tool, ok := e.registry.Get(tc.Name)
	if !ok {
		if e.config.UnknownToolHandler != nil {
			return e.config.UnknownToolHandler(ctx, tc)
		}
		return "", fmt.Errorf("工具未找到: %s", tc.Name)
	}

	// Unconditional JSON validity check. When the model output is truncated by
	// max_tokens the tool-call arguments are cut mid-string, producing invalid
	// JSON. Running the tool with partial arguments risks semantic corruption
	// (e.g. a half-written file path). Reject early with a clear message so the
	// model regenerates the call. This runs regardless of ValidateArguments
	// because it is a correctness guard, not a schema conformance check.
	if tc.Arguments != "" && !json.Valid([]byte(tc.Arguments)) {
		return "", fmt.Errorf(
			"工具 %s 的参数不是有效的 JSON — 上一条响应可能已被 max_tokens 截断；请重新生成包含完整参数的工具调用",
			tc.Name,
		)
	}

	if e.config.ValidateArguments {
		if err := ValidateToolArguments(tool, tc.Arguments); err != nil {
			return "", fmt.Errorf("参数校验失败 %s: %w", tc.Name, err)
		}
	}

	result, err := tool.Func(ctx, json.RawMessage(tc.Arguments))
	if err != nil {
		// Interrupt signals pass through without wrapping so the result
		// string and the interrupt error are both preserved.
		if IsInterrupt(err) {
			var resultStr string
			if str, ok := result.(string); ok {
				resultStr = str
			} else if result != nil {
				data, marshalErr := json.Marshal(result)
				if marshalErr != nil {
					resultStr = fmt.Sprintf("%v", result)
				} else {
					resultStr = string(data)
				}
			}
			return resultStr, err
		}
		return "", fmt.Errorf("工具 %s 执行失败: %w", tc.Name, err)
	}

	// Handle DualToolOutput: use ForLLM content for the model context.
	// The separate ForLLM/ForUser distinction is resolved here rather than
	// through string encoding.
	if dual, ok := result.(*DualToolOutput); ok {
		if dual.Terminate {
			if tm, ok := ctx.Value(terminateKey{}).(*terminateMarker); ok {
				tm.set = true
				content := dual.ForLLM
				if content == "" {
					content = dual.ForUser
				}
				tm.content = content
			}
		}
		if dual.ForLLM != "" {
			return dual.ForLLM, nil
		}
		return dual.ForUser, nil
	}

	if str, ok := result.(string); ok {
		return str, nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}
	return string(data), nil
}

// Execute runs a single tool call: tool-before → global-before → middleware chain → global-after → tool-after.
func (e *Executor) Execute(ctx context.Context, tc ToolCall, state *AgentState) ToolResult {
	start := time.Now()
	tm := &terminateMarker{}
	ctx = context.WithValue(ctx, terminateKey{}, tm)

	hc := &HookContext{
		ToolName:  tc.Name,
		Arguments: json.RawMessage(tc.Arguments),
		State:     state,
	}

	tool, hasTool := e.registry.Get(tc.Name)

	// tool-level before hooks
	if hasTool {
		for _, hook := range tool.Before {
			if err := hook(ctx, hc); err != nil {
				return ToolResult{ToolCallID: tc.ID, ToolName: tc.Name, Err: err, Duration: time.Since(start)}
			}
		}
	}
	// global before hooks
	for _, hook := range e.config.Before {
		if err := hook(ctx, hc); err != nil {
			return ToolResult{ToolCallID: tc.ID, ToolName: tc.Name, Err: err, Duration: time.Since(start)}
		}
	}

	// middleware chain → core
	result, err := e.chain(ctx, tc)

	tr := ToolResult{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Result:     result,
		Err:        err,
		Duration:   time.Since(start),
	}
	if tm.set {
		tr.Terminate = true
		if tr.Result == "" {
			tr.Result = tm.content
		}
	}

	// tool-level after hooks
	if hasTool {
		for _, hook := range tool.After {
			hook(ctx, hc, result, err)
		}
	}
	// global after hooks
	for _, hook := range e.config.After {
		hook(ctx, hc, result, err)
	}

	return tr
}

// ExecuteAll runs multiple tool calls using the configured execution mode,
// firing optional callbacks in real time for each tool.
func (e *Executor) ExecuteAll(ctx context.Context, calls []ToolCall, state *AgentState, cb *ExecuteCallbacks) []ToolResult {
	if e.config.Mode == ModeParallel && len(calls) > 1 {
		return e.executeParallel(ctx, calls, state, cb)
	}
	return e.executeSerial(ctx, calls, state, cb)
}

func (e *Executor) executeSerial(ctx context.Context, calls []ToolCall, state *AgentState, cb *ExecuteCallbacks) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, tc := range calls {
		if cb != nil && cb.OnStart != nil {
			cb.OnStart(tc)
		}
		results[i] = e.Execute(ctx, tc, state)
		if cb != nil && cb.OnEnd != nil {
			cb.OnEnd(results[i])
		}
	}
	return results
}

func (e *Executor) executeParallel(ctx context.Context, calls []ToolCall, state *AgentState, cb *ExecuteCallbacks) []ToolResult {
	results := make([]ToolResult, len(calls))
	var wg sync.WaitGroup

	maxConcurrent := int(e.config.Concurrency)
	if maxConcurrent <= 0 {
		maxConcurrent = len(calls)
	}
	pool := concurrency.NewPool(maxConcurrent)

	for i, tc := range calls {
		wg.Add(1)
		go func(idx int, call ToolCall) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = ToolResult{ToolName: call.Name, Result: fmt.Sprintf("异常: %v", r)}
				}
			}()

			// Context-aware acquire: respects ctx cancellation.
			if err := pool.Acquire(ctx); err != nil {
				results[idx] = ToolResult{ToolName: call.Name, Result: fmt.Sprintf("并发获取失败: %v", err)}
				return
			}
			defer pool.Release()

			if cb != nil && cb.OnStart != nil {
				cb.OnStart(call)
			}
			results[idx] = e.Execute(ctx, call, state)
			if cb != nil && cb.OnEnd != nil {
				cb.OnEnd(results[idx])
			}
		}(i, tc)
	}

	wg.Wait()
	return results
}
