package doomloop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// Core types
// ============================================================================

// DetectorID identifies a specific detector.
type DetectorID string

const (
	// DetectorToolCallLoop identifies the tool-call loop detector.
	DetectorToolCallLoop DetectorID = "tool_call_loop"
	// DetectorTextRepetition identifies the text repetition detector.
	DetectorTextRepetition DetectorID = "text_repetition"
	// DetectorCycle identifies the execution cycle detector.
	DetectorCycle DetectorID = "cycle"
	// DetectorEmptyResult identifies the empty result detector.
	DetectorEmptyResult DetectorID = "empty_result"
	// DetectorCircuitBreaker identifies the circuit breaker detector.
	DetectorCircuitBreaker DetectorID = "circuit_breaker"
	// DetectorCompactionBreaker identifies the compaction breaker detector.
	DetectorCompactionBreaker DetectorID = "compaction_breaker"
)

// Signal is emitted when a detector identifies a doom loop condition.
type Signal struct {
	// Detector that triggered this signal.
	Detector DetectorID `json:"detector"`
	// Reason is a human-readable explanation.
	Reason string `json:"reason"`
	// Turn is the agent turn when the signal was emitted.
	Turn int64 `json:"turn"`
	// Fatal, when true, immediately terminates the agent run.
	Fatal bool `json:"fatal"`
}

// Detector is the interface each individual detector must implement.
type Detector interface {
	// ID returns the unique detector identifier.
	ID() DetectorID
	// RecordModelCall records an AfterModelCall observation.
	RecordModelCall(ctx *agentcore.ModelCallContext) *Signal
	// RecordToolResult records an AfterToolExecution observation.
	RecordToolResult(ctx *agentcore.ToolExecutionContext) *Signal
	// Reset clears all accumulated state (called when agent starts a new run).
	Reset()
}

// ============================================================================
// DetectorConfiguration
// ============================================================================

// Config holds all configurable parameters for the doomloop detectors.
type Config struct {
	// ToolCallLoopMax is the max identical tool calls before triggering. 0=disabled.
	ToolCallLoopMax int
	// TextRepetitionMinRepeat is the min consecutive repeated text blocks. 0=disabled.
	TextRepetitionMinRepeat int
	// CycleLength is the minimum cycle length to detect (e.g., 2 for A→B→A→B). 0=disabled.
	CycleLength int
	// EmptyResultMax is the max consecutive empty tool results. 0=disabled.
	EmptyResultMax int
	// CircuitBreakerMax is the max total tool calls across all detectors. 0=disabled.
	CircuitBreakerMax int
	// CompactionMax is the max consecutive compaction summaries without progress. 0=disabled.
	CompactionMax int

	// ReminderThresholds 列出触发建议性提醒的连续同参调用次数（在下一轮模型
	// 请求注入提醒，不阻塞不终止）。空表示禁用。默认 3/5/8 次，先于
	// ToolCallLoopMax 的硬熔断给模型一次自纠机会。
	ReminderThresholds []int

	// OnSignal is an optional callback triggered when any detector fires.
	OnSignal func(Signal)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		ToolCallLoopMax:         5,
		TextRepetitionMinRepeat: 4,
		CycleLength:             2,
		EmptyResultMax:          5,
		CircuitBreakerMax:       100,
		CompactionMax:           5,
		ReminderThresholds:      []int{3, 5, 8},
	}
}

// Option is a functional option for New.
type Option func(*Config)

// WithToolCallLoop sets the maximum identical tool calls before triggering.
func WithToolCallLoop(n int) Option { return func(c *Config) { c.ToolCallLoopMax = n } }

// WithTextRepetition sets the minimum consecutive repeated text blocks before triggering.
func WithTextRepetition(n int) Option { return func(c *Config) { c.TextRepetitionMinRepeat = n } }

// WithCycleLength sets the minimum cycle length to detect.
func WithCycleLength(n int) Option { return func(c *Config) { c.CycleLength = n } }

// WithEmptyResultMax sets the maximum consecutive empty tool results before triggering.
func WithEmptyResultMax(n int) Option { return func(c *Config) { c.EmptyResultMax = n } }

// WithCircuitBreaker sets the maximum total tool calls before triggering.
func WithCircuitBreaker(n int) Option { return func(c *Config) { c.CircuitBreakerMax = n } }

