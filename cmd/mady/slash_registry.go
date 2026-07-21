package main

// slash_registry.go defines a registry for TUI slash commands so the command
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
	"sort"
	"strings"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/fuzzy"
	"github.com/xujian519/mady/tui/core"
)

// slashCtx is passed to every Handler. It carries the session (for state +
// agent rebuild) and the full input line. Handlers must not assume the input
// has been validated beyond the Match check.
type slashCtx struct {
	s     *tuiSession
	input string
}

// slashHandler executes one slash command.
type slashHandler func(ctx slashCtx)

// SlashCommand describes one registered slash command.
type SlashCommand struct {
	Name    string
	Aliases []string
	Desc    string
	// Category groups commands visually: "general"|"mode"|"session"|"case"|"settings".
	Category string
	// Usage is a one-line syntax hint, e.g. "/plan [on|off|status]".
	Usage string
	// Examples shows typical invocations (optional).
	Examples []string
	// Risk signals destructive potential: "none"|"destructive"|"data_loss".
	Risk  string
	Match func(input string) bool
	// Available returns (ok, reason). When ok is false, reason explains why
	// the command is unavailable (shown in autocomplete and /help).
	Available func(s *tuiSession) (bool, string)
	Handler   slashHandler
	// SuggestText overrides the autocomplete insert text. When empty,
	// Suggestions uses "/" + Name. Set this for commands whose trigger token
	// is not exactly "/" + Name (e.g. "/skill:" whose Name is "skill").
	SuggestText string
}

// availableBool wraps a legacy func(s *tuiSession) bool into the new
// (bool, string) signature, returning "" as the reason when unavailable.
func availableBool(fn func(s *tuiSession) bool) func(s *tuiSession) (bool, string) {
	if fn == nil {
		return nil
	}
	return func(s *tuiSession) (bool, string) {
		if fn(s) {
			return true, ""
		}
		return false, ""
	}
}

// Registry is an ordered collection of SlashCommands.
type Registry struct {
	cmds []SlashCommand
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{} }

// Register appends a command. Registration order is the lookup order.
func (r *Registry) Register(c SlashCommand) { r.cmds = append(r.cmds, c) }

// exactMatch matches "/name" or "/name " exactly, or any "/alias".
func exactMatch(name string, aliases ...string) func(string) bool {
	return func(input string) bool {
		if !strings.HasPrefix(input, "/") {
			return false
		}
		// Match "/name" exactly or "/name " followed by anything-but-the-name.
		if input == "/"+name || strings.HasPrefix(input, "/"+name+" ") {
			return true
		}
		for _, a := range aliases {
			if input == "/"+a || strings.HasPrefix(input, "/"+a+" ") {
				return true
			}
		}
		return false
	}
}

// prefixMatch matches any input starting with "/name" (used by commands that
// take sub-arguments without a space, e.g. "/skill:foo").
func prefixMatch(name string) func(string) bool {
	return func(input string) bool {
		return strings.HasPrefix(input, "/"+name)
	}
}

// Lookup returns the first registered command whose Match accepts the input
// and whose Available (if set) permits it. Returns nil when no command matches.
func (r *Registry) Lookup(input string, s *tuiSession) *SlashCommand {
	for i := range r.cmds {
		c := &r.cmds[i]
		if c.Available != nil {
			if ok, _ := c.Available(s); !ok {
				continue
			}
		}
		if c.Match(input) {
			return c
		}
	}
	return nil
}

// Suggestions builds the autocomplete list from every available command,
// producing one core.Suggestion per canonical name (aliases are not listed
// separately to keep the menu compact).
func (r *Registry) Suggestions(s *tuiSession) []core.Suggestion {
	var out []core.Suggestion
	for _, c := range r.cmds {
		if c.Available != nil {
			if ok, _ := c.Available(s); !ok {
				continue
			}
		}
		// SuggestText lets a command advertise a trigger that is not exactly
		// "/" + Name — e.g. "/skill:" whose Name is "skill". Without this the
		// menu would suggest "/skill", which the prefix matcher then rejects.
		text := c.SuggestText
		if text == "" {
			text = "/" + c.Name
		}
		out = append(out, core.Suggestion{
			InsertText:  text,
			Label:       text,
			Description: c.Desc,
			GroupLabel:  c.Category,
		})
	}
	return out
}

