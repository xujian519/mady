package agentcore

import (
	"encoding/json"
	"fmt"
	"strings"
)

const structuredCompactionSystemPrompt = `你是一个对话摘要助手。用中文创建以下对话的结构化 JSON 摘要，保留所有继续对话所需的关键上下文。

只输出一个 JSON 对象（不要 markdown 代码块，不要注释），包含以下字段（JSON key 必须使用英文，value 使用中文）：

active_task（当前任务） — 逐字复制用户最近一次请求或问题。
goal（目标） — 用户总体想要完成什么。
constraints_preferences（约束与偏好） — 用户偏好、编码风格、约束条件、重要决策。
completed_actions（已完成操作） — 编号列表，包含工具名称、操作目标和结果。格式：N. 操作 目标 — 结果。
active_state（当前状态） — 工作目录、分支、已修改文件、测试状态、运行中的进程。
in_progress（进行中） — 当前正在进行的工作。
blocked（阻塞） — 尚未解决的阻塞、错误或问题。请包含准确的错误信息。
key_decisions（关键决策） — 重要技术决策及其原因。
resolved_questions（已解决问题） — 已回答的问题——包含答案。
pending_user_asks（待用户回应） — 尚未回答或完成的用户问题或请求。
relevant_files（相关文件） — 已读取、修改或创建的文件——各附简要说明。
remaining_work（剩余工作） — 尚待完成的内容——以背景方式表述，而非指令。
critical_context（关键上下文） — 具体值、错误信息、配置细节。绝不包含密码等敏感信息。

每个字段需简洁但保留事实、决策、工具结果、错误和待办事项。
仅当字段确实无内容时才使用空字符串 ""。
要具体——包含文件路径、命令输出、错误信息、行号和具体数值。`

func extractJSONObject(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		// strip optional ```json ... ``` wrapper
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(s)
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
	}
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return s
	}
	return s[i : j+1]
}

func parseStructuredCompactionSummary(raw string) (StructuredCompactionSummary, error) {
	frag := extractJSONObject(raw)
	var out StructuredCompactionSummary
	if err := json.Unmarshal([]byte(frag), &out); err != nil {
		return out, fmt.Errorf("parse structured compaction: %w", err)
	}
	return out, nil
}
