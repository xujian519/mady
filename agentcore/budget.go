package agentcore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// BudgetDimension identifies which cost dimension was exceeded.
type BudgetDimension string

const (
	BudgetDimTokens    BudgetDimension = "tokens"
	BudgetDimCalls     BudgetDimension = "model_calls"
	BudgetDimToolCalls BudgetDimension = "tool_calls"
	BudgetDimDuration  BudgetDimension = "duration"
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

// BudgetUsage tracks cumulative consumption across the dimensions of Budget.
type BudgetUsage struct {
	Tokens    int64         `json:"tokens"`
	Calls     int64         `json:"calls"`
	ToolCalls int64         `json:"tool_calls"`
	Duration  time.Duration `json:"duration"`
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

// BudgetController enforces a Budget by acting as a LifecycleHook. It
// accumulates actual token usage from provider responses, counts model calls
// and tool invocations, and measures wall-clock duration. When any configured
// limit is reached, BeforeModelCall / BeforeToolExecution returns a
// *BudgetExceededError, which short-circuits the agent loop with StatusError.
//
// The controller resets its accumulator on each BeforeAgentRun, so a single
// instance tracks one Agent.Run invocation.
type BudgetController struct {
	BaseLifecycleHook
	budget Budget

	mu       sync.Mutex
	usage    BudgetUsage
	start    time.Time
	started  bool
	OnExceed func(dim BudgetDimension, budget Budget, usage BudgetUsage)
}

// NewBudgetController creates a controller enforcing the given budget.
func NewBudgetController(budget Budget) *BudgetController {
	return &BudgetController{budget: budget}
}

// Budget returns the configured limits.
func (c *BudgetController) Budget() Budget {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.budget
}

// Usage returns a snapshot of cumulative consumption. Duration reflects the
// time elapsed since the first model call of the current run.
func (c *BudgetController) Usage() BudgetUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := c.usage
	if c.started {
		u.Duration = time.Since(c.start)
		if u.Duration < c.usage.Duration {
			u.Duration = c.usage.Duration
		}
	}
	return u
}

// Reset clears all accumulated usage. Safe to call while idle.
func (c *BudgetController) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usage = BudgetUsage{}
	c.start = time.Time{}
	c.started = false
}

func (c *BudgetController) markStartLocked() {
	if !c.started {
		c.start = time.Now()
		c.started = true
	}
}

func (c *BudgetController) elapsedLocked() time.Duration {
	if !c.started {
		return 0
	}
	return time.Since(c.start)
}

// checkModelLocked verifies token / call / duration limits. Caller holds c.mu.
func (c *BudgetController) checkModelLocked() error {
	c.markStartLocked()
	b, u := c.budget, c.usage
	elapsed := c.elapsedLocked()
	if b.MaxTokens > 0 && u.Tokens >= b.MaxTokens {
		return &BudgetExceededError{Dimension: BudgetDimTokens, Limit: b.MaxTokens, Used: u.Tokens}
	}
	if b.MaxCalls > 0 && u.Calls >= b.MaxCalls {
		return &BudgetExceededError{Dimension: BudgetDimCalls, Limit: b.MaxCalls, Used: u.Calls}
	}
	if b.MaxDuration > 0 && elapsed >= b.MaxDuration {
		return &BudgetExceededError{Dimension: BudgetDimDuration, Limit: int64(b.MaxDuration), Used: int64(elapsed)}
	}
	return nil
}

// checkToolsLocked verifies the tool-call budget for a pending batch.
func (c *BudgetController) checkToolsLocked(pending int64) error {
	if c.budget.MaxToolCalls <= 0 {
		return nil
	}
	// Refuse the whole batch if dispatching it would breach the limit. This
	// keeps accounting simple and avoids partial-batch ambiguity.
	if c.usage.ToolCalls+pending > c.budget.MaxToolCalls {
		return &BudgetExceededError{Dimension: BudgetDimToolCalls, Limit: c.budget.MaxToolCalls, Used: c.usage.ToolCalls + pending}
	}
	return nil
}

func (c *BudgetController) fireExceed(err error) {
	if c.OnExceed == nil {
		return
	}
	var be *BudgetExceededError
	if errors.As(err, &be) {
		c.OnExceed(be.Dimension, c.budget, c.Usage())
	}
}

// BeforeAgentRun resets the accumulator at the start of each run so limits
// apply per invocation.
func (c *BudgetController) BeforeAgentRun(_ context.Context, _ *AgentRunContext) error {
	c.Reset()
	return nil
}

// BeforeModelCall checks token / call / duration limits before the LLM is
// invoked. A *BudgetExceededError short-circuits the run.
func (c *BudgetController) BeforeModelCall(_ context.Context, _ *AgentRunContext, _ *ModelCallContext) error {
	c.mu.Lock()
	err := c.checkModelLocked()
	c.mu.Unlock()
	if err != nil {
		c.fireExceed(err)
	}
	return err
}

// AfterModelCall accumulates actual usage from the provider response. A call
// is counted once issued regardless of success; tokens are only added when the
// response reports them.
func (c *BudgetController) AfterModelCall(_ context.Context, _ *AgentRunContext, mcc *ModelCallContext) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usage.Calls++
	if mcc != nil && mcc.Response != nil {
		total := mcc.Response.Usage.TotalTokens
		if total == 0 {
			total = mcc.Response.Usage.PromptTokens + mcc.Response.Usage.CompletionTokens
		}
		if total > 0 {
			c.usage.Tokens += total
		}
	}
}

// BeforeToolExecution checks the tool-call budget for the pending batch.
func (c *BudgetController) BeforeToolExecution(_ context.Context, _ *AgentRunContext, tec *ToolExecutionContext) error {
	if tec == nil || len(tec.ToolCalls) == 0 {
		return nil
	}
	c.mu.Lock()
	err := c.checkToolsLocked(int64(len(tec.ToolCalls)))
	c.mu.Unlock()
	if err != nil {
		c.fireExceed(err)
	}
	return err
}

// AfterToolExecution accumulates completed tool invocations.
func (c *BudgetController) AfterToolExecution(_ context.Context, _ *AgentRunContext, tec *ToolExecutionContext) {
	if tec == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	count := int64(0)
	for _, r := range tec.Results {
		// Count tools that actually executed (have an ID or a result/error).
		if r.ToolCallID != "" || r.Result != "" || r.Err != nil {
			count++
		}
	}
	c.usage.ToolCalls += count
}

// WithBudget attaches a BudgetController to the agent's lifecycle. It composes
// with any lifecycle hook already configured so existing hooks are preserved.
func WithBudget(budget Budget) ConfigOption {
	return func(c *Config) {
		bc := NewBudgetController(budget)
		if c.Lifecycle == nil {
			c.Lifecycle = bc
			return
		}
		if chain, ok := c.Lifecycle.(LifecycleChain); ok {
			merged := make(LifecycleChain, 0, len(chain)+1)
			merged = append(merged, chain...)
			merged = append(merged, bc)
			c.Lifecycle = merged
			return
		}
		c.Lifecycle = LifecycleChain{c.Lifecycle, bc}
	}
}
