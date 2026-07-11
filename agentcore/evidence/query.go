package evidence

import (
	"strings"
)

// HasSuccessfulWrite reports whether every path was the subject of a successful
// write tool call this turn.
func (l *Ledger) HasSuccessfulWrite(paths []string) bool {
	return l.hasSuccessfulPaths(paths, func(r Receipt) bool { return r.Write })
}

// HasSuccessfulReadOrWrite reports whether every path was successfully read or
// written this turn.
func (l *Ledger) HasSuccessfulReadOrWrite(paths []string) bool {
	return l.hasSuccessfulPaths(paths, func(r Receipt) bool { return r.Read || r.Write })
}

// HasSuccessfulCommand reports whether a specific bash command ran successfully
// this turn. The match is on the command subject (trimmed prefix).
func (l *Ledger) HasSuccessfulCommand(command string) bool {
	command = strings.TrimSpace(command)
	if l == nil || command == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts {
		if r.Success && r.ToolName == "bash" && commandMatches(command, r.Command) {
			return true
		}
	}
	return false
}

// HasFailedCommand reports whether the cited command ran but exited non-zero.
func (l *Ledger) HasFailedCommand(command string) bool {
	command = strings.TrimSpace(command)
	if l == nil || command == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts {
		if !r.Success && r.ToolName == "bash" && commandMatches(command, r.Command) {
			return true
		}
	}
	return false
}

// TouchedPaths returns up to limit distinct paths from this turn's successful
// receipts, most recent first. writtenOnly restricts to write receipts.
func (l *Ledger) TouchedPaths(limit int, writtenOnly bool) []string {
	if l == nil || limit <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for i := len(l.receipts) - 1; i >= 0 && len(out) < limit; i-- {
		r := l.receipts[i]
		if !r.Success {
			continue
		}
		if writtenOnly && !r.Write {
			continue
		}
		if !writtenOnly && !r.Read && !r.Write {
			continue
		}
		for _, p := range r.Paths {
			if !seen[p] && len(out) < limit {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// HasAnySuccessfulReceipt reports whether any tool succeeded this turn.
func (l *Ledger) HasAnySuccessfulReceipt() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts {
		if r.Success {
			return true
		}
	}
	return false
}

// HasWriteOrCommandSince reports whether a successful write or command receipt
// was recorded at or after the given index.
func (l *Ledger) HasWriteOrCommandSince(index int) bool {
	if l == nil {
		return false
	}
	if index < 0 {
		index = 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := index; i < len(l.receipts); i++ {
		r := l.receipts[i]
		if r.Success && (r.Write || r.Command != "") {
			return true
		}
	}
	return false
}

// SuccessfulCommands returns up to limit successful bash commands, most recent
// first, for self-correction hints in rejection messages.
func (l *Ledger) SuccessfulCommands(limit int) []string {
	if l == nil || limit <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for i := len(l.receipts) - 1; i >= 0 && len(out) < limit; i-- {
		r := l.receipts[i]
		if r.Success && r.ToolName == "bash" && r.Command != "" {
			out = append(out, r.Command)
		}
	}
	return out
}

func (l *Ledger) hasSuccessfulPaths(paths []string, accept func(Receipt) bool) bool {
	wanted := pathSet(paths)
	if l == nil || len(wanted) == 0 {
		return false
	}
	found := map[string]bool{}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts {
		if !r.Success || !accept(r) {
			continue
		}
		for _, p := range r.Paths {
			if wanted[p] {
				found[p] = true
			}
		}
	}
	return len(found) == len(wanted)
}

// commandMatches checks whether the actual command contains the requested one.
// A simple substring match after trimming, sufficient for the verification use
// cases (exact command or prefix).
func commandMatches(wanted, actual string) bool {
	wanted = strings.TrimSpace(wanted)
	actual = strings.TrimSpace(actual)
	if wanted == "" {
		return false
	}
	return strings.HasPrefix(actual, wanted) || actual == wanted
}

func pathSet(paths []string) map[string]bool {
	out := make(map[string]bool, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			out[p] = true
		}
	}
	return out
}
