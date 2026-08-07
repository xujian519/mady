package agentcore

import (
	"errors"
	"fmt"
	"time"
)

// BudgetDimension identifies which cost dimension was exceeded.
type BudgetDimension string

const (
	// BudgetDimTokens identifies total token consumption.
	BudgetDimTokens BudgetDimension = "tokens"
	// BudgetDimCalls identifies the number of model calls.
	BudgetDimCalls BudgetDimension = "model_calls"
	// BudgetDimToolCalls identifies the number of tool invocations.
	BudgetDimToolCalls BudgetDimension = "tool_calls"
	// BudgetDimDuration identifies wall-clock elapsed time.
	BudgetDimDuration BudgetDimension = "duration"
)

// Budget sets per-run cost limits. A zero value on any field means that
// dimension is unlimited.
type Budget struct {
	MaxTokens    int64         // total prompt+completion tokens, 0 = unlimited
	MaxCalls     int64         // number of LLM model calls, 0 = unlimited
	MaxToolCalls int64         // number of tool invocations, 0 = unlimited
	MaxDuration  time.Duration // wall-clock elapsed since the first call, 0 = unlimited
}

// IsUnlimited reports whether every dimension is unbounded.
func (b Budget) IsUnlimited() bool {
	return b.MaxTokens == 0 && b.MaxCalls == 0 && b.MaxToolCalls == 0 && b.MaxDuration == 0
}

// BudgetExceededError describes which dimension exceeded its limit and by how
// much. It unwraps through NewNodeError so callers can use errors.As even when
// the lifecycle layer wraps it.
type BudgetExceededError struct {
	Dimension BudgetDimension
	Limit     int64
	Used      int64
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: %s (limit %d, used %d)", e.Dimension, e.Limit, e.Used)
}

func (e *BudgetExceededError) Unwrap() error { return errBudgetExceeded }

// errBudgetExceeded is a sentinel so errors.Is(err, ErrBudgetExceeded) works
// through wrapping.
var errBudgetExceeded = errors.New("budget exceeded")

// ErrBudgetExceeded is the sentinel matched by errors.Is for any budget breach.
var ErrBudgetExceeded = errBudgetExceeded

// IsBudgetExceeded reports whether err (or any error in its chain) is a budget
// breach.
func IsBudgetExceeded(err error) bool {
	var target *BudgetExceededError
	return errors.Is(err, errBudgetExceeded) || errors.As(err, &target)
}

// BudgetController 和 WithBudget 已移除（2026-07-26）。
// 这些组件定义了完整的预算执行机制但从未在生产代码中实例化。
// Budget 类型和 BudgetExceededError 保留，供外部配置预算限制使用。
