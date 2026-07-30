package provisions

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// OrchestratorAgentConfig 返回专利编排器的 Agent 配置。
//
// 专利编排器（patent-orchestrator）是专利业务的总调度入口，
// 负责识别意图→路由到条款智能体→检查结果→交付用户的完整流程。
//
// 与 PatentAgentConfig 的关系：
//   - PatentAgentConfig 是所有工具的拥有者（tools + extensions + lifecycles）
//   - OrchestratorAgentConfig 是轻量的策略层（prompt + handoffs + checkers）
//     不重复注册 tools（由 runOrchestration 统一入口调用）
//
// 使用方式：
//   ShowCheckersTool 在扩展检查器体系内部注册，编排器通过 suggest_checkers /
//   run_checker_review 工具调用检查器。
// =============================================================================

// OrchestratorSystemPrompt 返回专利编排器的 System Prompt。
func OrchestratorSystemPrompt(manifest *PatentManifest) string {
	var b strings.Builder

	b.WriteString("你是 Mady 的专利编排器（patent-orchestrator），专利业务总调度入口。\n")
	b.WriteString("用简体中文回复，专业严谨。\n\n")

	b.WriteString("## 工作流程\n\n")
	b.WriteString("1. 分析用户需求，识别涉及的专利条款（新颖性/创造性/实用性等）\n")
	b.WriteString("2. 调用 transfer_to_provision-* 委派专项分析给对应的条款智能体\n")
	b.WriteString("3. 收集各条款智能体的分析结果\n")
	b.WriteString("4. 调用 suggest_checkers / run_checker_review 进行质量复核\n")
	b.WriteString("5. 综合所有结果，向用户输出完整的专利分析报告\n\n")

	b.WriteString("## 可用条款智能体\n\n")
	for _, entry := range manifest.Provisions {
		if !entry.PreRegister {
			continue
		}
		fmt.Fprintf(&b, "- transfer_to_%s: %s（%s）\n", entry.Worker, entry.Name, strings.Join(entry.LegalBasis, "、"))
	}
	b.WriteString("\n## 可用推理模式\n\n")
	for _, entry := range manifest.Reasoning {
		if !entry.PreRegister {
			continue
		}
		fmt.Fprintf(&b, "- transfer_to_%s: %s（服务 %s）\n", entry.Worker, entry.Name, strings.Join(entry.Serves, "、"))
	}

	b.WriteString("\n## 质量复核\n\n")
	b.WriteString("- 使用 suggest_checkers(artifact_path) 查看适用的检查器\n")
	b.WriteString("- 使用 run_checker_review(role_id, content) 执行复核\n")
	b.WriteString("- 关键节点（新颖性/创造性结论）必须经过复核\n\n")

	b.WriteString("## IPC 领域专家（按需启用）\n\n")
	b.WriteString("当 technical-analyzer 输出了 ipcHints（如 A61、G06），或案件明显涉及特定技术领域时：\n")
	b.WriteString("1. 解析 ipcHints 中的 IPC 大类\n")
	b.WriteString("2. 按需调用 domain-{IPC}-{suffix} 获取领域特定的审查标准\n")
	b.WriteString("3. 将领域专家的结论注入对应条款智能体的分析中\n")
	b.WriteString("已知 IPC 领域：A61（医学）、G06（计算）、H04（通信）、C07（化学）、C12（生物）、G01（测量）、B60（车辆）、F16（机械）、H01（电气）、E04（建筑）\n")
	b.WriteString("示例：IPC=G06 时调用 transfer_to_domain-G06-inventiveness 获取软件领域的创造性审查标准\n\n")

	b.WriteString("## 约束规则\n")
	b.WriteString("- 必须引用具体法律条文，不得编造对比文件或法条\n")
	b.WriteString("- 各条款智能体返回的结果直接使用，不需要解释切换过程\n")
	b.WriteString("- 信息不足时明确标注，不得强行结论\n")
	b.WriteString("- 最终报告须附声明：\"本分析由 AI 辅助生成，不构成正式法律意见。\"\n")

	return b.String()
}

// OrchestratorHandoffConfig 返回专利编排器的 Handoff 配置。
func OrchestratorHandoffConfig(manifest *PatentManifest, base agentcore.Config) agentcore.HandoffConfig {
	cfg := base
	cfg.Name = "patent-orchestrator"
	cfg.SystemPrompt = OrchestratorSystemPrompt(manifest)
	cfg.Extensions = nil
	cfg.Lifecycle = nil

	return agentcore.HandoffConfig{
		Name:        "patent-orchestrator",
		Description: "专利业务总调度——从意图识别到条款分析到质量复核的完整流程编排。",
		Mode:        agentcore.HandoffDelegate,
		AgentConfig: cfg,
		AllowedSources: []string{
			"patent-agent",
			"mady-router",
			"mady-agent",
		},
		FallbackMsg: "专利编排器暂时不可用，建议稍后重试或直接使用 run_orchestration 工具。",
		Invisible:   false, // 编排器对用户可见，用户可感知任务委派过程
	}
}
