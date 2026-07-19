package agentcore

import (
	"context"
	"fmt"
	"strings"
)

// NodeError is a structured error that carries the execution path
// (agent name, turn, node/tool) where the error occurred.
// This makes debugging complex multi-agent workflows significantly easier.
type NodeError struct {
	Path    []string // execution path, e.g. ["coordinator", "turn:3", "tool:get_weather"]
	Message string
	Err     error
}

func (e *NodeError) Error() string {
	path := strings.Join(e.Path, " → ")
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", path, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", path, e.Message)
}

func (e *NodeError) Unwrap() error { return e.Err }

// NewNodeError creates a NodeError with the given path segments.
func NewNodeError(msg string, err error, path ...string) *NodeError {
	return &NodeError{Path: path, Message: msg, Err: err}
}

// WrapNodeError appends a path segment to an existing NodeError,
// or wraps a plain error into a new NodeError.
func WrapNodeError(err error, pathSegment string) error {
	if err == nil {
		return nil
	}
	if ne, ok := err.(*NodeError); ok {
		ne.Path = append([]string{pathSegment}, ne.Path...)
		return ne
	}
	return &NodeError{
		Path:    []string{pathSegment},
		Message: err.Error(),
		Err:     err,
	}
}

// ErrExceedMaxSteps is returned when a graph/workflow exceeds the maximum step count.
var ErrExceedMaxSteps = fmt.Errorf("超出最大执行步数")

// UnknownToolHandler is called when the model requests a tool that doesn't exist.
// It receives the tool call and should return a result string to send back to the model.
// This is the recommended way to handle LLM "hallucinated" tool names gracefully.
type UnknownToolHandler func(ctx context.Context, tc ToolCall) (string, error)

// DefaultUnknownToolHandler returns an error message listing available tools.
func DefaultUnknownToolHandler(availableNames []string) UnknownToolHandler {
	return func(_ context.Context, tc ToolCall) (string, error) {
		return fmt.Sprintf(
			"错误: 工具 %q 不存在。可用工具: %s",
			tc.Name, strings.Join(availableNames, ", "),
		), nil
	}
}

// DynamicUnknownToolHandler returns an error message listing the registry's
// current tool names, which is useful when tools can be hot-reloaded.
func DynamicUnknownToolHandler(registry interface{ Names() []string }) UnknownToolHandler {
	return func(_ context.Context, tc ToolCall) (string, error) {
		availableNames := registry.Names()
		return fmt.Sprintf(
			"错误: 工具 %q 不存在。可用工具: %s",
			tc.Name, strings.Join(availableNames, ", "),
		), nil
	}
}

// ──────────────────────────────────────────────
// 错误类型分类体系
//
// 各模块可通过类型断言判断错误类别，
// 以便在入口层实现差异化重试/降级策略。
// ──────────────────────────────────────────────

// RetryableError 表示可重试的临时性错误（如 Provider 超时、网络抖动）。
type RetryableError struct {
	Op      string
	Details string
	Err     error
}

func (e *RetryableError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s 临时失败: %s (%v)", e.Op, e.Details, e.Err)
	}
	return fmt.Sprintf("%s 临时失败: %s", e.Op, e.Details)
}

func (e *RetryableError) Unwrap() error { return e.Err }

// NewRetryableError 创建一个可重试错误。
func NewRetryableError(op, details string, err error) error {
	return &RetryableError{Op: op, Details: details, Err: err}
}

// FatalError 表示不可恢复的致命错误（如配置错误、Provider 不可用）。
type FatalError struct {
	Op      string
	Details string
	Err     error
}

func (e *FatalError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s 致命错误: %s (%v)", e.Op, e.Details, e.Err)
	}
	return fmt.Sprintf("%s 致命错误: %s", e.Op, e.Details)
}

func (e *FatalError) Unwrap() error { return e.Err }

// NewFatalError 创建一个致命错误。
func NewFatalError(op, details string, err error) error {
	return &FatalError{Op: op, Details: details, Err: err}
}

// HandoffError 表示 Handoff 过程中的错误。
type HandoffError struct {
	Op         string
	TargetName string
	Details    string
	Err        error
}

func (e *HandoffError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("handoff %s -> %s: %s (%v)", e.Op, e.TargetName, e.Details, e.Err)
	}
	return fmt.Sprintf("handoff %s -> %s: %s", e.Op, e.TargetName, e.Details)
}

func (e *HandoffError) Unwrap() error { return e.Err }

// GuardrailError 表示护栏拦截错误。
type GuardrailError struct {
	LevelStr string // 护栏等级描述（"light"/"standard"/"strict"）
	Reason   string // 拦截原因
	Details  string
	Err      error
}

func (e *GuardrailError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("guardrail [%s] %s: %s (%v)", e.LevelStr, e.Reason, e.Details, e.Err)
	}
	return fmt.Sprintf("guardrail [%s] %s: %s", e.LevelStr, e.Reason, e.Details)
}

func (e *GuardrailError) Unwrap() error { return e.Err }

// IsRetryable 判断错误是否可重试。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*RetryableError)
	return ok
}

// IsFatal 判断错误是否致命。
func IsFatal(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*FatalError)
	return ok
}

// IsHandoffError 判断是否 Handoff 错误。
func IsHandoffError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*HandoffError)
	return ok
}

// IsGuardrailError 判断是否护栏错误。
func IsGuardrailError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*GuardrailError)
	return ok
}
