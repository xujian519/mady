package adapter

// CodexAdapter is the Codex CLI adapter. Registered via init().
var CodexAdapter = cliAdapter{
	name:      "codex",
	desc:      "OpenAI Codex CLI — terminal-based AI coding assistant",
	bin:       "codex",
	subcmd:    "exec",
	maxTokens: 128_000,
	models:    []string{"gpt-5", "gpt-5-mini", "o4-mini"},
	canEdit:   true,
}

func init() { RegisterAdapter(CodexAdapter) }
