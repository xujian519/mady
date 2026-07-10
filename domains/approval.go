package domains

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ApprovalGate is a LifecycleHook that pauses Agent execution at critical
// decision points, waiting for human confirmation before proceeding.
//
// It implements the "重点节点人机协作" principle from Mady's guiding
// philosophy. When the Agent reaches a high-stakes decision (e.g., final
// patent claim drafting, legal conclusion, risk assessment), this hook
// interrupts execution and waits for a human to review and approve.
//
// Usage:
//
//	gate := domains.NewApprovalGate(domains.ApprovalConfig{
//	    RequireApprovalFor: []string{"专利结论", "法律意见", "风险评估"},
//	    TimeoutMsg:         "等待人工审核中...",
//	})
//	cfg.Lifecycle = agentcore.LifecycleChain{gate}
//
// The human operator reviews the paused output and calls agent.Resume()
// to continue, or provides feedback through agent.FollowUp().
type ApprovalGate struct {
	agentcore.BaseLifecycleHook
	config ApprovalConfig
}

// ApprovalConfig controls when and how the approval gate triggers.
type ApprovalConfig struct {
	// RequireApprovalFor is a list of keywords. If the model's output
	// contains any of these, execution is paused for human approval.
	RequireApprovalFor []string

	// TimeoutMsg is the message shown to the human operator while waiting.
	// Default: "此步骤需要人工审核确认，请检查以下内容后回复'确认'继续，或提供修改意见。"
	TimeoutMsg string

	// SkipIfNoTools indicates whether to skip the gate if the output
	// doesn't involve tool calls (purely informational).
	SkipIfNoTools bool
}

// DefaultApprovalConfig returns a sensible default for patent/legal domains.
func DefaultApprovalConfig() ApprovalConfig {
	return ApprovalConfig{
		RequireApprovalFor: []string{
			"专利结论", "侵权判断", "有效性结论",
			"法律意见", "诉讼策略", "判决预测",
			"风险评估", "最终建议",
		},
		TimeoutMsg:   "此步骤需要人工审核确认。请检查以下内容后回复'确认'继续，或提供修改意见。",
		SkipIfNoTools: false,
	}
}

// NewApprovalGate creates an ApprovalGate with the given configuration.
func NewApprovalGate(config ApprovalConfig) *ApprovalGate {
	if len(config.RequireApprovalFor) == 0 {
		config = DefaultApprovalConfig()
	}
	if config.TimeoutMsg == "" {
		config.TimeoutMsg = "此步骤需要人工审核确认。请检查以下内容后回复'确认'继续，或提供修改意见。"
	}
	return &ApprovalGate{config: config}
}

// AfterModelCall implements LifecycleHook.AfterModelCall.
// It checks if the model's output triggers human approval and, if so,
// interrupts execution to wait for confirmation.
func (g *ApprovalGate) AfterModelCall(_ context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if mcc == nil || mcc.Response == nil || mcc.Err != nil {
		return
	}

	// Skip if no tool calls and SkipIfNoTools is set.
	if g.config.SkipIfNoTools && len(mcc.Response.ToolCalls) == 0 {
		return
	}

	// Check if output contains any trigger keywords.
	if !g.needsApproval(mcc.Response.Content) {
		return
	}

	// Interrupt and wait for human approval.
	// The interrupted output is preserved, and the human can review it
	// before calling Resume() to continue.
	arc.Agent.Steer(agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: g.buildApprovalMessage(mcc.Response.Content),
	})
}

// needsApproval checks if the content triggers the approval requirement.
func (g *ApprovalGate) needsApproval(content string) bool {
	for _, keyword := range g.config.RequireApprovalFor {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

// buildApprovalMessage constructs the human-readable approval prompt.
func (g *ApprovalGate) buildApprovalMessage(content string) string {
	// Truncate content for display.
	preview := content
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}

	return strings.Join([]string{
		"═══════════════════════════════════════",
		"⚠️  人 工 审 核 关 卡",
		"═══════════════════════════════════════",
		"",
		g.config.TimeoutMsg,
		"",
		"--- AI 生成内容预览 ---",
		preview,
		"",
		"操作方式：",
		"  • 回复「确认」→ 继续执行",
		"  • 回复修改意见 → AI 将根据您的意见调整",
		"  • 回复「取消」→ 终止当前任务",
		"═══════════════════════════════════════",
	}, "\n")
}

// RequireApproval is a helper function that domain code can call to
// explicitly mark a tool result as requiring human approval.
// It returns an InterruptError that pauses the Agent loop.
func RequireApproval(reason string, data map[string]any) error {
	return agentcore.NewInterruptErrorWithData(
		fmt.Sprintf("需要人工审核: %s", reason),
		data,
	)
}
