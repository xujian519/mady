package agentcore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CompactionParams bundles the parameters needed by runCompaction.
// Provider and Model identify the LLM used for summarization;
// CompressionProvider/Model override those for a dedicated compression model.
type CompactionParams struct {
	Provider            Provider
	Model               string
	State               *AgentState
	KeepRecentTokens    int64
	Structured          bool
	ProtectFirstN       int
	FocusTopic          string
	CompState           *compactionState
	CompressionModel    string
	CompressionProvider Provider

	// ContextWindow is the main model's context limit (in tokens). When
	// non-zero, runCompaction pre-truncates the messages-to-summarize so the
	// summarization request itself doesn't overflow. This is the SOLE
	// proactive truncation defense — all context engines (tiered, compressor,
	// chunked) benefit because they all flow through runCompaction.
	//
	// Note: this is the main model's window, not the compression model's.
	// If a dedicated CompressionModel with a smaller window is configured,
	// the pre-flight uses the main model's (larger) budget, which is
	// conservative-safe (over-truncates slightly) but never under-protects.
	ContextWindow int64

	// SummaryFailureCooldown is the cooldown duration after a summary
	// generation failure. During the cooldown, compaction is skipped to
	// avoid tight retry loops. Default is 600s.
	SummaryFailureCooldown time.Duration

	// IneffectiveCooldown is the cooldown duration after two consecutive
	// low-savings compactions. Prevents thrashing when compaction is not
	// beneficial. Default is 300s.
	IneffectiveCooldown time.Duration
}

func runCompaction(ctx context.Context, p CompactionParams) (int64, error) {
	state := p.State
	keepRecentTokens := p.KeepRecentTokens
	structured := p.Structured
	protectFirstN := p.ProtectFirstN
	focusTopic := p.FocusTopic
	compState := p.CompState
	msgs := state.Messages()

	// Reset the ineffective-compaction breaker now that we are actually
	// proceeding with a compaction. shouldCompact only checks the cooldown;
	// the reset happens here to keep that predicate side-effect-free.
	resetCompactionBreaker(compState)

	_, compressStart, compressEnd, msgs, ok := prepareCompactionRange(msgs, protectFirstN, keepRecentTokens, state)
	if !ok {
		return 0, nil
	}

	// Pre-flight: if the messages-to-summarize exceed the model's context
	// window, truncate each message proportionally BEFORE building the prompt.
	// Without this, a 3M-token conversation would produce a 3M-token
	// summarization request that itself overflows the model's context window.
	turnsToSummarize, ok := selectTurnsToSummarize(msgs, compressStart, compressEnd, compState)
	if !ok {
		return 0, nil
	}
	if p.ContextWindow > 0 {
		truncateForSummaryContext(turnsToSummarize, p.ContextWindow)
	}

	displayTokens := EstimateMessagesTokens(msgs)

	sysPrompt, userBody, maxSummaryTokens := buildSummaryRequest(
		turnsToSummarize, compState, focusTopic, structured, keepRecentTokens,
	)

	summaryReq := &ProviderRequest{
		Model: compModelFor(p),
		Messages: []Message{
			{Role: RoleSystem, Content: sysPrompt},
			{Role: RoleUser, Content: userBody},
		},
		Temperature: 0,
		MaxTokens:   maxSummaryTokens,
	}

	resp, err := compProviderFor(p).Complete(ctx, summaryReq)
	if err != nil {
		return 0, handleSummaryFailure(compState, p.SummaryFailureCooldown, err)
	}

	summaryContent := resp.Content
	if compState != nil {
		compState.mu.Lock()
		compState.previousSummary = summaryContent
		compState.lastSummaryError = ""
		compState.mu.Unlock()
	}

	summaryMsg := buildSummaryMessage(structured, resp.Content, turnsToSummarize)

	systemMsg := attachCompactionNote(msgs)
	compressed := buildCompressedMessages(systemMsg, summaryMsg, msgs, int64(len(msgs))-compressEnd)
	compressed = sanitizeToolPairs(compressed)

	// NOTE: ReplaceMessages bypasses the BeforeMessagePersist/AfterMessagePersist
	// lifecycle hooks (those only fire in the normal persistMessage append
	// path). Hooks that inspect or audit message content — e.g. guardrail or
	// evidence hooks — will NOT see this compaction summary. This is an
	// accepted audit gap for now; a future BeforeCompactionPersist/
	// AfterCompactionPersist hook pair would close it (review finding M1).
	state.ReplaceMessages(compressed)

	updateCompactionStats(compState, displayTokens, EstimateMessagesTokens(compressed), p.IneffectiveCooldown)
	return compressEnd - compressStart, nil
}

