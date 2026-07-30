package main

// slash_commands.go defines the TUI slash command registration table.
// set has a single source of truth. It replaces the two-branch switch in
// handleSubmit (prefix match + exact switch) and the parallel static list in
// slash_suggestions.go: both the dispatcher and the autocomplete menu read
// from the same Registry.
//
// Each SlashCommand carries:
//   - Name:    the canonical command token, e.g. "thinking" (without "/").
//   - Aliases: alternate tokens treated as the same command (e.g. "new" for "clear").
//   - Desc:    one-line description for the autocomplete menu and /help.
//   - Match:   decides whether an input line invokes this command. Defaults to
//              exact match on "/<name>" or any alias; prefix commands (thinking,
//              theme, case, skill:) supply a custom Match.
//   - Available: optional gate (e.g. only in multi-domain mode). When it returns
//                false the command is hidden from autocomplete and ignored.
//   - Handler:  runs the command. It receives the session and the full trimmed
//               input line so it can parse its own arguments.
//
// Lookup walks the registry in registration order and returns the first Match;
// this preserves the original short-circuit semantics.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/skill"
)

// buildSlashRegistry registers every TUI slash command. Order matters: more
// specific prefix commands (thinking, theme, case, skill:) must be registered
// before the generic fallback so Lookup short-circuits correctly — mirroring
// the original two-branch dispatch.
func (s *tuiSession) buildSlashRegistry() *Registry {
	r := NewRegistry()

	multiDomain := availableBool(func(s *tuiSession) bool { return true }) // 统一模式后始终可用

	r.Register(SlashCommand{
		Name:     SettingKeyThinking,
		Category: catMode,
		Desc:     "查看或修改推理模式",
		Match:    prefixMatch(SettingKeyThinking),
		Handler:  func(ctx slashCtx) { s.handleThinkingCommand(ctx.input) },
		Args: []ArgSuggestion{
			{Value: DefaultThinking, Description: "默认推理显示（完整思考过程）"},
			{Value: valSummarized, Description: "摘要模式（压缩思考过程）"},
			{Value: valOmitted, Description: "隐藏模式（不显示思考过程）"},
			{Value: valReset, Description: "恢复默认配置"},
		},
	})
	r.Register(SlashCommand{
		Name:     SettingKeyTheme,
		Category: catSettings,
		Desc:     "切换主题",
		Match:    prefixMatch(SettingKeyTheme),
		Handler:  func(ctx slashCtx) { s.handleThemeCommand(ctx.input) },
		Args: []ArgSuggestion{
			{Value: DefaultTheme, Description: "浅色主题"},
			{Value: valDark, Description: "深色主题"},
		},
	})
	r.Register(SlashCommand{
		Name:        "skill",
		Category:    catGeneral,
		Desc:        "显式调用技能（/skill:name [参数]）或列出可用技能",
		Usage:       "/skill:<技能名称> [参数]",
		Examples:    []string{"/skill:patent-agent", "/skill:patent-agent 分析权利要求"},
		Match:       prefixMatch("skill:"),
		SuggestText: "/skill:",
		Handler: func(ctx slashCtx) {
			cmd, ok := skill.ParseCommand(ctx.input)
			if !ok || cmd.Name == "" {
				skills := s.fc.BaseConfig.AvailableSkills
				if len(skills) == 0 {
					s.app.PrintSystem("⚠ 当前没有可用的技能。\n" +
						"技能扫描路径：SKILL_DIR 环境变量、~/.agent、.agent/、skills/、~/.mady/skills/、~/.agents/skills/\n" +
						"请确保在上述路径中存在有效的 SKILL.md 文件。")
					return
				}
				var b strings.Builder
				fmt.Fprintf(&b, "📋 可用技能（共 %d 个）：\n", len(skills))
				for _, sk := range skills {
					fmt.Fprintf(&b, "  /skill:%s  — %s\n", sk.Name, sk.Description)
				}
				b.WriteString("\n用法: /skill:<名称> [参数]\n")
				b.WriteString("示例: /skill:patent-agent 分析权利要求1的新颖性")
				s.app.PrintSystem(b.String())
				return
			}
			if _, found := skill.FindByName(s.fc.BaseConfig.AvailableSkills, cmd.Name); !found {
				names := make([]string, 0, len(s.fc.BaseConfig.AvailableSkills))
				for _, sk := range s.fc.BaseConfig.AvailableSkills {
					names = append(names, sk.Name)
				}
				s.app.PrintSystem(fmt.Sprintf("⚠ 技能 %q 未找到。可用技能：%s", cmd.Name, strings.Join(names, ", ")))
				return
			}
			s.submitInput(ctx.input)
		},
	})

	// 专利分析快捷命令：直接运行 Pregel 工作流，绕过 LLM 意图分类。
	r.Register(SlashCommand{
		Name:     "novelty",
		Category: catCase,
		Desc:     "新颖性/创造性分析：对发明进行技术特征提取、现有技术检索和规则引擎检查",
		Usage:    "/novelty <发明描述>",
		Examples: []string{`/novelty "一种基于深度学习的图像识别方法，包括卷积神经网络..."`},
		Risk:     riskNone,
		Match:    exactMatch("novelty"),
		Handler:  func(ctx slashCtx) { s.handleNoveltySlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "oa",
		Category: catCase,
		Desc:     "审查意见（OA）答复起草：分析通知书文本，生成答复书骨架",
		Usage:    "/oa <OA通知书文本>",
		Examples: []string{`/oa "审查员认为权利要求1不具备新颖性..."`},
		Risk:     riskNone,
		Match:    exactMatch("oa"),
		Handler:  func(ctx slashCtx) { s.handleOASlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "invalidation",
		Category: catCase,
		Desc:     "专利无效宣告分析：识别无效理由，逐项生成论证骨架",
		Usage:    "/invalidation <权利要求文本>",
		Examples: []string{`/invalidation "1. 一种图像处理方法..."`},
		Risk:     riskNone,
		Match:    exactMatch("invalidation"),
		Handler:  func(ctx slashCtx) { s.handleInvalidationSlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "infringement",
		Category: catCase,
		Desc:     "专利侵权比对分析：全面覆盖（字面侵权）+ 等同侵权分析",
		Usage:    "/infringement <权利要求文本> | <被控侵权方案>",
		Examples: []string{`/infringement 1. 一种装置包括A和B。 | 被控产品包含A和C`},
		Risk:     riskNone,
		Match:    exactMatch("infringement"),
		Handler:  func(ctx slashCtx) { s.handleInfringementSlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "reexamination",
		Category: catCase,
		Desc:     "驳回复审请求书起草：解析驳回决定，生成复审请求书骨架",
		Usage:    "/reexamination <驳回决定书文本>",
		Examples: []string{`/reexamination "驳回决定编号：2024-001..."`},
		Risk:     riskNone,
		Match:    exactMatch("reexamination"),
		Handler:  func(ctx slashCtx) { s.handleReexaminationSlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "patent",
		Category: catCase,
		Desc:     "专利分析工具帮助",
		Usage:    "/patent",
		Match:    exactMatch("patent"),
		Handler: func(ctx slashCtx) {
			s.app.PrintSystem("专利分析快捷命令：\n" +
				"  /novelty <描述>            — 新颖性/创造性分析\n" +
				"  /oa <通知书文本>           — OA答复书起草\n" +
				"  /invalidation <权利要求>   — 无效宣告分析\n" +
				"  /infringement <权利要求> | <被控方案> — 侵权比对分析\n" +
				"  /reexamination <驳回决定>  — 复审请求书起草\n" +
				"\n也可以直接在对话中输入自然语言描述需求，AI会自动调用分析工具。")
		},
	})

	// === Inspect 类命令：查看后端已注入但此前对用户不可见的能力 ===
	// 这些子系统在 framework.go 启动时已装配，本组命令提供 TUI 直接查看入口。
	// 参数解析统一走 parseSlashSubcommand / parseSlashRest，与其他命令一致。
	r.Register(SlashCommand{
		Name:     "ledger",
		Category: catInspect,
		Desc:     "查看本轮工具调用证据账本（可视化面板）",
		Usage:    "/ledger",
		Risk:     riskNone,
		Match:    exactMatch("ledger"),
		Handler:  func(ctx slashCtx) { s.openEvidenceOverlay("") },
	})
	r.Register(SlashCommand{
		Name:     "snapshots",
		Category: catInspect,
		Desc:     "列出文件快照历史（每轮写入工具前的文件状态）",
		Usage:    "/snapshots",
		Risk:     riskNone,
		Match:    exactMatch("snapshots"),
		Handler:  func(ctx slashCtx) { s.handleSnapshotsCommand() },
	})
	r.Register(SlashCommand{
		Name:     "undo",
		Category: catInspect,
		Desc:     "回退到指定轮的文件状态（仅对 edit/write_file 等追踪工具生效）",
		Usage:    "/undo <轮号>",
		Examples: []string{"/undo 3"},
		Risk:     "medium",
		Match:    exactMatch("undo"),
		Handler:  func(ctx slashCtx) { s.handleUndoCommand(parseSlashSubcommand(ctx.input, "undo")) },
	})
	r.Register(SlashCommand{
		Name:     "memory",
		Category: catInspect,
		Desc:     "查看长期记忆（跨 User/Session/LongTerm 三层）",
		Usage:    "/memory [关键词]",
		Examples: []string{"/memory", "/memory 偏好深色主题"},
		Risk:     riskNone,
		Match:    prefixMatch("memory"),
		Handler:  func(ctx slashCtx) { s.handleMemoryCommand(parseSlashRest(ctx.input, "memory")) },
	})
	// 证据面板 overlay
	r.Register(SlashCommand{
		Name:     "evidence",
		Category: catInspect,
		Desc:     "查看引用证据详情（工具调用账本或知识检索结果）",
		Usage:    "/evidence [查询关键词]",
		Examples: []string{"/evidence", "/evidence 专利法第22条"},
		Risk:     riskNone,
		Match:    prefixMatch("evidence"),
		Handler: func(ctx slashCtx) {
			query := parseSlashRest(ctx.input, "evidence")
			s.openEvidenceOverlay(query)
		},
	})
	// 系统态 overlay
	r.Register(SlashCommand{
		Name:     "status",
		Category: catInspect,
		Desc:     "查看系统态 — 运行模式、事件、影响",
		Usage:    "/status",
		Risk:     riskNone,
		Match:    exactMatch("status"),
		Handler:  func(ctx slashCtx) { s.openSystemStatus() },
	})
	// 知识库检索
	r.Register(SlashCommand{
		Name:     "knowledge",
		Category: catInspect,
		Desc:     "检索知识库（FTS 全文搜索）",
		Usage:    "/knowledge <关键词>",
		Examples: []string{"/knowledge 专利法第22条", "/knowledge 创造性判断"},
		Risk:     riskNone,
		Match:    prefixMatch("knowledge"),
		Handler:  func(ctx slashCtx) { s.handleKnowledgeCommand(parseSlashRest(ctx.input, "knowledge")) },
	})
	// MCP 服务器管理
	r.Register(SlashCommand{
		Name:     "mcp",
		Category: catInspect,
		Desc:     "查看已注册的 MCP 服务器",
		Usage:    "/mcp",
		Risk:     riskNone,
		Match:    exactMatch("mcp"),
		Handler:  func(ctx slashCtx) { s.handleMCPCommand(parseSlashSubcommand(ctx.input, "mcp")) },
	})
	// 提示词模板浏览
	r.Register(SlashCommand{
		Name:     "prompt",
		Category: catInspect,
		Desc:     "浏览提示词模板",
		Usage:    "/prompt [list|<模板名>]",
		Examples: []string{"/prompt list", "/prompt patent-analysis"},
		Risk:     riskNone,
		Match:    prefixMatch("prompt"),
		Handler:  func(ctx slashCtx) { s.handlePromptCommand(parseSlashRest(ctx.input, "prompt")) },
	})
	// 证据领域规则状态
	r.Register(SlashCommand{
		Name:     "evidence-domain",
		Category: catInspect,
		Desc:     "查看证据判断规则引擎状态",
		Usage:    "/evidence-domain",
		Risk:     riskNone,
		Match:    exactMatch("evidence-domain"),
		Handler:  func(ctx slashCtx) { s.handleEvidenceDomainCommand(parseSlashSubcommand(ctx.input, "evidence-domain")) },
	})

	r.Register(SlashCommand{
		Name:      catMode,
		Category:  catGeneral,
		Desc:      "显示当前 Agent 模式",
		Match:     exactMatch(catMode),
		Available: multiDomain,
		Handler: func(ctx slashCtx) {
			agent, initializing, initErr := s.agentStatus()
			if agent == nil {
				switch {
				case initializing:
					s.app.PrintSystem("Agent 正在初始化，请稍候…")
				case initErr != "":
					s.app.PrintSystem("Agent 初始化失败，请查看日志后重试当前操作。")
				default:
					s.app.PrintSystem("Agent 尚未就绪，请稍候…")
				}
				return
			}
			agentName := agent.Config().Name
			s.app.PrintSystem(fmt.Sprintf("当前 Agent: %s（统一模式）", agentName))
		},
	})
	r.Register(SlashCommand{
		Name:     "deadline",
		Category: catCase,
		Desc:     "显示当前案件期限",
		Match:    exactMatch("deadline"),
		Handler:  func(ctx slashCtx) { s.handleDeadlineCommand() },
	})

	r.Register(SlashCommand{
		Name:     "help",
		Category: catGeneral,
		Desc:     "显示快捷键",
		Match:    exactMatch("help"),
		Handler:  func(ctx slashCtx) { s.app.ToggleKeyHelp() },
	})
	r.Register(SlashCommand{
		Name:     "clear",
		Category: catSession,
		Aliases:  []string{"new"},
		Desc:     "开始新对话",
		Match:    exactMatch("clear", "new"),
		Handler:  func(ctx slashCtx) { s.handleClearCommand() },
	})
	r.Register(SlashCommand{
		Name:     "branch",
		Category: catSession,
		Desc:     "从当前对话创建分支",
		Match:    exactMatch("branch"),
		Handler:  func(ctx slashCtx) { s.handleBranchCommand() },
	})
	r.Register(SlashCommand{
		Name:     "save",
		Category: catSession,
		Desc:     "显示会话保存信息",
		Match:    exactMatch("save"),
		Handler:  func(ctx slashCtx) { s.handleSaveCommand() },
	})
	r.Register(SlashCommand{
		Name:     "copy",
		Category: catGeneral,
		Desc:     "复制最后一条回复",
		Match:    exactMatch("copy"),
		Handler:  func(ctx slashCtx) { s.handleCopyCommand() },
	})
	r.Register(SlashCommand{
		Name:     "export",
		Category: catSession,
		Desc:     "导出当前对话为 Markdown",
		Match:    exactMatch("export"),
		Handler:  func(ctx slashCtx) { s.handleExportCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:     SettingKeyReview,
		Category: catMode,
		Desc:     "切换审核关卡（关键内容人工确认）",
		Match:    exactMatch(SettingKeyReview),
		Handler:  func(ctx slashCtx) { s.handleReviewCommandEx(parseSlashSubcommand(ctx.input, SettingKeyReview)) },
		Args: []ArgSuggestion{
			{Value: "on", Description: "开启审核关卡"},
			{Value: DefaultPlan, Description: "关闭审核关卡"},
			{Value: "status", Description: "查看当前审核状态"},
		},
	})
	r.Register(SlashCommand{
		Name:     "approve",
		Category: catMode,
		Desc:     "确认AI输出，继续执行（审核模式下）",
		Match:    exactMatch("approve"),
		Handler: func(ctx slashCtx) {
			// Gate inside the handler (not via Available) so that when review
			// mode is off the user gets a guiding hint instead of "未知命令".
			if !s.isReviewMode() {
				s.app.PrintSystem("⚠ 审核关卡未启用。使用 /review 开启")
				return
			}
			s.recordApprovalDecision(domains.DecisionAdopted, "", "")
			s.app.PrintSystem("✅ 已确认 — Agent 将继续执行")
			// Hard-interrupt path (e.g. disclosure review_gate): agent loop has
			// exited at an InterruptError and only Resume() can continue it.
			// Fall back to submitInput for ApprovalGate keyword soft-interrupts
			// where the agent is still running and a new "确认" turn suffices.
			if !s.resumeIfInterrupted() {
				s.submitInput("确认")
			}
		},
	})
	r.Register(SlashCommand{
		Name:     "reject",
		Category: catMode,
		Desc:     "拒绝AI输出，请求修改（审核模式下）",
		Match:    exactMatch("reject"),
		Handler: func(ctx slashCtx) {
			if !s.isReviewMode() {
				s.app.PrintSystem("⚠ 审核关卡未启用。使用 /review 开启")
				return
			}
			s.recordApprovalDecision(domains.DecisionRejected, "", "用户拒绝，要求修改")
			s.app.PrintSystem("❌ 已拒绝 — Agent 将根据您的反馈调整")
			s.submitInput("拒绝，请根据审核意见修改后重新输出")
		},
	})
	r.Register(SlashCommand{
		Name:     SettingKeyPlan,
		Category: catMode,
		Desc:     "切换计划模式（高质量推理）",
		Match:    exactMatch(SettingKeyPlan),
		Handler:  func(ctx slashCtx) { s.handlePlanCommandEx(parseSlashSubcommand(ctx.input, SettingKeyPlan)) },
		Args: []ArgSuggestion{
			{Value: "on", Description: "开启计划模式"},
			{Value: DefaultPlan, Description: "关闭计划模式"},
			{Value: "status", Description: "查看当前模式状态"},
		},
	})
	r.Register(SlashCommand{
		Name:     "cmd",
		Desc:     "打开命令中心（搜索并执行所有命令）",
		Category: catGeneral,
		Usage:    "/cmd",
		Match:    exactMatch("cmd"),
		Handler:  func(ctx slashCtx) { s.openCommandCenter() },
	})

	r.Register(SlashCommand{
		Name:     "skills",
		Category: catGeneral,
		Desc:     "打开技能中心，浏览和管理可用技能",
		Usage:    "/skills",
		Examples: []string{"/skills"},
		Match:    exactMatch("skills"),
		Handler:  func(ctx slashCtx) { s.openSkillCenter() },
	})
	r.Register(SlashCommand{
		Name:     catSettings,
		Category: catSettings,
		Desc:     "打开设置面板",
		Match:    exactMatch(catSettings),
		Handler: func(ctx slashCtx) {
			sub := parseSlashSubcommand(ctx.input, catSettings)
			if sub == valReset {
				s.handleSettingsReset()
			} else {
				s.openSettings()
			}
		},
		Args: []ArgSuggestion{
			{Value: valReset, Description: "重置所有设置为默认值"},
		},
	})
	r.Register(SlashCommand{
		Name:     "provider",
		Category: catSettings,
		Desc:     "查看或切换 LLM 提供方",
		Match:    exactMatch("provider"),
		Handler:  func(ctx slashCtx) { s.handleProviderCommand(ctx.input) },
		Args: []ArgSuggestion{
			{Value: "status", Description: "查看当前 Provider"},
			{Value: "list", Description: "列出所有可用 Provider"},
		},
	})
	r.Register(SlashCommand{
		Name:     "model",
		Category: catSettings,
		Desc:     "查看或切换 LLM 模型",
		Match:    exactMatch("model"),
		Handler:  func(ctx slashCtx) { s.handleModelCommand(ctx.input) },
		Args: []ArgSuggestion{
			{Value: "list", Description: "列出所有可用模型"},
		},
	})
	r.Register(SlashCommand{
		Name:     "quit",
		Category: catGeneral,
		Desc:     "退出",
		Match:    func(input string) bool { return input == "/quit" || input == "exit" },
		Handler:  func(ctx slashCtx) { _ = s.app.Stop() },
	})

	// 会话管理命令。
	r.Register(SlashCommand{
		Name:     catSession,
		Category: "manage",
		Desc:     "查看/命名当前会话（/session <名称>）",
		Match:    exactMatch(catSession),
		Handler:  func(ctx slashCtx) { s.handleSessionNameCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:     "sessions",
		Category: "manage",
		Desc:     "管理已保存的会话（选择/切换/重命名/删除）",
		Usage:    "/sessions",
		Match:    exactMatch("sessions"),
		Handler:  func(ctx slashCtx) { s.openSessionSelector() },
	})

	return r
}
