package doomloop

import (
	"context"
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
	DetectorToolCallLoop      DetectorID = "tool_call_loop"
	DetectorTextRepetition    DetectorID = "text_repetition"
	DetectorCycle             DetectorID = "cycle"
	DetectorEmptyResult       DetectorID = "empty_result"
	DetectorCircuitBreaker    DetectorID = "circuit_breaker"
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
	}
}

// Option is a functional option for New.
type Option func(*Config)

func WithToolCallLoop(n int) Option       { return func(c *Config) { c.ToolCallLoopMax = n } }
func WithTextRepetition(n int) Option     { return func(c *Config) { c.TextRepetitionMinRepeat = n } }
func WithCycleLength(n int) Option        { return func(c *Config) { c.CycleLength = n } }
func WithEmptyResultMax(n int) Option     { return func(c *Config) { c.EmptyResultMax = n } }
func WithCircuitBreaker(n int) Option     { return func(c *Config) { c.CircuitBreakerMax = n } }
func WithCompactionMax(n int) Option      { return func(c *Config) { c.CompactionMax = n } }
func WithOnSignal(fn func(Signal)) Option { return func(c *Config) { c.OnSignal = fn } }

// ============================================================================
// DoomLoop — aggregate detector coordinator
// ============================================================================

// DoomLoop coordinates all registered detectors and implements
// agentcore.LifecycleHook to plug into the agent runtime.
type DoomLoop struct {
	mu        sync.Mutex
	config    Config
	detectors []Detector

	// aggregated state
	totalToolCalls int
	signals        []Signal
}

// New creates a DoomLoop with the given options.
func New(opts ...Option) *DoomLoop {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	dl := &DoomLoop{config: cfg}

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
func (dl *DoomLoop) AsHook() agentcore.LifecycleHook {
	return &doomLoopHook{parent: dl}
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
	dl.totalToolCalls = 0
	dl.signals = nil
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
	agentcore.BaseLifecycleHook
	parent *DoomLoop
}

func (h *doomLoopHook) BeforeAgentRun(_ context.Context, arc *agentcore.AgentRunContext) error {
	h.parent.Reset()
	return nil
}

func (h *doomLoopHook) AfterModelCall(_ context.Context, _ *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if mcc == nil || mcc.Response == nil || mcc.Err != nil {
		return
	}
	dl := h.parent
	dl.mu.Lock()
	dl.totalToolCalls += len(mcc.Response.ToolCalls)
	dl.mu.Unlock()

	for _, d := range dl.detectors {
		if sig := d.RecordModelCall(mcc); sig != nil {
			dl.emitSignal(*sig)
		}
	}
}

func (h *doomLoopHook) AfterToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) {
	if tec == nil {
		return
	}
	dl := h.parent
	for _, d := range dl.detectors {
		if sig := d.RecordToolResult(tec); sig != nil {
			dl.emitSignal(*sig)
		}
	}
}

// ============================================================================
// Detector 1: ToolCallLoop — repeated identical tool calls
// ============================================================================

type toolCallLoopDetector struct {
	max  int
	last []agentcore.ToolCall // sliding window of recent calls
}

func (d *toolCallLoopDetector) ID() DetectorID { return DetectorToolCallLoop }

func (d *toolCallLoopDetector) RecordModelCall(mcc *agentcore.ModelCallContext) *Signal {
	if mcc == nil || mcc.Response == nil {
		return nil
	}
	calls := mcc.Response.ToolCalls
	if len(calls) == 0 {
		d.last = nil // reset on text-only response
		return nil
	}

	d.last = append(d.last, calls...)

	// Check if the last N calls are identical.
	n := len(d.last)
	if n < d.max {
		return nil
	}

	// Window: last `max` calls.
	window := d.last[n-d.max:]
	firstKey := toolCallKey(window[0])
	for _, tc := range window[1:] {
		if toolCallKey(tc) != firstKey {
			return nil // not all identical
		}
	}

	// All identical → signal.
	return &Signal{
		Detector: DetectorToolCallLoop,
		Reason:   fmt.Sprintf("工具调用死循环：连续 %d 次调用 %s，参数完全相同", d.max, window[0].Name),
		Fatal:    true,
	}
}