// resetCompactionBreaker 在真正执行压缩前重置低效压缩断路器。
func resetCompactionBreaker(compState *compactionState) {
	if compState == nil {
		return
	}
	compState.mu.Lock()
	if compState.ineffectiveCompactions >= 2 &&
		time.Now().After(compState.ineffectiveCooldownUntil) {
		compState.ineffectiveCompactions = 0
	}
	compState.mu.Unlock()
}

// prepareCompactionRange 计算压缩边界：头部保护条数、可压缩起点与终点，
// 并先裁剪旧工具结果（prune）以减少后续摘要输入。
// 返回 (headProtect, compressStart, compressEnd, 裁剪后消息, 是否可压缩)。
func prepareCompactionRange(msgs []Message, protectFirstN int, keepRecentTokens int64, state *AgentState) (int64, int64, int64, []Message, bool) {
	nMessages := len(msgs)
	headProtect := int64(protectFirstN)
	if headProtect <= 0 {
		headProtect = 3
	}
	minForCompress := headProtect + 3 + 1
	if int64(nMessages) <= minForCompress {
		return headProtect, 0, 0, msgs, false
	}

	// Use token-based tail boundary instead of rough keepRecentTokens/100 estimate.
	tailStart := findTailCutByTokens(msgs, headProtect, keepRecentTokens)
	protectTail := len(msgs) - int(tailStart)
	msgs, prunedCount := pruneOldToolResults(msgs, protectTail)
	if prunedCount > 0 {
		state.ReplaceMessages(msgs)
		msgs = state.Messages()
	}

	compressStart := int64(0)
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		compressStart = 1
	}
	compressStart = alignBoundaryForward(msgs, compressStart)

	compressEnd := findTailCutByTokens(msgs, headProtect, keepRecentTokens)
	if compressStart >= compressEnd {
		return headProtect, compressStart, compressEnd, msgs, false
	}
	return headProtect, compressStart, compressEnd, msgs, true
}

// selectTurnsToSummarize 确定要摘要的消息区间：若已有历史摘要则从其后开始
// （迭代增量摘要），否则从 compressStart 开始。返回 (区间消息, 是否可压缩)。
func selectTurnsToSummarize(msgs []Message, compressStart, compressEnd int64, compState *compactionState) ([]Message, bool) {
	summarySearchStart := int64(0)
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		summarySearchStart = 1
	}
	summaryIdx, summaryBody := findLatestContextSummary(msgs, summarySearchStart, compressEnd)

	if summaryIdx >= 0 {
		if summaryBody != "" && compState != nil {
			compState.mu.Lock()
			if compState.previousSummary == "" {
				compState.previousSummary = summaryBody
			}
			compState.mu.Unlock()
		}
		startIdx := compressStart
		if summaryIdx+1 > startIdx {
			startIdx = summaryIdx + 1
		}
		if startIdx >= compressEnd {
			return nil, false
		}
		return msgs[startIdx:compressEnd], true
	}
	return msgs[compressStart:compressEnd], true
}