// WithCompactionMax sets the maximum consecutive compaction summaries without progress.
func WithCompactionMax(n int) Option { return func(c *Config) { c.CompactionMax = n } }

// WithReminderThresholds sets the consecutive same-call counts at which an
// advisory reminder is injected. Empty slice or nil disables the reminder.
func WithReminderThresholds(counts ...int) Option {
	return func(c *Config) { c.ReminderThresholds = counts }
}

// WithoutReminder disables the advisory repeat-call reminder.
func WithoutReminder() Option { return func(c *Config) { c.ReminderThresholds = nil } }

// WithOnSignal sets a callback function invoked when any detector fires.
func WithOnSignal(fn func(Signal)) Option { return func(c *Config) { c.OnSignal = fn } }

// ============================================================================
// DoomLoop — aggregate detector coordinator
// ============================================================================

// DoomLoop coordinates all registered detectors and implements
// agentcore.LifecycleHook to plug into the agent runtime. 除致命检测器外，
// 它还携带建议性防循环提醒（reminderState）：连续同参工具调用达到阈值时，
// 在下一轮模型请求注入一条提醒，先于硬熔断给模型自纠机会。
type DoomLoop struct {
	mu        sync.Mutex
	config    Config
	detectors []Detector

	// aggregated state
	signals []Signal

	// advisory reminder state（mu 保护；thresholds 在 New 时构建为只读集合）
	reminder           reminderState
	reminderThresholds map[int]bool
}

// New creates a DoomLoop with the given options.
func New(opts ...Option) *DoomLoop {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	dl := &DoomLoop{config: cfg}
	dl.reminderThresholds = make(map[int]bool, len(cfg.ReminderThresholds))
	for _, n := range cfg.ReminderThresholds {
		if n > 0 {
			dl.reminderThresholds[n] = true
		}
	}

	// Build detector list based on config.
	if cfg.ToolCallLoopMax > 0 {
		dl.detectors = append(dl.detectors, &toolCallLoopDetector{max: cfg.ToolCallLoopMax})
	}
	if cfg.TextRepetitionMinRepeat > 0 {
		dl.detectors = append(dl.detectors, &textRepetitionDetector{minRepeat: cfg.TextRepetitionMinRepeat})
	}
	if cfg.CycleLength > 0 {
		dl.detectors = append(dl.detectors, &cycleDetector{cycleLen: cfg.CycleLength})
	}
	if cfg.EmptyResultMax > 0 {
		dl.detectors = append(dl.detectors, &emptyResultDetector{max: cfg.EmptyResultMax})
	}
	if cfg.CircuitBreakerMax > 0 {
		dl.detectors = append(dl.detectors, &circuitBreaker{max: cfg.CircuitBreakerMax})
	}
	if cfg.CompactionMax > 0 {
		dl.detectors = append(dl.detectors, &compactionBreaker{max: cfg.CompactionMax})
	}

	return dl
}

// AsHook returns a LifecycleHook that monitors the agent runtime.
func (dl *DoomLoop) AsHook() agentcore.LifecycleHook { //nolint:staticcheck
	return agentcore.ObserversToHook(&doomLoopHook{parent: dl})
}

// Signals returns all signals emitted so far.
func (dl *DoomLoop) Signals() []Signal {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	result := make([]Signal, len(dl.signals))
	copy(result, dl.signals)
	return result
}

// Reset clears all accumulated state in all detectors.
func (dl *DoomLoop) Reset() {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.signals = nil
	dl.reminder = reminderState{}
	for _, d := range dl.detectors {
		d.Reset()
	}
}

// emitSignal records a signal and calls the optional callback.
func (dl *DoomLoop) emitSignal(s Signal) {
	dl.mu.Lock()
	dl.signals = append(dl.signals, s)
	dl.mu.Unlock()
	if dl.config.OnSignal != nil {
		dl.config.OnSignal(s)
	}
}

// ============================================================================
// Lifecycle hook wrapper
// ============================================================================

type doomLoopHook struct {
	parent *DoomLoop
}

// Compile-time interface assertions.
var (
	_ agentcore.AgentRunObserver  = (*doomLoopHook)(nil)
	_ agentcore.ModelCallObserver = (*doomLoopHook)(nil)
	_ agentcore.ToolCallObserver  = (*doomLoopHook)(nil)
)

