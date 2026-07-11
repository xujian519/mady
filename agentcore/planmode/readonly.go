package planmode

import (
	"encoding/json"
	"strings"
)

// readOnlyCommands are bash commands that don't modify state.
// Classification is conservative: if uncertain, treat as write.
var readOnlyCommands = map[string]bool{
	// File inspection
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"wc": true, "file": true, "stat": true, "du": true, "df": true,
	// Search
	"grep": true, "rg": true, "find": true, "locate": true, "which": true,
	// Listing
	"ls": true, "tree": true, "lsof": true,
	// Diff
	"diff": true, "comm": true,
	// Git read-only
	"git": true, // git itself is allowed; subcommands are checked
	// Process info
	"ps": true, "top": true, "htop": true, "killall": false,
	// Network read
	"ping": true, "dig": true, "nslookup": true, "host": true,
	// Dev tools (read-only invocations)
	"go":     true, // go test, go vet, go list are read; go build writes
	"cargo":  true,
	"node":   true,
	"python": true, "python3": true,
	"ruby": true,
	// Text processing (no side effects)
	"sort": true, "uniq": true, "cut": true, "tr": true, "awk": true,
	"sed": false, // sed -i writes
	// Environment
	"echo": true, "printf": true, "env": true, "printenv": true,
	"date": true, "whoami": true, "id": true, "uname": true,
	"true": true, "false": true,
	// Misc
	"seq": true, "yes": true, "test": true, "[": true,
}

// writeSubcommands classifies commands that have both read and write modes.
// Key: command name, Value: set of read-only subcommands.
var readSubcommands = map[string]map[string]bool{
	"git": {
		"status": true, "diff": true, "log": true, "show": true,
		"branch": true, "remote": true, "tag": true, "stash": true,
		"blame": true, "shortlog": true, "describe": true,
		"ls-files": true, "ls-remote": true, "rev-parse": true,
		"config": true, "help": true,
	},
}

// writeBashCommands are always blocked.
var writeBashCommands = map[string]bool{
	"rm": true, "rmdir": true, "mkdir": true, "cp": true,
	"mv": true, "ln": true, "chmod": true, "chown": true,
	"touch": true, "truncate": true,
	"dd": true, "mkfs": true,
	"kill": true, "killall": true, "pkill": true,
	"shutdown": true, "reboot": true,
	"apt": true, "brew": true, "npm": true, "pip": true,
	"make": true, "cmake": true,
	"docker": true, "kubectl": true,
	"scp": true, "rsync": true, "ftp": true, "sftp": true,
	"curl": true, "wget": true,
}

// isReadOnlyBashCommand checks whether a bash command is read-only.
func isReadOnlyBashCommand(args json.RawMessage) bool {
	if len(args) == 0 {
		return false
	}

	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return false
	}

	cmd, ok := m["command"].(string)
	if !ok || cmd == "" {
		return false
	}

	return isReadOnlyCommandString(cmd)
}

// isReadOnlyCommandString classifies a raw command string as read-only.
func isReadOnlyCommandString(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}

	// Reject command chaining with &&, ||, ;, | unless we can verify
	// all parts are read-only. For safety, treat chained commands as write.
	if hasChainingOperator(cmd) {
		parts := splitChainedCommands(cmd)
		for _, part := range parts {
			if !isReadOnlyCommandString(part) {
				return false
			}
		}
		return true
	}

	// Reject output redirection
	if strings.Contains(cmd, ">") || strings.Contains(cmd, ">>") {
		return false
	}

	// Parse the first token (the command name)
	tokens := strings.Fields(cmd)
	if len(tokens) == 0 {
		return false
	}

	bin := tokens[0]

	// Check write commands first
	if writeBashCommands[bin] {
		return false
	}

	// Check known read-only commands
	if readOnlyCommands[bin] {
		// For commands with subcommands, verify the subcommand is read-only
		if subs, ok := readSubcommands[bin]; ok {
			if len(tokens) > 1 {
				sub := tokens[1]
				if strings.HasPrefix(sub, "-") && len(tokens) > 2 {
					sub = tokens[2]
				}
				return subs[sub]
			}
			// Command without subcommand: allow if it's inherently read-only
			return bin == "git" // bare git is harmless
		}
		return true
	}

	// Unknown command: fail-closed
	return false
}

func hasChainingOperator(cmd string) bool {
	for _, op := range []string{"&&", "||", ";", "|"} {
		if strings.Contains(cmd, op) {
			return true
		}
	}
	return false
}

func splitChainedCommands(cmd string) []string {
	var parts []string
	// Simple split on known operators
	for _, op := range []string{"&&", "||", ";", "|"} {
		cmd = strings.ReplaceAll(cmd, op, "\n")
	}
	for _, line := range strings.Split(cmd, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return parts
}