// Suggest returns up to 3 registered command names whose Levenshtein distance
// from the extracted token is ≤ 3, ranked closest first. Only commands that
// are currently available are considered. Used by handleSubmit to produce
// "你是不是想输入 /xxx？" hints for unknown commands.
func (r *Registry) Suggest(input string, s *tuiSession) []string {
	// Extract the command token: "/themes" → "themes", "/theme dark" → "theme"
	tok := strings.TrimPrefix(input, "/")
	if sp := strings.IndexByte(tok, ' '); sp >= 0 {
		tok = tok[:sp]
	}
	if tok == "" {
		return nil
	}

	type scored struct {
		name string
		dist int64
	}
	var candidates []scored
	for _, c := range r.cmds {
		if c.Available != nil {
			if ok, _ := c.Available(s); !ok {
				continue
			}
		}
		d := fuzzy.LevenshteinDistance(tok, c.Name)
		if d <= 3 {
			candidates = append(candidates, scored{c.Name, d})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].dist < candidates[j].dist })

	var out []string
	for i, c := range candidates {
		if i >= 3 {
			break
		}
		out = append(out, c.name)
	}
	return out
}

// parseSlashSubcommand extracts the first argument after the command name.
// Example: parseSlashSubcommand("/plan on", "plan") → "on"
// Example: parseSlashSubcommand("/plan", "plan") → ""
func parseSlashSubcommand(input, cmdName string) string {
	prefix := "/" + cmdName
	if !strings.HasPrefix(input, prefix) {
		return ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(input, prefix))
	if sp := strings.IndexByte(rest, ' '); sp >= 0 {
		return rest[:sp]
	}
	return rest
}

