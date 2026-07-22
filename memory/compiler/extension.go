package compiler

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the compiler extension.
const ExtensionName = "compiler"

// CompilerExtension integrates strategy learning into the agent lifecycle.
//
//   - BeforeTurn: selects a strategy and injects guidance into context
//   - AfterTurn: records the outcome and updates statistics
type CompilerExtension struct {
	compiler *Compiler
	agent    *agentcore.Agent

	// Per-turn state (protected by mu for concurrent safety)
	mu              sync.Mutex
	currentGoal     string
	currentStrategy string
	turnCount       int64
}

var (
	_ agentcore.Extension            = (*CompilerExtension)(nil)
	_ agentcore.LifecycleProvider    = (*CompilerExtension)(nil)
	_ agentcore.SystemPromptProvider = (*CompilerExtension)(nil)
)

// NewExtension creates a compiler extension.
func NewExtension(compiler *Compiler) *CompilerExtension {
	return &CompilerExtension{compiler: compiler}
}

// Name implements agentcore.Extension.
func (e *CompilerExtension) Name() string { return ExtensionName }

// Init implements agentcore.Extension.
func (e *CompilerExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose implements agentcore.Extension.
func (e *CompilerExtension) Dispose() error { return nil }

// SystemPromptSuffix implements SystemPromptProvider.
// Adds a brief note about available strategies.
func (e *CompilerExtension) SystemPromptSuffix() string {
	stats := e.compiler.Stats()
	if stats.TotalStrategies == 0 {
		return ""
	}
	return fmt.Sprintf("\n\n[策略学习系统] 已加载 %d 个领域策略，已记录 %d 次执行轨迹。",
		stats.TotalStrategies, stats.TotalTraces)
}

// LifecycleHook implements LifecycleProvider.
func (e *CompilerExtension) LifecycleHook() agentcore.LifecycleHook {
	return &compilerHook{ext: e}
}

type compilerHook struct {
	agentcore.BaseLifecycleHook
	ext *CompilerExtension
}

func (h *compilerHook) BeforeTurn(_ context.Context, arc *agentcore.AgentRunContext) error {
	guidance, strategyID := h.ext.compiler.StartTurn(arc.Input)

	h.ext.mu.Lock()
	h.ext.currentGoal = arc.Input
	h.ext.turnCount = arc.Turn
	h.ext.currentStrategy = strategyID
	h.ext.mu.Unlock()

	// Inject guidance as a steering message if available
	if guidance != "" && h.ext.agent != nil {
		h.ext.agent.Steer(agentcore.Message{
			Role:    agentcore.RoleSystem,
			Content: guidance,
		})
	}
	return nil
}

func (h *compilerHook) AfterTurn(_ context.Context, arc *agentcore.AgentRunContext, info agentcore.TurnInfo) {
	h.ext.mu.Lock()
	goal := h.ext.currentGoal
	strategyID := h.ext.currentStrategy
	h.ext.mu.Unlock()

	trace := NewTrace(
		fmt.Sprintf("turn-%d", arc.Turn),
		goal,
		strategyID,
		arc.Turn,
	)

	// Determine outcome based on turn execution signals.
	outcome := classifyTurnOutcome(arc, info)

	// Count tool calls from messages (approximate: count assistant messages
	// that contain tool-call markers).
	toolCalls, toolErrors := countToolStats(arc.Messages)

	trace.Complete(outcome, toolCalls, toolErrors)
	h.ext.compiler.FinishTurn(trace)
}

// classifyTurnOutcome determines the execution outcome from turn context.
// It inspects the last assistant message for failure signals.
// The arc parameter provides message history; TurnInfo is retained for future expansion.
func classifyTurnOutcome(arc *agentcore.AgentRunContext, _ agentcore.TurnInfo) Outcome {
	if len(arc.Messages) == 0 {
		return OutcomePartial
	}

	// Find the last assistant message
	var lastAssistant string
	for i := len(arc.Messages) - 1; i >= 0; i-- {
		if arc.Messages[i].Role == agentcore.RoleAssistant {
			lastAssistant = arc.Messages[i].Content
			break
		}
	}

	if lastAssistant == "" {
		return OutcomePartial
	}

	// Check for failure signals in the response
	if containsFailureSignal(lastAssistant) {
		return OutcomeFailure
	}

	// No failure signal detected — the turn completed without indication of failure.
	return OutcomeSuccess
}

// failureSignals are substrings that indicate the agent failed to accomplish
// the goal. Kept conservative to avoid false positives.
var failureSignals = []string{
	"无法完成",
	"无法执行",
	"操作失败",
	"执行出错",
	"I cannot",
	"I'm unable to",
	"failed to",
	"error occurred",
}

// containsFailureSignal checks whether the response text contains any
// known failure indicator.
func containsFailureSignal(text string) bool {
	lower := strings.ToLower(text)
	for _, sig := range failureSignals {
		if strings.Contains(lower, strings.ToLower(sig)) {
			return true
		}
	}
	return false
}

// countToolStats approximates tool call and error counts from messages.
// It counts tool-result messages and checks for error content.
// The matching is deliberately conservative to avoid false positives:
// patterns like "error:" (with colon) and "error occurred" are preferred
// over a bare "error" substring.
func countToolStats(msgs []agentcore.Message) (toolCalls, toolErrors int) {
	for _, msg := range msgs {
		if msg.Role == agentcore.RoleTool {
			toolCalls++
			if containsToolErrorMessage(msg.Content) {
				toolErrors++
			}
		}
	}
	return toolCalls, toolErrors
}

// toolErrorSignals are patterns in tool result content that indicate execution errors.
// These are more specific than a bare "error" substring to reduce false positives.
var toolErrorSignals = []string{
	"error:",
	"error occurred",
	"exit status",
	"command not found",
	"permission denied",
	"execution failed",
}

// containsToolErrorMessage checks whether the text contains known error indicators.
func containsToolErrorMessage(text string) bool {
	lower := strings.ToLower(text)
	for _, sig := range toolErrorSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}