func (h *doomLoopHook) BeforeAgentRun(_ context.Context, arc *agentcore.AgentRunContext) error {
	h.parent.Reset()
	return nil
}

func (h *doomLoopHook) AfterAgentRun(_ context.Context, _ *agentcore.AgentRunContext, _ string, _ error) {
}

func (h *doomLoopHook) BeforeModelCall(_ context.Context, _ *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) error {
	if mcc == nil || mcc.Request == nil {
		return nil
	}
	dl := h.parent
	dl.mu.Lock()
	// 真实用户消息变化开启新一轮：连击计数与待注入提醒清零。
	// 提醒消息自带标记前缀，扫描时跳过，不会把上一次注入误判为新轮次；
	// 首次观察只记录用户消息不清零（连击可能已在本轮模型调用前开始累计）。
	if userContent := lastRealUserContent(mcc.Request.Messages); userContent != dl.reminder.lastRealUser {
		if dl.reminder.lastRealUser == "" {
			dl.reminder.lastRealUser = userContent
		} else {
			dl.reminder = reminderState{lastRealUser: userContent}
		}
	}
	pending := dl.reminder.pending
	dl.reminder.pending = ""
	dl.mu.Unlock()
	if pending == "" {
		return nil
	}
	mcc.Request.Messages = append(mcc.Request.Messages, agentcore.Message{
		Role:    agentcore.RoleUser,
		Content: pending,
	})
	return nil
}

func (h *doomLoopHook) AfterModelCall(_ context.Context, _ *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if mcc == nil || mcc.Response == nil || mcc.Err != nil {
		return
	}
	dl := h.parent
	// Hold the lock across the detector sweep so detector state (which is
	// NOT internally synchronized) is protected. The DoomLoop contract
	// requires serial hook invocation; the lock is defensive against a
	// shared DoomLoop or a future concurrent runtime. OnSignal callbacks
	// fire AFTER the lock is released (in emitSignal) so they cannot
	// deadlock and cannot observe partial state.
	dl.mu.Lock()
	pending := make([]Signal, 0)
	for _, d := range dl.detectors {
		if sig := d.RecordModelCall(mcc); sig != nil {
			pending = append(pending, *sig)
		}
	}
	dl.mu.Unlock()
	for _, s := range pending {
		dl.emitSignal(s)
	}
}

func (h *doomLoopHook) BeforeToolExecution(_ context.Context, _ *agentcore.AgentRunContext, _ *agentcore.ToolExecutionContext) error {
	return nil
}

func (h *doomLoopHook) AfterToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) {
	if tec == nil {
		return
	}
	dl := h.parent
	dl.mu.Lock()
	pending := make([]Signal, 0)
	for _, d := range dl.detectors {
		if sig := d.RecordToolResult(tec); sig != nil {
			pending = append(pending, *sig)
		}
	}
	// 建议性提醒与致命检测器共享锁窗口；计数独立于 detector 状态。
	if len(dl.reminderThresholds) > 0 {
		dl.reminder.record(tec.ToolCalls, dl.reminderThresholds)
	}
	dl.mu.Unlock()
	for _, s := range pending {
		dl.emitSignal(s)
	}
}

// Error implements error.
func (e *SignalError) Error() string {
	return fmt.Sprintf("doomloop %s: %s", e.Signal.Detector, e.Signal.Reason)
}

// IsDoomLoopFatal checks if the error from the agent runtime was caused by a
// doomloop signal. Returns the Signal if so, nil otherwise.
//
// It prefers structured SignalError recovery via errors.As; as a compatibility
// fallback it also scans the error string for known detector IDs, which may
// produce false positives if a caller's error message happens to contain a
// detector name like "cycle" or "empty_result". New code should construct
// doomloop errors with NewSignalError.
func IsDoomLoopFatal(err error) *Signal {
	if err == nil {
		return nil
	}
	var se *SignalError
	if errors.As(err, &se) {
		s := se.Signal
		return &s
	}
	// Compatibility fallback: string match (fragile).
	errStr := err.Error()
	for _, id := range []DetectorID{
		DetectorToolCallLoop, DetectorTextRepetition,
		DetectorCycle, DetectorEmptyResult,
		DetectorCircuitBreaker, DetectorCompactionBreaker,
	} {
		if strings.Contains(errStr, string(id)) {
			return &Signal{Detector: id, Reason: errStr, Fatal: true}
		}
	}
	return nil
}
