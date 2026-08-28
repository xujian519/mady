// reminder.go 实现建议性防循环提醒：同一工具以相同参数连续调用达到阈值
// （默认 3/5/8 次）时，向下一轮模型请求注入一条提醒消息，建议换方法或收尾。
// 提醒是 advisory 的——永不阻塞、永不终止；硬熔断仍由致命检测器负责。

package doomloop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// reminderMarker 是提醒消息的前缀标记。注入的消息扫描时按此前缀跳过，
// 保证"新用户消息清零计数"的判定不被提醒自身干扰。
const reminderMarker = "[repeat-guard]"

// reminderFormat 是注入的提醒文案（面向模型，非终端用户）。
const reminderFormat = reminderMarker + " 工具 %s 已以相同参数连续调用 %d 次。" +
	"如果结果不理想，请调整参数或更换方法；如果信息已足够，请直接推进任务，不要重复调用。"

// reminderState 跟踪连续同参调用与待注入提醒。非并发安全，由 DoomLoop.mu 保护。
type reminderState struct {
	// lastKey 是最近一次工具调用的归一化键（连续同参判定）。
	lastKey string
	// count 是 lastKey 的连续出现次数。
	count int
	// lastRealUser 是最近一条真实用户消息内容，用于新轮次检测。
	lastRealUser string
	// pending 是待注入的提醒文案；空表示无待注入。
	pending string
}

// record 累计本轮工具调用的连击并在命中阈值时设置待注入提醒。
// 连续性按"最近一次调用的键"判定：换工具或换参数即重新计数。
func (r *reminderState) record(calls []agentcore.ToolCall, thresholds map[int]bool) {
	for _, tc := range calls {
		key := tc.Name + ":" + normalizeToolArgs(tc.Arguments)
		if key == r.lastKey {
			r.count++
		} else {
			r.lastKey = key
			r.count = 1
		}
		if thresholds[r.count] {
			r.pending = fmt.Sprintf(reminderFormat, tc.Name, r.count)
		}
	}
}

// normalizeToolArgs 把 JSON 参数归一化为键序确定的字符串（Go 的 json.Marshal
// 对 map 按 key 排序），使参数书写顺序不同的调用判定为相同。非 JSON 参数
// 原样返回。
func normalizeToolArgs(args string) string {
	var v any
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return args
	}
	norm, err := json.Marshal(v)
	if err != nil {
		return args
	}
	return string(norm)
}

// lastRealUserContent 返回消息列表中最后一条真实用户消息的内容；
// 提醒消息（带 reminderMarker 前缀）跳过。没有真实用户消息时返回空串。
func lastRealUserContent(msgs []agentcore.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == agentcore.RoleUser && !strings.HasPrefix(m.Content, reminderMarker) {
			return m.Content
		}
	}
	return ""
}
