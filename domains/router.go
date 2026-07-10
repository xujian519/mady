package domains

import (
	"context"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/workflow"
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

// RouterConfig builds the Router Agent configuration.
// It sets up domain sub-agents as HandoffDelegate targets so the
// Router can dispatch work to the appropriate domain specialist.
func RouterConfig(base agentcore.Config) agentcore.Config {
	return RouterConfigWithClassifier(base, nil)
}

// RouterConfigWithClassifier builds the Router Agent configuration with an
// optional IntentClassifier. If classifier is nil, KeywordClassifier is used.
func RouterConfigWithClassifier(base agentcore.Config, classifier IntentClassifier) agentcore.Config {
	base.Name = "mady-router"

	base.SystemPrompt = strings.Join([]string{
		"你是 Mady（中观智能体）的调度路由 Agent。",
		"你的职责是分析用户意图，将请求路由到对应的领域专家：",
		"",
		"- chat-agent: 日常聊天与情感陪伴。处理问候、闲聊、情绪支持等纯对话场景。",
		"- assistant-agent: 通用智能助理。处理代码生成、文件操作、网页搜索、数据分析等工具密集型任务。",
		"- patent-agent: 专利检索、权利要求分析、新颖性比对、专利申请文书",
		"- legal-advisor: 法条检索、判例检索、法律分析、法律文书",
		"",
		"识别到专业领域问题时，使用 transfer_to_<domain> 工具将任务委派给对应专家。",
		"一般对话和无法明确分类的请求，自己直接回答即可。",
	}, "\n")

	base.Handoffs = []agentcore.HandoffConfig{
		{
			Name:           DomainChat,
			Description:    "日常聊天与情感陪伴。处理问候、闲聊、情绪支持等纯对话场景。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    ChatAgentConfig(base),
			AllowedSources: []string{"*"}, // 任何 Agent 都可以交回给 Chat
			FallbackMsg:    "聊天模块暂时不可用，请稍后再试。",
		},
		{
			Name:           DomainAssistant,
			Description:    "通用智能助理。处理代码生成、文件操作、网页搜索、数据分析等工具密集型任务。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    AssistantAgentConfig(base),
			AllowedSources: []string{"mady-router"}, // 仅 Router 可发起交接
			FallbackMsg:    "这个任务处理遇到点问题，要不你换个方式再说一遍，或稍后再试？",
		},
		{
			Name:           DomainPatent,
			Description:    "专利代理与知识产权分析。处理专利检索、权利要求分析、新颖性比对。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    PatentAgentConfig(base),
			AllowedSources: []string{"mady-router"}, // 仅 Router 可发起交接
			FallbackMsg:    "专利分析功能暂时不可用，建议稍后重试或联系专业代理人。",
		},
		{
			Name:           DomainLegal,
			Description:    "法律咨询与研究。处理法条检索、判例检索、法律分析。",
			Mode:           agentcore.HandoffDelegate,
			AgentConfig:    LegalAgentConfig(base),
			AllowedSources: []string{"mady-router"}, // 仅 Router 可发起交接
			FallbackMsg:    "法律分析功能暂时不可用，建议稍后重试或咨询专业律师。",
		},
	}

	return base
}

// RouterStep returns a workflow.Router that classifies intent and routes
// to the appropriate domain sub-graph (as a Step).
func RouterStep(chatStep, assistantStep, patentStep, legalStep workflow.Step) workflow.Step {
	return RouterStepWithClassifier(chatStep, assistantStep, patentStep, legalStep, nil)
}

// RouterStepWithClassifier returns a workflow.Router with an optional classifier.
func RouterStepWithClassifier(chatStep, assistantStep, patentStep, legalStep workflow.Step, classifier IntentClassifier) workflow.Step {
	if classifier == nil {
		classifier = &KeywordClassifier{}
	}
	return &workflow.Router{
		Route: func(ctx context.Context, input string) string {
			domain, _, err := classifier.Classify(ctx, input)
			if err != nil {
				domain = DomainChat // fall back to chat on error
			}
			switch domain {
			case DomainAssistant:
				return DomainAssistant
			case DomainPatent:
				return DomainPatent
			case DomainLegal:
				return DomainLegal
			default:
				return DomainChat
			}
		},
		Steps: map[string]workflow.Step{
			DomainChat:      chatStep,
			DomainAssistant: assistantStep,
			DomainPatent:    patentStep,
			DomainLegal:     legalStep,
		},
	}
}

// appendLifecycle composes lifecycle hooks safely (delegates to agentcore.AppendLifecycle).
func appendLifecycle(existing, next agentcore.LifecycleHook) agentcore.LifecycleHook {
	return agentcore.AppendLifecycle(existing, next)
}
