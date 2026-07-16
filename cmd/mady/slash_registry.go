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
	"strings"

	"github.com/xujian519/mady/domains"
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
	Name      string
	Aliases   []string
	Desc      string
	Match     func(input string) bool
	Available func(s *tuiSession) bool
	Handler   slashHandler
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
	tokens := map[string]bool{"": true}
	_ = tokens["/"+name]
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
		if c.Available != nil && !c.Available(s) {
			continue
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
		if c.Available != nil && !c.Available(s) {
			continue
		}
		out = append(out, core.Suggestion{
			InsertText:  "/" + c.Name,
			Label:       "/" + c.Name,
			Description: c.Desc,
		})
	}
	return out
}

// buildSlashRegistry registers every TUI slash command. Order matters: more
// specific prefix commands (thinking, theme, case, skill:) must be registered
// before the generic fallback so Lookup short-circuits correctly — mirroring
// the original two-branch dispatch.
func (s *tuiSession) buildSlashRegistry() *Registry {
	r := NewRegistry()

	multiDomain := func(s *tuiSession) bool { return s.useMultiDomain }
	reviewOn := func(s *tuiSession) bool { return s.reviewMode }

	r.Register(SlashCommand{
		Name:    "thinking",
		Desc:    "查看或修改推理模式",
		Match:   prefixMatch("thinking"),
		Handler: func(ctx slashCtx) { s.handleThinkingCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:    "theme",
		Desc:    "切换主题",
		Match:   prefixMatch("theme"),
		Handler: func(ctx slashCtx) { s.handleThemeCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name: "case",
		Desc: "查看或切换案件",
		Match: func(input string) bool {
			return input == "/case" || strings.HasPrefix(input, "/case ")
		},
		Handler: func(ctx slashCtx) { s.handleCaseCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:  "skill",
		Desc:  "显式调用技能",
		Match: prefixMatch("skill:"),
		Handler: func(ctx slashCtx) {
			s.app.PrintSystem("mady tui 简化版未加载技能，请使用 example/cli-chat 配合 SKILL_DIRS")
		},
	})

	r.Register(SlashCommand{
		Name:      "mode",
		Desc:      "显示当前 Agent 模式",
		Match:     exactMatch("mode"),
		Available: multiDomain,
		Handler: func(ctx slashCtx) {
			agentName := s.currentAgent.Config().Name
			s.app.PrintSystem(fmt.Sprintf("当前 Agent: %s（多域路由模式）", agentName))
		},
	})
	r.Register(SlashCommand{
		Name:    "deadline",
		Desc:    "显示当前案件期限",
		Match:   exactMatch("deadline"),
		Handler: func(ctx slashCtx) { s.handleDeadlineCommand() },
	})

	r.Register(SlashCommand{
		Name:    "help",
		Desc:    "显示快捷键",
		Match:   exactMatch("help"),
		Handler: func(ctx slashCtx) { s.app.ToggleKeyHelp() },
	})
	r.Register(SlashCommand{
		Name:    "clear",
		Aliases: []string{"new"},
		Desc:    "开始新对话",
		Match:   exactMatch("clear", "new"),
		Handler: func(ctx slashCtx) { s.handleClearCommand() },
	})
	r.Register(SlashCommand{
		Name:    "branch",
		Desc:    "从当前对话创建分支",
		Match:   exactMatch("branch"),
		Handler: func(ctx slashCtx) { s.handleBranchCommand() },
	})
	r.Register(SlashCommand{
		Name:    "save",
		Desc:    "显示会话保存信息",
		Match:   exactMatch("save"),
		Handler: func(ctx slashCtx) { s.handleSaveCommand() },
	})
	r.Register(SlashCommand{
		Name:    "copy",
		Desc:    "复制最后一条回复",
		Match:   exactMatch("copy"),
		Handler: func(ctx slashCtx) { s.handleCopyCommand() },
	})
	r.Register(SlashCommand{
		Name:    "export",
		Desc:    "导出当前对话为 Markdown",
		Match:   exactMatch("export"),
		Handler: func(ctx slashCtx) { s.handleExportCommand(ctx.input) },
	})
	r.Register(SlashCommand{
		Name:    "review",
		Desc:    "切换审核关卡（关键内容人工确认）",
		Match:   exactMatch("review"),
		Handler: func(ctx slashCtx) { s.handleReviewCommand() },
	})
	r.Register(SlashCommand{
		Name:      "approve",
		Desc:      "确认AI输出，继续执行（审核模式下）",
		Match:     exactMatch("approve"),
		Available: reviewOn,
		Handler: func(ctx slashCtx) {
			s.recordApprovalDecision(domains.DecisionAdopted, "", "")
			s.app.PrintSystem("✅ 已确认 — Agent 将继续执行")
			s.submitInput("确认")
		},
	})
	r.Register(SlashCommand{
		Name:      "reject",
		Desc:      "拒绝AI输出，请求修改（审核模式下）",
		Match:     exactMatch("reject"),
		Available: reviewOn,
		Handler: func(ctx slashCtx) {
			s.recordApprovalDecision(domains.DecisionRejected, "", "用户拒绝，要求修改")
			s.app.PrintSystem("❌ 已拒绝 — Agent 将根据您的反馈调整")
			s.submitInput("拒绝，请根据审核意见修改后重新输出")
		},
	})
	r.Register(SlashCommand{
		Name:    "plan",
		Desc:    "切换计划模式（高质量推理）",
		Match:   exactMatch("plan"),
		Handler: func(ctx slashCtx) { s.handlePlanCommand() },
	})
	r.Register(SlashCommand{
		Name:    "settings",
		Desc:    "打开设置面板",
		Match:   exactMatch("settings"),
		Handler: func(ctx slashCtx) { s.openSettings() },
	})
	r.Register(SlashCommand{
		Name:    "quit",
		Desc:    "退出",
		Match:   func(input string) bool { return input == "/quit" || input == "exit" },
		Handler: func(ctx slashCtx) { _ = s.app.Stop() },
	})

	return r
}
