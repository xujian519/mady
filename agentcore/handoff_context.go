package agentcore

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
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
// 当 Agent 配置了 Provider 时，UserIntent 使用 LLM 摘要（v2）；
// 若 Provider 不可用或 LLM 调用失败，自动回退到最后一条用户消息（v1）。
func (a *Agent) ExtractHandoffContext(toAgent string, recentN int) HandoffContext {
	msgs := a.state.Messages()
	if recentN <= 0 {
		recentN = 6
	}

	fullText := joinMessageText(msgs)

	// 传递 fullText 避免 summarizeUserIntent 重复拼接消息
	intent := a.summarizeUserIntent(fullText, msgs)

	return HandoffContext{
		FromAgent:         a.config.Name,
		ToAgent:           toAgent,
		UserIntent:        intent,
		ExtractedEntities: extractEntities(fullText),
		RecentMessages:    lastN(msgs, recentN),
		Timestamp:         time.Now(),
	}
}

// --- UserIntent 摘要 (v2: LLM 驱动) ---

const (
	userIntentSystemPrompt = "用一句话概括用户的核心意图，不超过20个字。直接输出摘要，不要前缀。"
	intentCacheTTL         = 5 * time.Minute
	intentCacheMaxRunes    = 500  // 缓存键截断长度（符文安全）
	intentInputMaxRunes    = 2000 // LLM 输入截断长度（符文安全）
)

type intentCacheEntry struct {
	intent    string
	expiresAt time.Time
}

// intentCacheEntry holds a cached user intent summary with expiry.
// The cache itself is per-Agent (see Agent.intentCache field) to avoid
// cross-agent cache pollution in multi-agent setups.

// summarizeUserIntent 使用 Agent 的 Provider 生成用户意图摘要。
// 先查缓存，缓存未命中时调用 LLM，再写缓存。
// provider 不可用或 LLM 失败时回退到 lastUserMessage。
// fullText 是已拼接的消息文本，复用避免重复 joinMessageText。
func (a *Agent) summarizeUserIntent(fullText string, msgs []Message) string {
	// v1 fallback: 最后一条用户消息
	lastMsg := LastUserMessage(msgs)
	v1Fallback := func() string { return lastMsg }

	// Provider 不可用时直接回退
	if a == nil || a.config.Provider == nil {
		return v1Fallback()
	}

	// 用拼接后的全文做缓存键（符文安全截断以避免缓存膨胀）
	cacheKey := truncateString(fullText, intentCacheMaxRunes)

	// Lazy-init per-agent cache
	if a.intentCache == nil {
		a.intentCache = make(map[string]intentCacheEntry)
	}

	a.intentCacheMu.Lock()
	now := time.Now()
	for k, v := range a.intentCache {
		if now.After(v.expiresAt) {
			delete(a.intentCache, k)
		}
	}
	if entry, ok := a.intentCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		a.intentCacheMu.Unlock()
		return entry.intent
	}
	a.intentCacheMu.Unlock()

	// 调用 LLM（使用 context.Background() 加超时，LLM 摘要不绑定请求生命周期，
	// 因为即使在请求取消后，填充缓存的摘要值仍对后续请求有用）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := a.config.Provider.Complete(ctx, &ProviderRequest{
		Model: a.config.Model,
		Messages: []Message{
			{Role: RoleSystem, Content: userIntentSystemPrompt},
			{Role: RoleUser, Content: truncateString(fullText, intentInputMaxRunes)},
		},
		MaxTokens: 50,
	})

	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return v1Fallback()
	}

	intent := strings.TrimSpace(resp.Content)

	// 写缓存
	a.intentCacheMu.Lock()
	a.intentCache[cacheKey] = intentCacheEntry{
		intent:    intent,
		expiresAt: time.Now().Add(intentCacheTTL),
	}
	a.intentCacheMu.Unlock()

	return intent
}

// truncateString 符文安全地截断字符串到指定长度（超出部分替换为 "…"）。
func truncateString(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	// 逐符文截断，避免破坏多字节 UTF-8 序列
	runes := []rune(s)
	return string(runes[:maxLen]) + "…"
}

// --- 实体抽取 ---

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

// --- 消息工具函数 ---

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