func (d *toolCallLoopDetector) RecordToolResult(_ *agentcore.ToolExecutionContext) *Signal {
	return nil
}

func (d *toolCallLoopDetector) Reset() { d.last = nil }

func toolCallKey(tc agentcore.ToolCall) string {
	return tc.Name + ":" + tc.Arguments
}

// ============================================================================
// Detector 2: TextRepetition — repetitive text in model output
// ============================================================================

type textRepetitionDetector struct {
	minRepeat int
	lastLines []string // sliding window of recent line blocks
}

func (d *textRepetitionDetector) ID() DetectorID { return DetectorTextRepetition }

func (d *textRepetitionDetector) RecordModelCall(mcc *agentcore.ModelCallContext) *Signal {
	if mcc == nil || mcc.Response == nil || mcc.Response.Content == "" {
		return nil
	}

	content := mcc.Response.Content
	lines := strings.Split(strings.TrimSpace(content), "\n")

	// Extract the last meaningful paragraph (skip empty lines).
	var lastBlock string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			lastBlock = line
			break
		}
	}
	if lastBlock == "" {
		return nil
	}

	d.lastLines = append(d.lastLines, lastBlock)

	// Check for consecutive identical blocks.
	n := len(d.lastLines)
	if n < d.minRepeat {
		return nil
	}

	window := d.lastLines[n-d.minRepeat:]
	first := window[0]
	for _, block := range window[1:] {
		if block != first {
			return nil
		}
	}

	return &Signal{
		Detector: DetectorTextRepetition,
		Reason:   fmt.Sprintf("文本输出死循环：连续 %d 次输出相同内容", d.minRepeat),
		Fatal:    true,
	}
}

func (d *textRepetitionDetector) RecordToolResult(_ *agentcore.ToolExecutionContext) *Signal {
	return nil
}

func (d *textRepetitionDetector) Reset() { d.lastLines = nil }

// ============================================================================
// Detector 3: Cycle — A→B→A→B execution cycles
// ============================================================================

type cycleDetector struct {
	cycleLen int
	history  []string // tool name history
}

func (d *cycleDetector) ID() DetectorID { return DetectorCycle }

func (d *cycleDetector) RecordModelCall(mcc *agentcore.ModelCallContext) *Signal {
	if mcc == nil || mcc.Response == nil {
		return nil
	}
	for _, tc := range mcc.Response.ToolCalls {
		d.history = append(d.history, tc.Name)
	}

	// Check for cycles in the history.
	n := len(d.history)
	if n < d.cycleLen*2 {
		return nil
	}

	// Look for repeating pattern at the end of the history.
	// Pattern: [... A B A B] where A→B→A→B forms a cycle.
	for patternLen := 1; patternLen <= d.cycleLen; patternLen++ {
		if n < patternLen*2 {
			continue
		}
		// Check tail against previous matching segment.
		match := true
		for i := 0; i < patternLen; i++ {
			if d.history[n-patternLen*2+i] != d.history[n-patternLen+i] {
				match = false
				break
			}
		}
		if match {
			cycle := d.history[n-patternLen:]
			return &Signal{
				Detector: DetectorCycle,
				Reason:   fmt.Sprintf("工具调用循环：检测到执行模式 [%s] 重复", strings.Join(cycle, "→")),
				Fatal:    true,
			}
		}
	}

	return nil
}

func (d *cycleDetector) RecordToolResult(_ *agentcore.ToolExecutionContext) *Signal {
	return nil
}

func (d *cycleDetector) Reset() { d.history = nil }

// ============================================================================
// Detector 4: EmptyResult — consecutive empty tool results
// ============================================================================

type emptyResultDetector struct {
	max         int
	consecutive int
}

