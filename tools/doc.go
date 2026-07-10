// Package tools provides a built-in tool extension for the Mady agent framework.
//
// Tools are registered as a single agentcore.Extension and provide 40+ capabilities:
//
//	File operations:   read, write, edit, delete, move, patch
//	Shell:             bash (local/SSH), process management
//	Search:            ls, grep, find, glob, fuzzy (fzf-style)
//	Browser:           Playwright, Camofox, AgentBrowser providers
//	Web:               web_search, web_fetch
//	Code execution:    Python, JavaScript, Go (sandboxed via PTC)
//	Git:               commit, diff, log, branch, checkout, merge
//	MCP bridge:        delegate tool calls to MCP servers
//	Vision:            image analysis API integration
//	macOS:             desktop control (volume, clipboard, notifications)
//
// Usage:
//
//	ext := tools.NewExtension(tools.ExtensionConfig{WorkingDir: "/project"})
//	agent := agentcore.New(agentcore.Config{Extensions: []agentcore.Extension{ext}})
//
// Each tool can be overridden at construction time for remote execution or
// different backends via the Options pattern.
package tools
