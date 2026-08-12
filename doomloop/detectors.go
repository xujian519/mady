package doomloop

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

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

// SignalError wraps a doomloop Signal as an error so callers can propagate
// fatal signals through the standard error chain and recover them with
// errors.As or IsDoomLoopFatal. Prefer NewSignalError over fmt.Errorf when
// constructing doomloop errors.
type SignalError struct {
	Signal Signal
}