func (d *emptyResultDetector) ID() DetectorID { return DetectorEmptyResult }

func (d *emptyResultDetector) RecordModelCall(_ *agentcore.ModelCallContext) *Signal {
	return nil
}

func (d *emptyResultDetector) RecordToolResult(tec *agentcore.ToolExecutionContext) *Signal {
	if tec == nil {
		return nil
	}
	allEmpty := true
	for _, r := range tec.Results {
		if r.Result != "" || r.Err != nil {
			allEmpty = false
			break
		}
	}

	if allEmpty && len(tec.Results) > 0 {
		d.consecutive++
	} else {
		d.consecutive = 0
	}

	if d.consecutive >= d.max {
		d.consecutive = 0 // reset after triggering
		return &Signal{
			Detector: DetectorEmptyResult,
			Reason:   fmt.Sprintf("空结果死循环：连续 %d 次工具调用返回空结果", d.max),
			Fatal:    true,
		}
	}
	return nil
}

func (d *emptyResultDetector) Reset() { d.consecutive = 0 }

// ============================================================================
// Detector 5: CircuitBreaker — global iteration limit
// ============================================================================

// circuitBreaker tracks total tool calls across the entire agent run.
// It needs external state from DoomLoop for the total count.
type circuitBreaker struct {
	max        int
	localCount int
}

func (d *circuitBreaker) ID() DetectorID { return DetectorCircuitBreaker }

func (d *circuitBreaker) RecordModelCall(mcc *agentcore.ModelCallContext) *Signal {
	if mcc == nil || mcc.Response == nil {
		return nil
	}
	d.localCount += len(mcc.Response.ToolCalls)
	if d.localCount >= d.max {
		return &Signal{
			Detector: DetectorCircuitBreaker,
			Reason:   fmt.Sprintf("熔断器触发：总工具调用次数 %d 超过上限 %d", d.localCount, d.max),
			Fatal:    true,
		}
	}
	return nil
}

func (d *circuitBreaker) RecordToolResult(_ *agentcore.ToolExecutionContext) *Signal {
	return nil
}

func (d *circuitBreaker) Reset() { d.localCount = 0 }

// ============================================================================
// Detector 6: CompactionBreaker — repeated compaction without progress
// ============================================================================

type compactionBreaker struct {
	max         int
	consecutive int
}

func (d *compactionBreaker) ID() DetectorID { return DetectorCompactionBreaker }

func (d *compactionBreaker) RecordModelCall(mcc *agentcore.ModelCallContext) *Signal {
	if mcc == nil || mcc.Response == nil {
		return nil
	}

	content := mcc.Response.Content
	isCompaction := strings.Contains(content, "【总结") ||
		strings.Contains(content, "【摘要") ||
		strings.Contains(content, "[SUMMARY]") ||
		strings.Contains(content, "[COMPACTION]") ||
		strings.Contains(content, "总结如下") ||
		strings.Contains(content, "概括如下")

	if isCompaction && len(mcc.Response.ToolCalls) == 0 {
		d.consecutive++
	} else {
		d.consecutive = 0
	}

	if d.consecutive >= d.max {
		d.consecutive = 0
		return &Signal{
			Detector: DetectorCompactionBreaker,
			Reason:   fmt.Sprintf("压缩死循环：连续 %d 次输出摘要/总结，未执行任何工具调用", d.max),
			Fatal:    true,
		}
	}
	return nil
}

func (d *compactionBreaker) RecordToolResult(_ *agentcore.ToolExecutionContext) *Signal {
	return nil
}

func (d *compactionBreaker) Reset() { d.consecutive = 0 }

// ============================================================================
// Signal helpers
// ============================================================================

// IsDoomLoopFatal checks if the error from the agent runtime was caused by a
// doomloop signal. Returns the Signal if so, nil otherwise.
func IsDoomLoopFatal(err error) *Signal {
	if err == nil {
		return nil
	}
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
