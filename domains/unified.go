package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
	"github.com/xujian519/mady/tools"
)

// UnifiedAgentConfig 构建合并后的统一 Agent 配置。
//
// 融合了原 Chat Agent（对话/情感陪伴）、Assistant Agent（工具执行）
// 和 Router（领域路由）三者的能力。用户面对的唯一智能体入口，
// 内部通过 Invisible Handoff 委派专利/法律专业任务。
func UnifiedAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "mady-agent"

	// 统一场景需要足够轮次：对话 + 工具链式调用。
	if cfg.MaxTurns == 0 || cfg.MaxTurns > 100 {
		cfg.MaxTurns = 100
	}

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady（中观智能体），用户的所有对话和任务都经过你。",
		"用简体中文回复，语气自然友好，像同事而不是客服。",
		"",
		"【能力范围】",
		"- 日常对话、情感交流和倾听陪伴",
		"- 信息检索与网页搜索（使用 web_search / web_fetch 工具）",
		"- 学术论文检索（使用 scholar_search 工具）",
		"- 代码生成、阅读和修改（使用 read / write_file / edit 工具）",
		"- 文件操作和项目管理（使用 ls / glob / grep / find 工具）",
		"- 内容创作、数据整理和导出",
		"",
		"【专业任务路由】",
		"当用户提出专利或法律领域问题，使用 transfer_to_* 工具委派给领域专家：",
		"- transfer_to_patent → 专利代理与知识产权分析（专利检索、权利要求分析、新颖性比对）",
		"- transfer_to_legal → 法律咨询与研究（法条检索、判例检索、法律分析）",
		"委派完成后直接向用户呈现结果，不需要解释切换过程。",
		"",
		"【工具使用原则】",
		"使用工具前先简要说明你要做什么，执行完给出结构化结果。",
		"不确定的专业问题建议用户咨询相关专业人士。",
	}, "\n")

	// DoomLoop: 死循环检测器。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())

	// ReasoningStrategy: 根据问题复杂度动态调整推理 effort/budget，
	// 并在系统提示中注入策略提示（如 StepByStep / StructuredAnalysis）。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewReasoningStrategyRouter(
			agentcore.NewDefaultClassifier(),
			agentcore.NewDefaultStrategySelector(),
		),
	)

	// Guardrail: LevelLight — 统一使用轻量护栏。
	// 安全防护未来通过人机协作和 plan 模式替代。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.New(
			guardrails.WithLevel(guardrails.LevelLight),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		)),
	)

	// 工具扩展 — 沿用 Assistant Agent 的完整配置。
	// WorkingDir 从 base.ProjectDir 透传，回退到 base.WorkspaceDir。
	// SandboxEnabled=true 确保文件操作被限制在项目目录内。
	workingDir := base.ProjectDir
	if workingDir == "" {
		workingDir = base.WorkspaceDir
	}
	allowRead, allowWrite := BuildSandboxAllowLists()
	toolExt := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: base.Provider,
			Model:    base.Model,
		},
		WebSearch:   &tools.WebSearchToolConfig{},
		WebFetch:    &tools.WebFetchToolConfig{},
		ComputerUse: true,
		MaxBytes:    100 * 1024,
		MaxLines:    5000,
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode,
			tools.ToolProcess,
		},
	})

	// 心理引擎 — 轻量模式：VAD/OCC 语气调整，不做认知扭曲诊断。
	cfg.Extensions = append(cfg.Extensions, toolExt, psychological.NewExtension(
		psychological.Config{SkipDistortionDetection: true},
	))

	// 注册专业领域 Handoff（Patent/Legal），标记为不可见。
	cfg.Handoffs = ProfessionalHandoffConfigs(base)
	for i := range cfg.Handoffs {
		cfg.Handoffs[i].Invisible = true
	}

	return cfg
}
