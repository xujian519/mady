package domains

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/internal/intentrules"
)

// Domain names used for intent classification and routing.
//
// 注意：这些常量定义在 domains 根包而非 domains/router/ 子包中，是因为
// ClassifyIntent 需要引用 internal/intentrules，而 router/ 子包若导入
// domains 根包会形成循环依赖（根包 imports router/, router/ imports 根包）。
// 如果未来将 ClassifyIntent 独立到单独的 classifier 包，这些常量可一并
// 迁移到 domains/router/。
const (
	DomainChat      = "chat"
	DomainAssistant = "assistant"
	DomainPatent    = "patent"
	DomainLegal     = "legal"
)

// ClassifyIntent analyzes user input and returns the target domain name.
// This is a simple keyword-based classifier; a future version should use
// LLM-based classification for better accuracy.
func ClassifyIntent(input string) string {
	lower := strings.ToLower(input)

	for _, kw := range intentrules.PatentKeywords {
		if strings.Contains(lower, kw) {
			return DomainPatent
		}
	}
	for _, kw := range intentrules.LegalKeywords {
		if strings.Contains(lower, kw) {
			return DomainLegal
		}
	}
	for _, kw := range intentrules.AssistantKeywords {
		if strings.Contains(lower, kw) {
			return DomainAssistant
		}
	}

	return DomainChat
}

// ProfessionalHandoffConfigs 返回专业领域（Patent/Legal）的 HandoffConfig 列表，
// 供 UnifiedAgentConfig 和 RouterConfigFromManifests 共享使用。
// 不包含 chat/assistant（已合并进 UnifiedAgent）。
//
// AllowedSources 包含 "mady-router"（遗留 Router 委派）和 "mady-agent"
// （统一 Agent 委派），两者都是受信任的调度入口。
// 扩展此白名单需要安全审阅。不包含 "*" 通配符，防止未授权 Agent 触发专业领域委派。
func ProfessionalHandoffConfigs(base agentcore.Config, patentToolExt agentcore.Extension, legalToolExt agentcore.Extension) []agentcore.HandoffConfig {
	return []agentcore.HandoffConfig{
		{
			Name:           DomainPatent,
			Description:    "专利代理与知识产权分析。处理专利检索、权利要求分析、新颖性比对。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    PatentAgentConfig(base, patentToolExt),
			AllowedSources: []string{"mady-router", "mady-agent"},
			FallbackMsg:    "专利分析功能暂时不可用，建议稍后重试或联系专业代理人。",
		},
		{
			Name:           DomainLegal,
			Description:    "法律咨询与研究。处理法条检索、判例检索、法律分析。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    LegalAgentConfig(base, legalToolExt),
			AllowedSources: []string{"mady-router", "mady-agent"},
			FallbackMsg:    "法律分析功能暂时不可用，建议稍后重试或咨询专业律师。",
		},
	}
}

// domainFactoryMap 将领域名称映射到对应的 Agent 工厂函数。
// RouterConfigFromManifests 使用此映射将声明式 Manifest 转换
// 为可执行的 HandoffConfig。
//
// chat 和 assistant 均映射到 UnifiedAgentConfig（三合一后不再区分）。
// 工厂函数接受 toolExt 参数，由 RouterConfigFromManifests 透传。
var domainFactoryMap = map[string]func(agentcore.Config, agentcore.Extension) agentcore.Config{
	DomainChat:      UnifiedAgentFunc,
	DomainAssistant: UnifiedAgentFunc,
	DomainPatent:    PatentAgentFunc,
	DomainLegal:     LegalAgentFunc,
}

// UnifiedAgentFunc 是 UnifiedAgentConfig 的适配器，适配 domainFactoryMap 签名。
// 在 Router 模式下 chat/assistant 无需 Handoff 子 Agent 工具，忽略 toolExt。
func UnifiedAgentFunc(base agentcore.Config, _ agentcore.Extension) agentcore.Config {
	return base
}

// PatentAgentFunc 是 PatentAgentConfig 的适配器，适配 domainFactoryMap 签名。
func PatentAgentFunc(base agentcore.Config, toolExt agentcore.Extension) agentcore.Config {
	return PatentAgentConfig(base, toolExt)
}

// LegalAgentFunc 是 LegalAgentConfig 的适配器，适配 domainFactoryMap 签名。
func LegalAgentFunc(base agentcore.Config, toolExt agentcore.Extension) agentcore.Config {
	return LegalAgentConfig(base, toolExt)
}

// RouterConfigFromManifests 从 AgentManifest 列表构建 Router Agent 配置。
// 它扫描 manifests，将每个 manifest 映射到对应的领域工厂函数，
// 生成 HandoffConfig 条目。
//
// 不在 factoryMap 中的 domain 会被自动跳过（不做 fallback），
// 因为入口已在 ScanManifests 阶段验证过 domain 有效性。
// manifests 为空时返回仅含 base 的配置（无 Handoff）。
func RouterConfigFromManifests(base agentcore.Config, manifests []agentcore.AgentManifest, toolExt agentcore.Extension) agentcore.Config {
	if len(manifests) == 0 {
		return base
	}

	base.Name = "mady-router"

	base.SystemPrompt = buildRouterSystemPrompt(manifests)

	var handoffs []agentcore.HandoffConfig
	for _, m := range manifests {
		factory, ok := domainFactoryMap[m.Domain]
		if !ok {
			continue
		}
		handoffs = append(handoffs, agentcore.HandoffConfig{
			Name:           m.Name,
			Description:    m.Description,
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    factory(base, toolExt),
			AllowedSources: []string{"mady-router", "mady-agent"}, // 与 ProfessionalHandoffConfigs 对齐，不使用通配符
			FallbackMsg:    fmt.Sprintf("%s 功能暂时不可用，请稍后再试。", m.Description),
		})
	}

	base.Handoffs = handoffs
	return base
}

// buildRouterSystemPrompt 基于 manifest 列表动态构建 Router 的 System Prompt。
func buildRouterSystemPrompt(manifests []agentcore.AgentManifest) string {
	var b strings.Builder
	b.WriteString("你是 Mady（中观智能体）的调度路由 Agent。\n")
	b.WriteString("你的职责是分析用户意图，将请求路由到对应的领域专家：\n")
	b.WriteString("\n")

	for _, m := range manifests {
		name := m.Name
		desc := m.Description
		fmt.Fprintf(&b, "- %s: %s\n", name, desc)
	}

	b.WriteString("\n")
	b.WriteString("识别到专业领域问题时，使用 transfer_to_<domain> 工具将任务委派给对应专家。\n")
	b.WriteString("一般对话和无法明确分类的请求，自己直接回答即可。\n")
	return b.String()
}

// appendLifecycle composes lifecycle hooks safely (delegates to agentcore.AppendLifecycle).
func appendLifecycle(existing, next agentcore.LifecycleHook) agentcore.LifecycleHook { //nolint:staticcheck
	return agentcore.AppendLifecycle(existing, next)
}

// ProjectHandoffName 返回规范化的案件 Handoff 目标名称。
func ProjectHandoffName(projectID string) string {
	return "project-" + projectID
}
