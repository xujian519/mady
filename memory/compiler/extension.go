package compiler

import (
	"context"
	"fmt"

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

	// Per-turn state
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
	h.ext.currentGoal = arc.Input
	h.ext.turnCount = arc.Turn

	guidance, strategyID := h.ext.compiler.StartTurn(arc.Input)
	h.ext.currentStrategy = strategyID

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
	trace := NewTrace(
		fmt.Sprintf("turn-%d", arc.Turn),
		h.ext.currentGoal,
		h.ext.currentStrategy,
		arc.Turn,
	)

	// Determine outcome based on whether there were tool calls and messages
	outcome := OutcomeSuccess
	if !info.HadToolCalls && arc.Messages != nil && len(arc.Messages) > 0 {
		// Turn ended without tool calls — could be a direct response (success)
		// or a refusal (partial). We default to success for non-tool turns.
		outcome = OutcomeSuccess
	}

	trace.Complete(outcome, 0, 0)
	h.ext.compiler.FinishTurn(trace)
}
