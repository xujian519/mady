package adapter

// ClaudeAdapter is the Claude Code adapter. Registered via init().
var ClaudeAdapter = cliAdapter{
	name:      "claude",
	desc:      "Anthropic Claude Code — CLI-based AI coding assistant",
	bin:       "claude",
	subcmd:    "-p",
	maxTokens: 200_000,
	models:    []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-3-5"},
	canEdit:   true,
}

func init() { RegisterAdapter(ClaudeAdapter) }
