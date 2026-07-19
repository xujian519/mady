package planmode

import (
	"encoding/json"
	"strings"
)

// readOnlyCommands are bash commands that don't modify state.
// Classification is conservative: if uncertain, treat as write.
//
// NOTE: general-purpose interpreters (python, node, ruby, ...) are
// intentionally absent — their -c/-e flags can execute arbitrary code
// (file deletion, network exfiltration) without any shell redirection
// operator, which the redirect check below cannot detect. They are
// fail-closed by virtue of not being listed here.
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
	// Git read-only (subcommands are checked below)
	"git": true,
	// Go read-only (subcommands are checked below): go test/vet/list are
	// read; go run/build/generate execute code or write output.
	"go": true,
	// Process info
	"ps": true, "top": true, "htop": true,
	// Network read
	"ping": true, "dig": true, "nslookup": true, "host": true,
	// Text processing (emit to stdout only, cannot execute code).
	"sort": true, "uniq": true, "cut": true, "tr": true,
	// sed (-i) and awk (system()/redirect inside the program) can mutate
	// state → fail-closed.
	"sed": false, "awk": false,
	// Environment
	"echo": true, "printf": true, "env": true, "printenv": true,
	"date": true, "whoami": true, "id": true, "uname": true,
	"true": true, "false": true,
	// Misc
	"seq": true, "yes": true, "test": true, "[": true,
}

// readSubcommands classifies commands that have both read and write modes.
// Key: command name, Value: set of read-only subcommands. A subcommand not
// present here is treated as write (fail-closed).
var readSubcommands = map[string]map[string]bool{
	"git": {
		"status": true, "diff": true, "log": true, "show": true,
		"branch": true, "remote": true, "tag": true, "stash": true,
		"blame": true, "shortlog": true, "describe": true,
		"ls-files": true, "ls-remote": true, "rev-parse": true,
		"config": true, "help": true,
	},
	"go": {
		"test": true, "vet": true, "list": true, "show": true,
		"doc": true, "version": true, "env": true, "help": true,
		"bug": true,
		// build/run/install/get/mod/fmt/generate write files or execute
		// code → not whitelisted.
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
	// all parts are read-only. Operators inside quotes are ignored.
	parts := splitCommandChain(cmd)
	if len(parts) > 1 {
		for _, part := range parts {
			if !isReadOnlyCommandString(part) {
				return false
			}
		}
		return true
	}

	// Reject output redirection (>, >>) occurring outside quotes.
	if strings.Contains(stripQuoted(cmd), ">") {
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
			// Command without subcommand: allow only for git (bare git
			// prints usage and exits). go/cargo with no subcommand are
			// blocked to avoid surprises.
			return bin == "git"
		}
		return true
	}

	// Unknown command: fail-closed
	return false
}

// stripQuoted returns a copy of s with single- and double-quoted regions
// replaced by spaces (preserving length), so that operators or redirections
// that appear inside quotes — e.g. the pipe in `grep "a|b"` — are not
// mistaken for shell operators. Backslash escapes inside double quotes are
// respected.
func stripQuoted(s string) string {
	var b strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
				b.WriteByte(' ')
			} else {
				b.WriteByte(' ')
			}
		case inDouble:
			switch {
			case c == '\\' && i+1 < len(s):
				b.WriteByte(' ')
				b.WriteByte(' ')
				i++
			case c == '"':
				inDouble = false
				b.WriteByte(' ')
			default:
				b.WriteByte(' ')
			}
		default:
			switch c {
			case '\'':
				inSingle = true
				b.WriteByte(' ')
			case '"':
				inDouble = true
				b.WriteByte(' ')
			default:
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// splitCommandChain splits a command string on shell operators
// (&&, ||, ;, |) that occur outside single- or double-quoted regions. It
// returns the trimmed sub-commands; if no top-level operator is present it
// returns a single-element slice. A single trailing '&' (background) is also
// treated as a separator.
func splitCommandChain(cmd string) []string {
	var parts []string
	var seg strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if s := strings.TrimSpace(seg.String()); s != "" {
			parts = append(parts, s)
		}
		seg.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
			seg.WriteByte(c)
		case inDouble:
			switch {
			case c == '\\' && i+1 < len(cmd):
				seg.WriteByte(c)
				if i+1 < len(cmd) {
					seg.WriteByte(cmd[i+1])
					i++
				}
			case c == '"':
				inDouble = false
				seg.WriteByte(c)
			default:
				seg.WriteByte(c)
			}
		default:
			switch c {
			case '\'':
				inSingle = true
				seg.WriteByte(c)
			case '"':
				inDouble = true
				seg.WriteByte(c)
			case ';', '|':
				// ';' separates statements; '|' pipes. '||' is two adjacent
				// '|' which collapses naturally into one split point.
				flush()
			case '&':
				if i+1 < len(cmd) && cmd[i+1] == '&' {
					i++ // consume second '&'
					flush()
				} else {
					// single trailing '&' = background; treat as separator
					flush()
				}
			default:
				seg.WriteByte(c)
			}
		}
	}
	flush()
	if len(parts) == 0 {
		return []string{strings.TrimSpace(cmd)}
	}
	return parts
}
