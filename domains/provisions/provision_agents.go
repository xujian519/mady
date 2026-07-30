package provisions

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// Tier A — 条款智能体工厂
// =============================================================================

// BuildProvisionSystemPrompt 为条款智能体构建 System Prompt。
func BuildProvisionSystemPrompt(entry *ProvisionManifestEntry) string {
	var b strings.Builder

	fmt.Fprintf(&b, "你是 Mady 的%s。\n", entry.Name)
	fmt.Fprintf(&b, "法条依据：%s\n\n", strings.Join(entry.LegalBasis, "、"))

	if len(entry.ConceptIDs) > 0 {
		fmt.Fprintf(&b, "核心概念：%s\n\n", strings.Join(entry.ConceptIDs, "、"))
	}

	b.WriteString("分析步骤：\n")
	for i, step := range entry.MethodologySteps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	b.WriteString("\n")

	b.WriteString("约束规则：\n")
	b.WriteString("- 必须引用具体法律条文，不得编造对比文件或法条\n")
	b.WriteString("- 信息不足时明确标注，不得强行结论\n")
	b.WriteString("- 使用 knowledge_search / search_knowledge 检索知识库后再下结论\n")
	b.WriteString("- 输出结构化分析，含 legal_basis 字段\n")
	b.WriteString("- 用简体中文回复\n")

	if entry.ExistingSubgraph != "" {
		fmt.Fprintf(&b, "\n注意：Mady 已有 %s 的专用分析工具。\n", entry.Name)
		b.WriteString("使用原则：\n")
		b.WriteString("- 优先调用专用工具（而非纯 LLM 推理），确保分析结果可重复、可追溯\n")
		b.WriteString("- 收集必要的输入参数（权利要求、对比文件等），按工具参数要求传入\n")
		if entry.ToolHints != "" {
			fmt.Fprintf(&b, "- 工具指引：%s\n", entry.ToolHints)
		}
		b.WriteString("- 工具返回的结果直接采纳，不需要重复分析\n")
		b.WriteString("- 信息不足时使用 knowledge_search 补充知识，再调用工具\n")
	}

	return b.String()
}

// ProvisionToHandoff 将一条 Manifest 条目转换为 HandoffConfig。
func ProvisionToHandoff(entry *ProvisionManifestEntry, base agentcore.Config) agentcore.HandoffConfig {
	cfg := base
	cfg.Name = entry.Worker
	cfg.SystemPrompt = BuildProvisionSystemPrompt(entry)

	// 限制工具集：防止条款智能体使用危险工具
	cfg.Extensions = nil // 不继承父 Agent 的扩展，保持轻量
	cfg.Lifecycle = nil

	return agentcore.HandoffConfig{
		Name:        entry.Worker,
		Description: fmt.Sprintf("专利条款分析：%s（%s）。%s", entry.Name, entry.ID, strings.Join(entry.LegalBasis, "、")),
		Mode:        agentcore.HandoffDelegate,
		AgentConfig: cfg,
		AllowedSources: []string{
			"patent-agent", // PatentAgentConfig 默认名称
			"mady-router",  // Router 委派
			"mady-agent",   // UnifiedAgent 委派
		},
		FallbackMsg: fmt.Sprintf("%s 功能暂时不可用，建议稍后重试或咨询专业专利代理人。", entry.Name),
		Invisible:   true, // 内部路由，用户不可见
	}
}

// ProvisionHandoffs 从 Manifest 中提取所有预注册的条款智能体 Handoff 列表。
// base 是基础的 Agent 配置（用于继承 Provider/Model）。
func ProvisionHandoffs(manifest *PatentManifest, base agentcore.Config) []agentcore.HandoffConfig {
	var handoffs []agentcore.HandoffConfig
	for _, entry := range manifest.Provisions {
		if !entry.PreRegister {
			continue
		}
		handoffs = append(handoffs, ProvisionToHandoff(&entry, base))
	}
	return handoffs
}

// ReasoningHandoffs 从 Manifest 中提取所有预注册的推理模式 Handoff 列表。
func ReasoningHandoffs(manifest *PatentManifest, base agentcore.Config) []agentcore.HandoffConfig {
	var handoffs []agentcore.HandoffConfig
	for _, entry := range manifest.Reasoning {
		if !entry.PreRegister {
			continue
		}
		handoffs = append(handoffs, ReasoningToHandoff(&entry, base))
	}
	return handoffs
}

// AllHandoffs 返回 Manifest 中所有预注册的 Handoff（条款 + 推理模式）。
func AllHandoffs(manifest *PatentManifest, base agentcore.Config) []agentcore.HandoffConfig {
	var handoffs []agentcore.HandoffConfig
	handoffs = append(handoffs, ProvisionHandoffs(manifest, base)...)
	handoffs = append(handoffs, ReasoningHandoffs(manifest, base)...)
	return handoffs
}