// truncateForSummaryContext 在摘要请求超出模型上下文窗口时按比例截断每条
// 消息，避免构造一个自身溢出的超大请求。
func truncateForSummaryContext(turnsToSummarize []Message, contextWindow int64) {
	maxSummaryInput := contextWindow - summaryTokensCeiling - compactionSystemPromptOverhead
	if maxSummaryInput <= 0 {
		return
	}
	summarizeTokens := EstimateMessagesTokens(turnsToSummarize)
	if summarizeTokens <= maxSummaryInput {
		return
	}
	perMsgLimit := max(maxSummaryInput/int64(len(turnsToSummarize)),
		compactionMinPerMsgTokens)
	for i := range turnsToSummarize {
		turnsToSummarize[i].Content = truncateToTokenBudget(
			turnsToSummarize[i].Content,
			EstimateMessageTokens(turnsToSummarize[i]),
			perMsgLimit,
			"...[truncated for summary]",
		)
	}
}

// buildSummaryRequest 组装摘要系统提示词与用户正文，并决定摘要输出 token 上限。
func buildSummaryRequest(turnsToSummarize []Message, compState *compactionState, focusTopic string, structured bool, keepRecentTokens int64) (string, string, int64) {
	var prevSummaryContext string
	if compState != nil {
		compState.mu.Lock()
		if compState.previousSummary != "" {
			prevSummaryContext = fmt.Sprintf("\n\nPrevious summary (iterative update):\n%s\n\n", compState.previousSummary)
		}
		compState.mu.Unlock()
	}

	var sb strings.Builder
	for _, msg := range turnsToSummarize {
		fmt.Fprintf(&sb, "[%s]: %s\n", msg.Role, MessageStringForSummary(msg))
	}

	sysPrompt := compactionSystemPrompt
	userBody := fmt.Sprintf("Summarize this conversation:%s\n\n%s", prevSummaryContext, sb.String())
	if focusTopic != "" {
		userBody = fmt.Sprintf("Focus on preserving information related to: %s\n\n%s", focusTopic, userBody)
	}

	maxSummaryTokens := int64(1024)
	if structured {
		sysPrompt = structuredCompactionSystemPrompt
		userBody = fmt.Sprintf("Summarize this conversation transcript into the required JSON object:%s\n\n%s", prevSummaryContext, sb.String())
		if focusTopic != "" {
			userBody = fmt.Sprintf("Focus on preserving information related to: %s\n\n%s", focusTopic, userBody)
		}
		targetTokens := int64(0)
		if compState != nil && keepRecentTokens > 0 {
			targetTokens = int64(float64(keepRecentTokens) * summaryRatio)
			if targetTokens < minSummaryTokens {
				targetTokens = minSummaryTokens
			}
			if targetTokens > summaryTokensCeiling {
				targetTokens = summaryTokensCeiling
			}
		}
		if targetTokens > 0 {
			maxSummaryTokens = targetTokens
		} else {
			maxSummaryTokens = 2048
		}
	}
	return sysPrompt, userBody, maxSummaryTokens
}

// compProviderFor 选择摘要使用的 Provider：优先 CompressionProvider。
func compProviderFor(p CompactionParams) Provider {
	if p.CompressionProvider != nil {
		return p.CompressionProvider
	}
	return p.Provider
}

// compModelFor 选择摘要使用的模型：优先 CompressionModel。
func compModelFor(p CompactionParams) string {
	if p.CompressionModel != "" {
		return p.CompressionModel
	}
	return p.Model
}

// handleSummaryFailure 记录摘要生成失败并设置冷却，避免紧重试循环。
// 返回包装后的错误供调用方报告。
func handleSummaryFailure(compState *compactionState, cooldown time.Duration, err error) error {
	// Summary generation failed: preserve the original messages rather
	// than replacing them with a lossy fallback. Previously this path
	// built a one-line "summary failed" placeholder and still called
	// ReplaceMessages, permanently dropping the [compressStart:compressEnd)
	// slice — unrecoverable data loss on a transient provider error.
	// Rely on summaryFailureCooldown to suppress tight retry loops.
	if compState != nil {
		compState.mu.Lock()
		compState.previousSummary = ""
		compState.lastSummaryError = err.Error()
		if cooldown <= 0 {
			cooldown = summaryFailureCooldownSeconds * time.Second
		}
		compState.summaryFailureCooldown = time.Now().Add(cooldown)
		compState.mu.Unlock()
	}
	return fmt.Errorf("compaction summary generation failed: %w", err)
}

