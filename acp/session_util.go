package acp

import (
	"encoding/json"
	"fmt"
)

func parseToolArgs(raw string) map[string]any {
	args := make(map[string]any)
	if raw == "" {
		return args
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		args["raw"] = raw
	}
	return args
}

// ToolKind maps a tool name to a category label for ACP display.
func ToolKind(name string) string {
	kinds := map[string]string{
		"read_file":          "read",
		"write_file":         "edit",
		"patch":              "edit",
		"search_files":       "search",
		"terminal":           "execute",
		"process":            "execute",
		"execute_code":       "execute",
		"todo":               "other",
		"skill_view":         "read",
		"skills_list":        "read",
		"skill_manage":       "edit",
		"web_search":         "fetch",
		"web_extract":        "fetch",
		"browser_navigate":   "fetch",
		"browser_click":      "execute",
		"browser_type":       "execute",
		"browser_snapshot":   "read",
		"browser_vision":     "read",
		"browser_scroll":     "execute",
		"browser_press":      "execute",
		"browser_back":       "execute",
		"browser_get_images": "read",
		"delegate_task":      "execute",
		"vision_analyze":     "read",
		"image_generate":     "execute",
		"_thinking":          "think",
	}
	if k, ok := kinds[name]; ok {
		return k
	}
	return "other"
}

// BuildToolTitle builds a human-readable title for a tool call.
//
//nolint:gocognit // 原因：工具标题生成，多工具类型 switch 路由
func BuildToolTitle(name string, args map[string]any) string {
	switch name {
	case "terminal":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 80 {
				cmd = cmd[:77] + "..."
			}
			return "terminal: " + cmd
		}
	case "read_file":
		return "read: " + fmt.Sprint(args["path"])
	case "write_file":
		return "write: " + fmt.Sprint(args["path"])
	case "patch":
		mode := fmt.Sprint(args["mode"])
		path := fmt.Sprint(args["path"])
		return "patch (" + mode + "): " + path
	case "search_files":
		return "search: " + fmt.Sprint(args["pattern"])
	case "web_search":
		return "web search: " + fmt.Sprint(args["query"])
	case "web_extract":
		if urls, ok := args["urls"].([]any); ok && len(urls) > 0 {
			label := fmt.Sprint(urls[0])
			if len(urls) > 1 {
				label += fmt.Sprintf(" (+%d)", len(urls)-1)
			}
			return "extract: " + label
		}
		return "web extract"
	case "delegate_task":
		if goal, ok := args["goal"].(string); ok && goal != "" {
			if len(goal) > 60 {
				goal = goal[:57] + "..."
			}
			return "delegate: " + goal
		}
		return "delegate task"
	case "execute_code":
		if code, ok := args["code"].(string); ok {
			lines := splitLines(code)
			for _, line := range lines {
				line = trimSpace(line)
				if line != "" {
					if len(line) > 70 {
						line = line[:67] + "..."
					}
					return "python: " + line
				}
			}
		}
		return "python code"
	case "browser_navigate":
		return "navigate: " + fmt.Sprint(args["url"])
	case "browser_snapshot":
		return "browser snapshot"
	case "browser_vision":
		return "browser vision: " + truncateStr(fmt.Sprint(args["question"]), 50)
	case "browser_get_images":
		return "browser images"
	case "vision_analyze":
		return "analyze image: " + truncateStr(fmt.Sprint(args["question"]), 50)
	case "image_generate":
		prompt := ""
		if p, ok := args["prompt"].(string); ok {
			prompt = p
		} else if d, ok := args["description"].(string); ok {
			prompt = d
		}
		if prompt != "" {
			return "generate image: " + truncateStr(prompt, 50)
		}
		return "generate image"
	}
	return name
}

func splitLines(s string) []string {
	result := []string{}
	current := ""
	for _, c := range s {
		if c == '\n' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
