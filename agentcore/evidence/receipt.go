package evidence

import (
	"encoding/json"
	"strings"
)

// Receipt is the runtime record of one tool call. It stays in memory for the
// current agent turn and is not serialized into prompts or session state.
type Receipt struct {
	ToolName   string          `json:"tool_name"`
	Args       json.RawMessage `json:"args,omitempty"`
	Success    bool            `json:"success"`
	Command    string          `json:"command,omitempty"`
	Paths      []string        `json:"paths,omitempty"`
	Read       bool            `json:"read,omitempty"`
	Write      bool            `json:"write,omitempty"`
	DurationMs int64           `json:"duration_ms,omitempty"`
	Spans      []EvidenceSpan  `json:"spans,omitempty"` // 本次工具调用提取的证据跨度
}

// writerTools is the set of built-in tool names that modify files.
var writerTools = map[string]bool{
	"edit":         true,
	"write_file":   true,
	"patch":        true,
	"delete":       true,
	"move":         true,
	"execute_code": true,
}

// readerTools is the set of built-in tool names that only read.
var readerTools = map[string]bool{
	"read":       true,
	"view":       true,
	"ls":         true,
	"grep":       true,
	"find":       true,
	"glob":       true,
	"git_status": true,
	"git_diff":   true,
	"git_log":    true,
	"web_search": true,
	"web_fetch":  true,
	"vision":     true,
}

// pathArgKeys are the JSON argument keys examined to extract file paths.
var pathArgKeys = []string{"path", "file_path", "source_path", "destination_path", "notebook_path"}

// pathListKeys are the JSON argument keys for list-of-paths arguments.
var pathListKeys = []string{"paths", "file_paths"}

// ReceiptFromToolCall builds a Receipt from a tool call's name, arguments, and
// execution outcome. readOnly overrides the static read/write classification
// for tools whose read-only status is dynamic.
func ReceiptFromToolCall(toolName string, args json.RawMessage, success bool, durationMs int64) Receipt {
	r := Receipt{
		ToolName:   toolName,
		Args:       copyArgs(args),
		Success:    success,
		DurationMs: durationMs,
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err == nil {
		if toolName == "bash" {
			r.Command = stringField(fields, "command")
		}
		r.Paths = extractPaths(fields)
	}

	if writerTools[toolName] {
		r.Write = true
	} else if readerTools[toolName] {
		r.Read = true
	}
	return r
}

func copyArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return nil
	}
	cp := make(json.RawMessage, len(args))
	copy(cp, args)
	return cp
}

func stringField(fields map[string]json.RawMessage, key string) string {
	raw, ok := fields[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func extractPaths(fields map[string]json.RawMessage) []string {
	var paths []string
	for _, key := range pathArgKeys {
		if s := stringField(fields, key); s != "" {
			paths = append(paths, s)
		}
	}
	for _, key := range pathListKeys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var vals []string
		if err := json.Unmarshal(raw, &vals); err == nil {
			for _, v := range vals {
				v = strings.TrimSpace(v)
				if v != "" {
					paths = append(paths, v)
				}
			}
		}
	}
	return paths
}