// buildSummaryMessage 将摘要响应包装为 CompactionSummary 消息；结构化模式
// 解析失败时保留原始内容并附加错误说明（避免静默丢弃消息）。
func buildSummaryMessage(structured bool, content string, turnsToSummarize []Message) Message {
	if !structured {
		// Wrap with compactionSummaryPrefix so that findLatestContextSummary
		// can locate this summary on subsequent compactions, enabling
		// iterative (delta) summarisation rather than full re-summarisation.
		return Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("%s\n%s%s", compactionSummaryPrefix, content, compactionSummaryEndMarker),
			Type:    MessageTypeCompactionSummary,
		}
	}

	sum, perr := parseStructuredCompactionSummary(content)
	if perr != nil {
		nDropped := int64(len(turnsToSummarize))
		summaryContent := fmt.Sprintf("%s\n"+"Summary parsing failed: %v. %d message(s) were dropped."+"%s", compactionSummaryPrefix, perr, nDropped, compactionSummaryEndMarker)
		return Message{
			Role:    RoleSystem,
			Content: summaryContent,
			Type:    MessageTypeCompactionSummary,
		}
	}
	readable := sum.ToReadableSummary()
	meta := sum.MarshalJSONMetadata()
	return Message{
		Role:     RoleSystem,
		Content:  fmt.Sprintf("%s\n%s%s", compactionSummaryPrefix, readable, compactionSummaryEndMarker),
		Type:     MessageTypeCompactionSummary,
		Metadata: meta,
	}
}

// attachCompactionNote 在系统消息尾部追加压缩提示（若尚未存在），返回
// 深拷贝后的系统消息指针；无系统消息时返回 nil。
func attachCompactionNote(msgs []Message) *Message {
	if len(msgs) == 0 || msgs[0].Role != RoleSystem {
		return nil
	}
	sysCopy := msgs[0]
	compactionNote := "[Note: Some earlier conversation turns have been compacted into a handoff summary to preserve context space. The current session state may still reflect earlier work, so build on that summary and state rather than re-doing work.]"
	if !strings.Contains(sysCopy.Content, compactionNote) {
		sysCopy.Content = sysCopy.Content + "\n\n" + compactionNote
	}
	return &sysCopy
}

// buildCompressedMessages 组装压缩后的会话：系统消息 + 摘要 + 确认消息 +
// 保留的尾部消息。
func buildCompressedMessages(systemMsg *Message, summaryMsg Message, msgs []Message, tailStart int64) []Message {
	compressed := make([]Message, 0, int64(len(msgs))-tailStart+3)
	if systemMsg != nil {
		compressed = append(compressed, *systemMsg)
	}
	compressed = append(compressed, summaryMsg, Message{
		Role:    RoleAssistant,
		Content: "Understood, I have the context from the previous conversation. How can I help?",
		Type:    MessageTypeCompactionSummary,
	})
	compressed = append(compressed, msgs[tailStart:]...)
	return compressed
}

// updateCompactionStats 记录压缩节省比例，并维护低效压缩断路器状态。
func updateCompactionStats(compState *compactionState, displayTokens, newEstimate int64, cooldown time.Duration) {
	if compState == nil {
		return
	}
	savingsPct := float64(0)
	savedEstimate := displayTokens - newEstimate
	if displayTokens > 0 {
		savingsPct = float64(savedEstimate) / float64(displayTokens) * 100
	}
	compState.mu.Lock()
	compState.lastSavingsPct = savingsPct
	if savingsPct < 10 {
		compState.ineffectiveCompactions++
		// When the breaker trips, set a cooldown so it can recover later.
		if compState.ineffectiveCompactions >= 2 {
			if cooldown <= 0 {
				cooldown = ineffectiveCompactionCooldownSeconds * time.Second
			}
			compState.ineffectiveCooldownUntil = time.Now().Add(cooldown)
		}
	} else {
		compState.ineffectiveCompactions = 0
	}
	compState.mu.Unlock()
}
