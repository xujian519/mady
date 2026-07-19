package permission

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Rule specifies when a policy entry applies to a tool call.
//
// Syntax: "Tool" or "Tool(specifier)"
//   - "Bash"           — matches all Bash calls
//   - "Bash(go test:*)" — matches Bash calls whose command starts with "go test"
//   - "Edit(docs/**)"   — matches Edit calls whose path matches the glob "docs/**"
//   - "Delete"          — matches all Delete calls
type Rule struct {
	Tool      string
	Specifier string // empty = match all calls to this tool
}

// ParseRule parses a rule string in the format "Tool" or "Tool(specifier)".
func ParseRule(s string) (Rule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rule{}, fmt.Errorf("empty rule")
	}

	open := strings.IndexByte(s, '(')
	if open == -1 {
		return Rule{Tool: s}, nil
	}

	close := strings.LastIndexByte(s, ')')
	if close == -1 || close < open {
		return Rule{}, fmt.Errorf("malformed rule %q: missing closing ')'", s)
	}

	tool := strings.TrimSpace(s[:open])
	spec := s[open+1 : close]
	if tool == "" {
		return Rule{}, fmt.Errorf("malformed rule %q: empty tool name", s)
	}
	return Rule{Tool: tool, Specifier: spec}, nil
}

// MustParseRule is like ParseRule but panics on error. For tests and constants.
func MustParseRule(s string) Rule {
	r, err := ParseRule(s)
	if err != nil {
		panic(err)
	}
	return r
}

// Matches reports whether the rule applies to the given tool call.
func (r Rule) Matches(toolName string, args json.RawMessage) bool {
	if !strings.EqualFold(r.Tool, toolName) {
		return false
	}
	if r.Specifier == "" {
		return true
	}

	val := extractMatchValue(toolName, args)
	if val == "" {
		return false
	}

	spec := r.Specifier

	// "prefix:*" convention for command matching: prefix before ":" is matched
	// with strings.HasPrefix, "*" matches the rest including path separators.
	if idx := strings.Index(spec, ":"); idx >= 0 {
		prefix := spec[:idx]
		rest := spec[idx+1:]
		if rest == "*" {
			return strings.HasPrefix(val, prefix)
		}
		// Non-wildcard after ":" → exact match
		return val == spec
	}

	// Glob pattern matching with "**" support
	if strings.Contains(spec, "**") {
		return globMatch(spec, val)
	}
	matched, _ := filepath.Match(spec, val)
	return matched
}

// extractMatchValue pulls the string most relevant for rule matching from
// the tool call arguments. For bash it's the "command" field; for file
// tools it's the first path-like field.
func extractMatchValue(toolName string, args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return ""
	}

	// Bash: use "command" field
	if strings.EqualFold(toolName, "bash") {
		if cmd, ok := m["command"].(string); ok {
			return cmd
		}
	}

	// Try known path keys in priority order
	for _, key := range []string{"path", "file_path", "source_path", "destination_path"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}

	// 已知路径键已在上文处理。不再使用"map 第一个 string 值"作为 fallback，
	// 因为 Go map 遍历顺序随机，会导致同一输入多次匹配返回不同字段，
	// 规则判定不确定。未覆盖的键返回空串（不匹配），行为可预测。
	return ""
}

// globMatch handles "**" patterns by converting to a simple recursive match.
// Supports:
//   - "**" matches any number of path segments
//   - "*" matches within a single segment
//   - "?" matches a single character
func globMatch(pattern, name string) bool {
	return globMatchSeg(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func globMatchSeg(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			// ** matches zero or more segments
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if globMatchSeg(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		matched, _ := filepath.Match(pattern[0], name[0])
		if !matched {
			return false
		}
		pattern = pattern[1:]
		name = name[1:]
	}
	return len(name) == 0
}
