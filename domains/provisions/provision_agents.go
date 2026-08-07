package provisions

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// filterTools 按允许名称列表过滤工具集。
func filterTools(allowed []string, all []*agentcore.Tool) []*agentcore.Tool {
	if len(allowed) == 0 {
		return all
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	filtered := make([]*agentcore.Tool, 0, len(allowed))
	for _, t := range all {
		if _, ok := allowedSet[t.Name]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

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
//
//nolint:dupl // ProvisionToHandoff and ReasoningToHandoff share the same HandoffConfig boilerplate
func ProvisionToHandoff(entry *ProvisionManifestEntry, base agentcore.Config) agentcore.HandoffConfig {
	cfg := base
	cfg.Name = entry.Worker
	cfg.SystemPrompt = BuildProvisionSystemPrompt(entry)

	// 限制工具集：防止条款智能体使用危险工具
	cfg.Extensions = nil // 不继承父 Agent 的扩展，保持轻量
	cfg.Lifecycle = nil

	allowedTools := entry.PrimaryTools
	if len(allowedTools) == 0 {
		allowedTools = DefaultPatentTools
	}
	cfg.Tools = filterTools(allowedTools, cfg.Tools)

	return agentcore.HandoffConfig{
		Name:        entry.Worker,
		Description: fmt.Sprintf("专利条款分析：%s（%s）。%s", entry.Name, entry.ID, strings.Join(entry.LegalBasis, "、")),
		Mode:        agentcore.HandoffDelegate,
		AgentConfig: cfg,
		AllowedSources: []string{
			"patent-agent",        // PatentAgentConfig 默认名称
			"patent-orchestrator", // 编排器委派
			"mady-router",         // Router 委派
			"mady-agent",          // UnifiedAgent 委派
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

// BuildProvisionListForSystemPrompt 根据 Manifest 构建条款智能体列表字符串。
// 用于 PatentAgentConfig 的系统提示词中的"条款智能体（专业分析委派）"段落，
// 替代原有的硬编码列表，使列表随 manifest.yaml 自动更新。
func BuildProvisionListForSystemPrompt(manifest *PatentManifest) string {
	var b strings.Builder

	b.WriteString("## 条款智能体（专业分析委派）\n")
	b.WriteString("对专利法各条款的专项分析，可使用 transfer_to_provision-* 工具委派给对应的条款智能体：\n")

	for _, entry := range manifest.Provisions {
		if !entry.PreRegister {
			continue
		}
		fmt.Fprintf(&b, "- transfer_to_%s → %s（%s）\n", entry.Worker, abbreviateProvisionName(entry.Name), abbreviateLegalBasis(entry.LegalBasis))
	}

	b.WriteString("对跨条款的推理步骤，可使用 transfer_to_reasoning-* 委派给对应的推理模式。\n")
	b.WriteString("委派完成后直接使用结果，不需要解释切换过程。\n")

	return b.String()
}

// abbreviateLegalBasis 将法条依据列表缩写成简短引用字符串。
// "专利法第22条第2款" → "A22.2"
// 多个法条用 "/" 分隔。
func abbreviateLegalBasis(basis []string) string {
	if len(basis) == 0 {
		return ""
	}
	var parts []string
	for _, b := range basis {
		parts = append(parts, abbreviateOneBasis(b))
	}
	return strings.Join(parts, "/")
}

// abbreviateOneBasis 将单条法条依据缩写。
// "专利法第22条第2款" → "A22.2"
func abbreviateOneBasis(basis string) string {
	basis = strings.TrimPrefix(basis, "专利法")
	if idx := strings.Index(basis, "（"); idx >= 0 {
		basis = basis[:idx]
	}
	basis = strings.ReplaceAll(basis, "第", "")
	basis = strings.ReplaceAll(basis, "条", ".")
	basis = strings.ReplaceAll(basis, "款", "")
	basis = strings.ReplaceAll(basis, "项", "")
	basis = strings.ReplaceAll(basis, "之", ".")
	basis = strings.TrimRight(basis, ".")
	if !strings.HasPrefix(basis, "A") && !strings.HasPrefix(basis, "细则") {
		basis = "A" + basis
	}
	basis = strings.ReplaceAll(basis, "实施细则", "细则")
	return basis
}

// abbreviateProvisionName 将条款智能体名称缩短为简要分析描述。
// "新颖性条款智能体" → "新颖性分析"
func abbreviateProvisionName(name string) string {
	name = strings.TrimSuffix(name, "条款智能体")
	name = strings.ReplaceAll(name, "条款", "")
	name = strings.TrimSpace(name)
	if name != "" {
		return name + "分析"
	}
	return "专业分析"
}

// =============================================================================
