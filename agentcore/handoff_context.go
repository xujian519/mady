package agentcore

import (
	"regexp"
	"strings"
	"time"
)

// HandoffContext 是交接时传递给目标 Agent 的结构化上下文，
// 替代整段转发对话历史的方式，减少 token 消耗并提升交接质量。
type HandoffContext struct {
	FromAgent         string            `json:"from_agent"`         // 来源 Agent 名称
	ToAgent           string            `json:"to_agent"`           // 目标 Agent 名称
	UserIntent        string            `json:"user_intent"`        // 用户意图摘要
	ExtractedEntities map[string]string `json:"extracted_entities"` // 抽取的结构化实体
	RecentMessages    []Message         `json:"recent_messages"`    // 最近 N 条消息
	Timestamp         time.Time         `json:"timestamp"`          // 交接时间戳
}

// 实体抽取正则模式 —— 覆盖专利号、申请号、案件编号等格式固定的实体。
// 格式固定的实体用正则比 LLM 抽取更准确且零成本。
var (
	patentNumPattern = regexp.MustCompile(`CN\d{9}[A-Z]`)         // CN109690000A
	appNumPattern    = regexp.MustCompile(`\d{13}`)               // 13位申请号
	pctAppNumPattern = regexp.MustCompile(`PCT/CN\d{4}/\d{6}`)    // PCT/CN2024/123456
	caseIDPattern    = regexp.MustCompile(`[A-Z]{2}\d{4}-\d{4,}`) // 案件编号
)

// ExtractHandoffContext 从 Agent 当前状态中抽取交接上下文。
//
// recentN 控制携带的最近消息条数，0 表示使用默认值 6。
// UserIntent 当前使用最后一条用户消息作为摘要（v1 规则占位），
// 后续可按需升级为 LLM 摘要（接口无需变更）。
func (a *Agent) ExtractHandoffContext(toAgent string, recentN int) HandoffContext {
	msgs := a.state.Messages()
	if recentN <= 0 {
		recentN = 6
	}

	fullText := joinMessageText(msgs)

	return HandoffContext{
		FromAgent:         a.config.Name,
		ToAgent:           toAgent,
		UserIntent:        lastUserMessage(msgs),
		ExtractedEntities: extractEntities(fullText),
		RecentMessages:    lastN(msgs, recentN),
		Timestamp:         time.Now(),
	}
}

// extractEntities 使用正则从消息文本中抽取结构化实体。
func extractEntities(text string) map[string]string {
	entities := map[string]string{}

	if m := patentNumPattern.FindString(text); m != "" {
		entities["patent_no"] = m
	}
	if m := appNumPattern.FindString(text); m != "" {
		entities["app_no"] = m
	}
	if m := pctAppNumPattern.FindString(text); m != "" {
		entities["pct_app_no"] = m
	}
	if m := caseIDPattern.FindString(text); m != "" {
		entities["case_id"] = m
	}

	if len(entities) == 0 {
		return nil
	}
	return entities
}

// lastUserMessage 返回最后一条 RoleUser 消息的内容。
func lastUserMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

// lastN 返回最近 n 条消息。
func lastN(msgs []Message, n int) []Message {
	if n <= 0 || len(msgs) == 0 {
		return nil
	}
	start := max(len(msgs)-n, 0)
	out := make([]Message, len(msgs)-start)
	copy(out, msgs[start:])
	return out
}

// joinMessageText 拼接所有消息的文本内容用于实体抽取。
func joinMessageText(msgs []Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Content)
		b.WriteString(" ")
	}
	return b.String()
}
