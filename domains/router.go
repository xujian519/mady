package domains

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// Domain names used for intent classification and routing.
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

	// Patent-related keywords
	patentKeywords := []string{
		"专利", "权利要求", "发明", "实用新型", "外观设计",
		"新颖性", "创造性", "实用性", "prior art", "现有技术",
		"patent", "invention", "claim", "IPC", "分类号",
		"pct", "巴黎公约", "优先权",
	}
	for _, kw := range patentKeywords {
		if strings.Contains(lower, kw) {
			return DomainPatent
		}
	}

	// Legal-related keywords
	legalKeywords := []string{
		"法律", "法条", "法规", "判例", "判决", "裁定",
		"诉讼", "起诉", "被告", "原告", "法院", "法官",
		"合同", "侵权", "赔偿", "证据", "仲裁",
		"刑法", "民法", "行政法", "公司法", "劳动法",
		"司法解释", "指导性案例",
		"law", "legal", "court", "statute", "regulation",
	}
	for _, kw := range legalKeywords {
		if strings.Contains(lower, kw) {
			return DomainLegal
		}
	}

	// Assistant-related keywords (task execution, code, file ops)
	// Note: "分析" is intentionally excluded to avoid conflict with patent/legal.
	assistantKeywords := []string{
		"查一下", "帮我搜", "搜索", "检索", "查找",
		"起草", "生成", "写一个", "创建", "整理", "导出", "统计",
		"写代码", "实现一个", "调试", "优化", "重构",
		"代码", "编程", "python", "javascript", "go语言",
		"bash", "shell", "命令行", "脚本",
	}
	for _, kw := range assistantKeywords {
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
// AllowedSources 包含 "mady-router"（遗留 Router 委派）、"chat-agent"
// （遗留集成模式委派）和 "mady-agent"（统一 Agent 委派），三者都是受信任的调度入口。
// 扩展此白名单需要安全审阅。不包含 "*" 通配符，防止未授权 Agent 触发专业领域委派。
func ProfessionalHandoffConfigs(base agentcore.Config) []agentcore.HandoffConfig {
	return []agentcore.HandoffConfig{
		{
			Name:           DomainPatent,
			Description:    "专利代理与知识产权分析。处理专利检索、权利要求分析、新颖性比对。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    PatentAgentConfig(base),
			AllowedSources: []string{"mady-router", "chat-agent", "mady-agent"},
			FallbackMsg:    "专利分析功能暂时不可用，建议稍后重试或联系专业代理人。",
		},
		{
			Name:           DomainLegal,
			Description:    "法律咨询与研究。处理法条检索、判例检索、法律分析。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    LegalAgentConfig(base),
			AllowedSources: []string{"mady-router", "chat-agent", "mady-agent"},
			FallbackMsg:    "法律分析功能暂时不可用，建议稍后重试或咨询专业律师。",
		},
	}
}

// domainFactoryMap 将领域名称映射到对应的 Agent 工厂函数。
// RouterConfigFromManifests 使用此映射将声明式 Manifest 转换
// 为可执行的 HandoffConfig。
//
// chat 和 assistant 均映射到 UnifiedAgentConfig（三合一后不再区分）。
var domainFactoryMap = map[string]func(agentcore.Config) agentcore.Config{
	DomainChat:      UnifiedAgentConfig,
	DomainAssistant: UnifiedAgentConfig,
	DomainPatent:    PatentAgentConfig,
	DomainLegal:     LegalAgentConfig,
}

// RouterConfigFromManifests 从 AgentManifest 列表构建 Router Agent 配置。
// 它扫描 manifests，将每个 manifest 映射到对应的领域工厂函数，
// 生成 HandoffConfig 条目。
//
// 不在 factoryMap 中的 domain 会被自动跳过（不做 fallback），
// 因为入口已在 ScanManifests 阶段验证过 domain 有效性。
// manifests 为空时返回仅含 base 的配置（无 Handoff）。
func RouterConfigFromManifests(base agentcore.Config, manifests []agentcore.AgentManifest) agentcore.Config {
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
			AgentConfig:    factory(base),
			AllowedSources: []string{"mady-router", "chat-agent", "mady-agent"}, // 与 ProfessionalHandoffConfigs 对齐，不使用通配符
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
func appendLifecycle(existing, next agentcore.LifecycleHook) agentcore.LifecycleHook {
	return agentcore.AppendLifecycle(existing, next)
}

// ProjectHandoffName 返回规范化的案件 Handoff 目标名称。
func ProjectHandoffName(projectID string) string {
	return "project-" + projectID
}
