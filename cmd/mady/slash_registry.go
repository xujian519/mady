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
	"sort"
	"strings"

	"github.com/xujian519/mady/fuzzy"
	"github.com/xujian519/mady/tui/core"
)

// Slash command category labels.
const (
	catGeneral  = "general"
	catMode     = "mode"
	catSettings = "settings"
	catCase     = "case"
	catSession  = "session"
	catInspect  = "inspect"
)

// Slash command risk levels.
const riskNone = "none"

// Common slash command argument values.
const (
	valSummarized = "summarized"
	valOmitted    = "omitted"
	valReset      = "reset"
	valDark       = "dark"
)

// SlashArgProvider is an AutocompleteProvider that suggests command arguments
// when the cursor is after a complete slash command (e.g. "/theme ").
// It implements FullInputProvider to access the full input buffer.
type SlashArgProvider struct {
	registry *Registry
}

// NewSlashArgProvider creates a provider that reads command args from the registry.
func NewSlashArgProvider(r *Registry) *SlashArgProvider {
	return &SlashArgProvider{registry: r}
}

// Trigger returns "" (always-active provider).
func (p *SlashArgProvider) Trigger() string { return "" }

// Complete returns empty suggestions; use CompleteWithFull instead.
func (p *SlashArgProvider) Complete(_ string) []core.Suggestion { return nil }

// CompleteWithFull returns argument suggestions when the buffer contains a
// completed slash command at the cursor position.
func (p *SlashArgProvider) CompleteWithFull(token string, fullValue string, cursorPos int64) []core.Suggestion {
	if len(fullValue) == 0 {
		return nil
	}
	runes := []rune(fullValue)
	if cursorPos < 0 || cursorPos > int64(len(runes)) {
		return nil
	}
	// Must start with "/" to be a slash command context
	if len(runes) == 0 || runes[0] != '/' {
		return nil
	}
	// Find the end of the command name: position of space before cursor
	// Using strings.LastIndexByte for clarity and robustness.
	spaceIdx := -1
	end := int(cursorPos)
	if end > len(runes) {
		end = len(runes)
	}
	for i := end - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			spaceIdx = i
			break
		}
	}
	// We need at least "/name " prefix
	if spaceIdx < 1 || runes[spaceIdx] != ' ' {
		return nil
	}
	// Extract command name: "/name " → "name" (trim trailing spaces just in case)
	cmdName := strings.TrimRight(string(runes[1:spaceIdx]), " ")
	if cmdName == "" {
		return nil
	}
	// Look up the command
	for _, c := range p.registry.cmds {
		if len(c.Args) == 0 {
			continue
		}
		if c.Name == cmdName {
			return filterSuggestions(c, token)
		}
		for _, alias := range c.Aliases {
			if alias == cmdName {
				return filterSuggestions(c, token)
			}
		}
	}
	return nil
}

// filterSuggestions builds a filtered suggestion list from a command's Args,
// matching suggestions whose Value has the given token as a prefix.
func filterSuggestions(c SlashCommand, token string) []core.Suggestion {
	out := make([]core.Suggestion, 0, len(c.Args))
	for _, arg := range c.Args {
		if token != "" && !strings.HasPrefix(strings.ToLower(arg.Value), strings.ToLower(token)) {
			continue
		}
		out = append(out, core.Suggestion{
			Label:       arg.Value,
			InsertText:  arg.Value,
			Description: arg.Description,
			GroupLabel:  c.Category,
		})
	}
	return out
}

// slashCtx is passed to every Handler. It carries the session (for state +
// agent rebuild) and the full input line. Handlers must not assume the input
// has been validated beyond the Match check.
type slashCtx struct {
	s     *tuiSession
	input string
}

// slashHandler executes one slash command.
type slashHandler func(ctx slashCtx)

// ArgSuggestion describes a possible sub-command argument for autocomplete.
type ArgSuggestion struct {
	// Value is the argument text (e.g. "light", "on", "default").
	Value string
	// Description explains what this argument does (e.g. "浅色主题").
	Description string
}

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
	// Args lists sub-command arguments for inline autocomplete.
	// When non-empty, typing "/name " will show these as completions.
	Args []ArgSuggestion
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

// parseSlashRest extracts everything after the command name's first space,
// preserving multi-word arguments (unlike parseSlashSubcommand which takes
// only the first token).
// Example: parseSlashRest("/memory 用户偏好 深色", "memory") → "用户偏好 深色"
// Example: parseSlashRest("/memory", "memory") → ""
func parseSlashRest(input, cmdName string) string {
	prefix := "/" + cmdName
	if !strings.HasPrefix(input, prefix) {
		return ""
	}
	// rest 形如 " args..." 或 ""（无参数）。不 TrimSpace，保留分隔空格作为边界。
	rest := input[len(prefix):]
	sp := strings.IndexByte(rest, ' ')
	if sp < 0 {
		// 没有空格 → input 恰好是 "/cmdName"，无参数
		return ""
	}
	return strings.TrimSpace(rest[sp+1:])
}
