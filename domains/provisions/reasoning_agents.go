package provisions

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// Tier B — 推理模式工厂
// =============================================================================

// BuildReasoningSystemPrompt 为推理模式构建 System Prompt。
func BuildReasoningSystemPrompt(entry *ReasoningManifestEntry) string {
	var b strings.Builder

	fmt.Fprintf(&b, "你是 Mady 的%s（推理模式 %s）。\n", entry.Name, entry.ID)
	fmt.Fprintf(&b, "服务条款簇：%s\n\n", strings.Join(entry.Serves, "、"))

	b.WriteString("职责：封装跨条款的认定步骤，供条款智能体作为子步骤调用。\n\n")

	if len(entry.MethodologySteps) > 0 {
		b.WriteString("推理步骤：\n")
		for i, step := range entry.MethodologySteps {
			fmt.Fprintf(&b, "%d. %s\n", i+1, step)
		}
		b.WriteString("\n")
	}

	b.WriteString("约束规则：\n")
	b.WriteString("- 专注单一推理任务，不回答超出本推理模式范围的问题\n")
	b.WriteString("- 必须引用法律依据和推理逻辑\n")
	b.WriteString("- 信息不足时明确标注，不得强行结论\n")

	return b.String()
}

// ReasoningToHandoff 将一条推理模式 Manifest 条目转换为 HandoffConfig。
func ReasoningToHandoff(entry *ReasoningManifestEntry, base agentcore.Config) agentcore.HandoffConfig {
	cfg := base
	cfg.Name = entry.Worker
	cfg.SystemPrompt = BuildReasoningSystemPrompt(entry)

	// 推理模式子 Agent 保持轻量：不继承扩展和生命周期
	cfg.Extensions = nil
	cfg.Lifecycle = nil

	allowedTools := entry.PrimaryTools
	if len(allowedTools) == 0 {
		allowedTools = DefaultPatentTools
	}
	cfg.Tools = filterTools(allowedTools, cfg.Tools)

	return agentcore.HandoffConfig{
		Name:        entry.Worker,
		Description: fmt.Sprintf("专利推理模式：%s（%s）。服务 %s", entry.Name, entry.ID, strings.Join(entry.Serves, "、")),
		Mode:        agentcore.HandoffDelegate,
		AgentConfig: cfg,
		AllowedSources: []string{
			"patent-agent",
			"patent-orchestrator",
			"mady-router",
			"mady-agent",
		},
		FallbackMsg: fmt.Sprintf("%s 推理功能暂时不可用。", entry.Name),
		Invisible:   true,
	}
}
