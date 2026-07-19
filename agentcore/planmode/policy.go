package planmode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Decision reports whether plan mode blocks a tool call.
type Decision struct {
	Blocked bool
	Message string
}

// Policy configures which tools are allowed during plan mode.
type Policy struct {
	// AllowedTools is a whitelist of additional read-only tools to permit.
	// The core read-only tools (read, ls, grep, glob, view, etc.) are
	// always allowed based on their ReadOnly flag.
	AllowedTools []string
}

// blockedTools are always blocked in plan mode regardless of settings.
var blockedTools = map[string]bool{
	"edit":       true,
	"write_file": true,
	"patch":      true,
	"delete":     true,
	"move":       true,
}

// alwaysAllowed are tools that should always work even in plan mode.
var alwaysAllowed = map[string]bool{
	"ask":  true,
	"todo": true,
}

// Decide evaluates whether a tool call should be blocked in plan mode.
//
// Decision logic (fail-closed):
//  1. Explicitly blocked tools (edit, write_file, delete, move, patch) → blocked
//  2. Always-allowed tools (ask, todo) → allowed
//  3. Whitelisted tools → allowed
//  4. Read-only tools → allowed
//  5. Bash with read-only command → allowed
//  6. Everything else → blocked
func (p Policy) Decide(toolName string, readOnly bool, args json.RawMessage) Decision {
	// Tool names are matched case-insensitively so that "Edit", "EDIT", and
	// "edit" are treated identically. All map keys below are lowercase.
	name := strings.ToLower(toolName)

	if blockedTools[name] {
		return Decision{
			Blocked: true,
			Message: fmt.Sprintf("计划模式下禁止使用写入工具 %s", toolName),
		}
	}

	if alwaysAllowed[name] {
		return Decision{}
	}

	for _, allowed := range p.AllowedTools {
		if strings.EqualFold(allowed, toolName) {
			return Decision{}
		}
	}

	if readOnly {
		return Decision{}
	}

	if name == "bash" {
		if isReadOnlyBashCommand(args) {
			return Decision{}
		}
	}

	return Decision{
		Blocked: true,
		Message: fmt.Sprintf("计划模式下仅允许只读操作，%s 可能产生副作用", toolName),
	}
}