// buildSlashRegistry registers every// buildSlashRegistry registers every TUI slash command. Order matters: more
// specific prefix commands (thinking, theme, case, skill:) must be registered
// before the generic fallback so Lookup short-circuits correctly — mirroring
// the original two-branch dispatch.
func (s *tuiSession) buildSlashRegistry() *Registry {
	r := NewRegistry()

	multiDomain := availableBool(func(s *tuiSession) bool { return s.useMultiDomain })

	r.Register(SlashCommand{
		Name:     "thinking",
		Category: "mode",
		Desc:     "查看或修改推理模式",
		Match:    prefixMatch("thinking"),
		Handler:  func(ctx slashCtx) { s.handleThinkingCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:     "theme",
		Category: "settings",
		Desc:     "切换主题",
		Match:    prefixMatch("theme"),
		Handler:  func(ctx slashCtx) { s.handleThemeCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:     "case",
		Category: "case",
		Desc:     "查看或切换案件",
		Match: func(input string) bool {
			return input == "/case" || strings.HasPrefix(input, "/case ")
		},
		Handler: func(ctx slashCtx) { s.handleCaseCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:        "skill",
		Category:    "general",
		Desc:        "显式调用技能",
		Match:       prefixMatch("skill:"),
		SuggestText: "/skill:",
		Handler: func(ctx slashCtx) {
			s.app.PrintSystem("mady tui 简化版未加载技能，请使用 example/cli-chat 配合 SKILL_DIRS")
		},
	})

	// 专利分析快捷命令：直接运行 Pregel 工作流，绕过 LLM 意图分类。
	r.Register(SlashCommand{
		Name:     "novelty",
		Category: "case",
		Desc:     "新颖性/创造性分析：对发明进行技术特征提取、现有技术检索和规则引擎检查",
		Usage:    "/novelty <发明描述>",
		Examples: []string{`/novelty "一种基于深度学习的图像识别方法，包括卷积神经网络..."`},
		Risk:     "none",
		Match:    exactMatch("novelty"),
		Handler:  func(ctx slashCtx) { s.handleNoveltySlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "oa",
		Category: "case",
		Desc:     "审查意见（OA）答复起草：分析通知书文本，生成答复书骨架",
		Usage:    "/oa <OA通知书文本>",
		Examples: []string{`/oa "审查员认为权利要求1不具备新颖性..."`},
		Risk:     "none",
		Match:    exactMatch("oa"),
		Handler:  func(ctx slashCtx) { s.handleOASlash(ctx) },
	})
	r.Register(SlashCommand{
		Name:     "patent",
		Category: "case",
		Desc:     "专利分析工具帮助",
		Usage:    "/patent",
		Match:    exactMatch("patent"),
		Handler: func(ctx slashCtx) {
			s.app.PrintSystem("专利分析快捷命令：\n" +
				"  /novelty <描述>    — 新颖性/创造性分析\n" +
				"  /oa <通知书文本>   — OA答复书起草\n" +
				"\n也可以直接在对话中输入自然语言描述需求，AI会自动调用分析工具。")
		},
	})

	r.Register(SlashCommand{
		Name:      "mode",
		Category:  "general",
		Desc:      "显示当前 Agent 模式",
		Match:     exactMatch("mode"),
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
			s.app.PrintSystem(fmt.Sprintf("当前 Agent: %s（多域路由模式）", agentName))
		},
	})
	r.Register(SlashCommand{
		Name:     "deadline",
		Category: "case",
		Desc:     "显示当前案件期限",
		Match:    exactMatch("deadline"),
		Handler:  func(ctx slashCtx) { s.handleDeadlineCommand() },
	})

	r.Register(SlashCommand{
		Name:     "help",
		Category: "general",
		Desc:     "显示快捷键",
		Match:    exactMatch("help"),
		Handler:  func(ctx slashCtx) { s.app.ToggleKeyHelp() },
	})
	r.Register(SlashCommand{
		Name:     "clear",
		Category: "session",
		Aliases:  []string{"new"},
		Desc:     "开始新对话",
		Match:    exactMatch("clear", "new"),
		Handler:  func(ctx slashCtx) { s.handleClearCommand() },
	})
	r.Register(SlashCommand{
		Name:     "branch",
		Category: "session",
		Desc:     "从当前对话创建分支",
		Match:    exactMatch("branch"),
		Handler:  func(ctx slashCtx) { s.handleBranchCommand() },
	})
	r.Register(SlashCommand{
		Name:     "save",
		Category: "session",
		Desc:     "显示会话保存信息",
		Match:    exactMatch("save"),
		Handler:  func(ctx slashCtx) { s.handleSaveCommand() },
	})
	r.Register(SlashCommand{
		Name:     "copy",
		Category: "general",
		Desc:     "复制最后一条回复",
		Match:    exactMatch("copy"),
		Handler:  func(ctx slashCtx) { s.handleCopyCommand() },
	})
	r.Register(SlashCommand{
		Name:     "export",
		Category: "session",
		Desc:     "导出当前对话为 Markdown",
		Match:    exactMatch("export"),
		Handler:  func(ctx slashCtx) { s.handleExportCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:     "review",
		Category: "mode",
		Desc:     "切换审核关卡（关键内容人工确认）",
		Match:    exactMatch("review"),
		Handler:  func(ctx slashCtx) { s.handleReviewCommandEx(parseSlashSubcommand(ctx.input, "review")) },
	})
	r.Register(SlashCommand{
		Name:     "approve",
		Category: "mode",
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
		Category: "mode",
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
		Name:     "plan",
		Category: "mode",
		Desc:     "切换计划模式（高质量推理）",
		Match:    exactMatch("plan"),
		Handler:  func(ctx slashCtx) { s.handlePlanCommandEx(parseSlashSubcommand(ctx.input, "plan")) },
	})
	r.Register(SlashCommand{
		Name:     "cmd",
		Desc:     "打开命令中心（搜索并执行所有命令）",
		Category: "general",
		Usage:    "/cmd",
		Match:    exactMatch("cmd"),
		Handler:  func(ctx slashCtx) { s.openCommandCenter() },
	})

	r.Register(SlashCommand{
		Name:     "settings",
		Category: "settings",
		Desc:     "打开设置面板",
		Match:    exactMatch("settings"),
		Handler: func(ctx slashCtx) {
			sub := parseSlashSubcommand(ctx.input, "settings")
			if sub == "reset" {
				s.handleSettingsReset()
			} else {
				s.openSettings()
			}
		},
	})
	r.Register(SlashCommand{
		Name:     "quit",
		Category: "general",
		Desc:     "退出",
		Match:    func(input string) bool { return input == "/quit" || input == "exit" },
		Handler:  func(ctx slashCtx) { _ = s.app.Stop() },
	})

	return r
}
