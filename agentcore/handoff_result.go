package agentcore

import (
	"encoding/json"
	"strings"
)

// HandoffResult 是子 Agent 处理完任务后返回给调用方 Agent 的结构化结果。
//
// Chat Agent 拿到 HandoffResult 后决定如何措辞回复用户，
// "任务怎么做的"和"怎么跟用户说"由此解耦，
// 助理的风格变化不会影响 Chat 的人设一致性。
type HandoffResult struct {
	Action        string `json:"action"`         // 做了什么，如 "检索专利信息"
	Result        string `json:"result"`         // 结果摘要，供 Chat 组织自然语言回复
	Success       bool   `json:"success"`        // 是否成功
	NeedsFollowup bool   `json:"needs_followup"` // 是否需要用户进一步确认/补充
	RawOutput     string `json:"raw_output"`     // 原始输出（调试/Tracer 记录，不直接展示）
}

// NewHandoffResult 创建一个成功的 HandoffResult。
func NewHandoffResult(action, result string) HandoffResult {
	return HandoffResult{
		Action:  action,
		Result:  result,
		Success: true,
	}
}

// NewFailureResult 创建一个失败的 HandoffResult。
func NewFailureResult(action, fallbackMsg string) HandoffResult {
	return HandoffResult{
		Action:        action,
		Result:        fallbackMsg,
		Success:       false,
		NeedsFollowup: true,
	}
}

// ParseHandoffResult 尝试从 Agent 的输出中解析 HandoffResult。
//
// 支持两种格式：
// 1. 纯 JSON: {"action":"...","result":"...",...}
// 2. Markdown 代码块包裹: ```json\n{...}\n```
//
// 如果无法解析为 HandoffResult，返回 false，
// 调用方应退回到将整个输出作为纯文本处理。
func ParseHandoffResult(output string) (HandoffResult, bool) {
	trimmed := strings.TrimSpace(output)

	// 尝试从 Markdown 代码块中提取 JSON
	if strings.HasPrefix(trimmed, "```") {
		if idx := strings.Index(trimmed, "\n"); idx > 0 {
			header := strings.TrimPrefix(trimmed[:idx], "```")
			header = strings.TrimSpace(header)
			if header == "json" || header == "handoff-result" || header == "" {
				// 找到代码块结束标记
				rest := trimmed[idx+1:]
				if endIdx := strings.LastIndex(rest, "```"); endIdx > 0 {
					trimmed = strings.TrimSpace(rest[:endIdx])
				}
			}
		}
	}

	// 仅当以 { 开头时才尝试解析为 HandoffResult
	if !strings.HasPrefix(trimmed, "{") {
		return HandoffResult{}, false
	}

	var hr HandoffResult
	if err := json.Unmarshal([]byte(trimmed), &hr); err != nil {
		return HandoffResult{}, false
	}

	// 必须有 Action 或 Result 才算有效的 HandoffResult
	if hr.Action == "" && hr.Result == "" {
		return HandoffResult{}, false
	}

	return hr, true
}

// ToHandoffResultJSON 返回 HandoffResult 的 JSON 字符串表示。
// 序列化失败时返回 "{}" 而非空字符串，避免下游收到空响应。
func (hr HandoffResult) ToHandoffResultJSON() string {
	b, err := json.Marshal(hr)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// IsFailure 返回是否为失败结果。
func (hr HandoffResult) IsFailure() bool {
	return !hr.Success
}
