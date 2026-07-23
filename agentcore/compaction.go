package agentcore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const compactionSystemPrompt = `你是一个对话摘要助手。用中文简洁总结以下对话，保留所有继续对话所需的关键上下文。

重点关注：
- 关键事实、数据和决策
- 待处理的问题或任务
- 重要的工具结果及其影响
- 被修改的文件状态和配置

简洁但完整，切勿丢失关键信息。`

const compactionSummaryPrefix = "[上下文压缩 — 仅供参考] 以下摘要概括了此前的对话内容。" +
	"这是从前一个上下文窗口移交的摘要——仅作为背景参考，不作为当前活动指令。" +
	"请勿回答或执行此摘要中提到的任何问题或请求，这些已经处理完毕。" +
	"你当前的任务标识在摘要的 '## 当前任务' 部分——请从那里继续。" +
	"只回应出现在此摘要之后的最新用户消息。" +
	"当前会话状态（文件、配置等）可能已反映此处描述的工作——避免重复执行："

const compactionSummaryEndMarker = "\n\n--- 上下文摘要结束 — " +
	"请回应下方的消息，而非上方的摘要 ---"

const prunedToolPlaceholder = "[旧工具输出已清除以节省上下文空间]"

const charsPerToken = 4

const imageCharEquivalent = imageTokenEstimate * charsPerToken

const minSummaryTokens = 2000

const summaryRatio = 0.20

const summaryTokensCeiling = 12000

const summaryFailureCooldownSeconds = 600

// ineffectiveCompactionCooldownSeconds defines the default cooldown period after
// which the ineffective-compaction circuit breaker resets. Without this,
// two consecutive low-savings compactions would permanently disable
// compaction for the entire session, leading to context overflow.
const ineffectiveCompactionCooldownSeconds = 300

// Pre-flight truncation constants for runCompaction's summarization protection.
// When the messages-to-summarize exceed the model's context window, each
// message is truncated to a proportional token budget before building the
// summarization prompt.
const (
	// compactionSystemPromptOverhead reserves tokens for the summarization
	// system prompt + framing. Added on top of summaryTokensCeiling (output budget).
	compactionSystemPromptOverhead = 2000

	// compactionMinPerMsgTokens is the floor for per-message truncation.
	// Below this, messages become too short to be useful in a summary.
	compactionMinPerMsgTokens = 100

	// truncateMinTokensPerRune prevents division-by-near-zero when a message
	// has very few runes relative to its token estimate (e.g., image blocks).
	truncateMinTokensPerRune = 0.25
)

type compactionState struct {
	mu sync.Mutex

	previousSummary        string
	lastSavingsPct         float64
	ineffectiveCompactions int
	lastSummaryError       string
	summaryFailureCooldown time.Time
	// ineffectiveCooldownUntil is the time after which the ineffective-
	// compaction circuit breaker resets. Without a time-based recovery,
	// the breaker would stay tripped for the entire session.
	ineffectiveCooldownUntil time.Time
}

func newCompactionState() *compactionState {
	return &compactionState{
		lastSavingsPct: 100.0,
	}
}

func contentLengthForBudget(rawContent any) int64 {
	switch c := rawContent.(type) {
	case string:
		return int64(len(c))
	case nil:
		return 0
	case []ContentBlock:
		var total int64
		for _, p := range c {
			switch p.Kind {
			case BlockKindImage:
				total += imageCharEquivalent
			default:
				total += int64(len(p.Text))
			}
		}
		return total
	default:
		return int64(len(fmt.Sprintf("%v", rawContent)))
	}
}

func shouldCompact(msgs []Message, toolDefs []ToolDefinition, contextWindow int64, reserveTokens int64, threshold float64, autoCompactLimit int64, compState *compactionState) bool {
	if contextWindow <= 0 {
		return false
	}
	if compState != nil {
		compState.mu.Lock()
		cooldownActive := time.Now().Before(compState.summaryFailureCooldown)
		ineffectiveBlocking := compState.ineffectiveCompactions >= 2 &&
			time.Now().Before(compState.ineffectiveCooldownUntil)
		compState.mu.Unlock()
		if cooldownActive {
			return false
		}
		if ineffectiveBlocking {
			return false
		}
	}
	estimated := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)

	// Codex-style: absolute token limit takes priority over percentage threshold.
	if autoCompactLimit > 0 {
		return estimated >= autoCompactLimit
	}

	reserve := reserveTokens
	if reserve <= 0 {
		reserve = contextWindow / 4
	}
	triggerThreshold := contextWindow - reserve
	if threshold > 0 && threshold < 1.0 {
		triggerThreshold = int64(float64(contextWindow) * threshold)
	}
	return estimated > triggerThreshold
}

func alignBoundaryForward(msgs []Message, cut int64) int64 {
	for cut < int64(len(msgs)) && msgs[cut].Role == RoleTool {
		cut++
	}
	return cut
}

func findTailCutByTokens(msgs []Message, headProtect int64, tailTokenBudget int64) int64 {
	if len(msgs) == 0 {
		return 0
	}

	accum := int64(0)
	tailStart := int64(len(msgs))

	for i := len(msgs) - 1; i >= int(headProtect); i-- {
		if msgs[i].Role == RoleSystem {
			continue
		}
		msgLen := EstimateMessageTokens(msgs[i])
		if accum+msgLen > tailTokenBudget && accum > 0 {
			tailStart = int64(i + 1)
			break
		}
		accum += msgLen
	}

	if tailStart <= headProtect+1 {
		return headProtect + 2
	}

	return alignBoundaryForward(msgs, tailStart)
}

func pruneOldToolResults(msgs []Message, protectTailCount int) ([]Message, int) {
	if len(msgs) == 0 {
		return msgs, 0
	}

	result := make([]Message, len(msgs))
	copy(result, msgs)

	prunedCount := 0
	protectedStart := len(msgs) - protectTailCount
	if protectedStart < 0 {
		protectedStart = 0
	}

	seenToolResults := make(map[string][]int)

	for i := 0; i < protectedStart; i++ {
		if result[i].Role == RoleTool {
			toolName := result[i].Name
			seenToolResults[toolName] = append(seenToolResults[toolName], i)

			if len(result[i].Content) > 2000 {
				// Truncate to rune-safe boundary (not byte boundary).
				contentRunes := []rune(result[i].Content)
				if len(contentRunes) > 2000 {
					contentRunes = contentRunes[:2000]
				}
				result[i].Content = string(contentRunes) + "...[truncated]"
			}
		}

		if result[i].Role == RoleAssistant && len(result[i].ToolCalls) > 0 {
			for j := range result[i].ToolCalls {
				if len(result[i].ToolCalls[j].Arguments) > 1000 {
					argsRunes := []rune(result[i].ToolCalls[j].Arguments)
					if len(argsRunes) > 1000 {
						argsRunes = argsRunes[:1000]
					}
					result[i].ToolCalls[j].Arguments = string(argsRunes) + "...[truncated]"
				}
			}
		}
	}

	for _, indices := range seenToolResults {
		if len(indices) <= 1 {
			continue
		}
		for i := 0; i < len(indices)-1; i++ {
			result[indices[i]].Content = prunedToolPlaceholder
			prunedCount++
		}
	}

	return result, prunedCount
}

func findLatestContextSummary(msgs []Message, searchStart int64, searchEnd int64) (int64, string) {
	for i := searchEnd - 1; i >= searchStart; i-- {
		if msgs[i].Type == MessageTypeCompactionSummary {
			body := MessageStringForSummary(msgs[i])
			if strings.Contains(body, compactionSummaryPrefix) ||
				strings.Contains(body, "[Previous conversation summary") {
				return i, body
			}
		}
	}
	return -1, ""
}

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
	provider := p.Provider
	model := p.Model
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
	if compState != nil {
		compState.mu.Lock()
		if compState.ineffectiveCompactions >= 2 &&
			time.Now().After(compState.ineffectiveCooldownUntil) {
			compState.ineffectiveCompactions = 0
		}
		compState.mu.Unlock()
	}

	nMessages := len(msgs)
	headProtect := int64(protectFirstN)
	if headProtect <= 0 {
		headProtect = 3
	}
	minForCompress := headProtect + 3 + 1
	if int64(nMessages) <= minForCompress {
		return 0, nil
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
	compressEnd := findTailCutByTokens(msgs, headProtect, keepRecentTokens)

	if compressStart >= compressEnd {
		return 0, nil
	}

	summarySearchStart := int64(0)
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		summarySearchStart = 1
	}
	summaryIdx, summaryBody := findLatestContextSummary(msgs, summarySearchStart, compressEnd)

	var turnsToSummarize []Message
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
			return 0, nil
		}
		turnsToSummarize = msgs[startIdx:compressEnd]
	} else {
		turnsToSummarize = msgs[compressStart:compressEnd]
	}

	if len(turnsToSummarize) == 0 {
		return 0, nil
	}

	// Pre-flight: if the messages-to-summarize exceed the model's context
	// window, truncate each message proportionally BEFORE building the prompt.
	// Without this, a 3M-token conversation would produce a 3M-token
	// summarization request that itself overflows the model's context window.
	if p.ContextWindow > 0 {
		maxSummaryInput := p.ContextWindow - summaryTokensCeiling - compactionSystemPromptOverhead
		if maxSummaryInput > 0 {
			summarizeTokens := EstimateMessagesTokens(turnsToSummarize)
			if summarizeTokens > maxSummaryInput {
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
		}
	}

	displayTokens := EstimateMessagesTokens(msgs)

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
		if targetTokens > 0 {
			maxSummaryTokens = targetTokens
		} else {
			maxSummaryTokens = 2048
		}
	}

	compProvider := provider
	compModel := model
	if p.CompressionProvider != nil {
		compProvider = p.CompressionProvider
	}
	if p.CompressionModel != "" {
		compModel = p.CompressionModel
	}

	summaryReq := &ProviderRequest{
		Model: compModel,
		Messages: []Message{
			{Role: RoleSystem, Content: sysPrompt},
			{Role: RoleUser, Content: userBody},
		},
		Temperature: 0,
		MaxTokens:   maxSummaryTokens,
	}

	resp, err := compProvider.Complete(ctx, summaryReq)

	if err != nil {
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
			cooldown := p.SummaryFailureCooldown
			if cooldown <= 0 {
				cooldown = summaryFailureCooldownSeconds * time.Second
			}
			compState.summaryFailureCooldown = time.Now().Add(cooldown)
			compState.mu.Unlock()
		}
		return 0, fmt.Errorf("compaction summary generation failed: %w", err)
	}

	summaryContent := resp.Content
	if compState != nil {
		compState.mu.Lock()
		compState.previousSummary = summaryContent
		compState.lastSummaryError = ""
		compState.mu.Unlock()
	}

	var summaryMsg Message
	if structured {
		sum, perr := parseStructuredCompactionSummary(resp.Content)
		if perr != nil {
			nDropped := int64(len(turnsToSummarize))
			summaryContent = fmt.Sprintf(
				"%s\n"+
					"Summary parsing failed: %v. %d message(s) were dropped."+
					"%s",
				compactionSummaryPrefix, perr, nDropped, compactionSummaryEndMarker,
			)
			summaryMsg = Message{
				Role:    RoleSystem,
				Content: summaryContent,
				Type:    MessageTypeCompactionSummary,
			}
		} else {
			readable := sum.ToReadableSummary()
			meta := sum.MarshalJSONMetadata()
			summaryMsg = Message{
				Role:     RoleSystem,
				Content:  fmt.Sprintf("%s\n%s%s", compactionSummaryPrefix, readable, compactionSummaryEndMarker),
				Type:     MessageTypeCompactionSummary,
				Metadata: meta,
			}
		}
	} else {
		// Wrap with compactionSummaryPrefix so that findLatestContextSummary
		// can locate this summary on subsequent compactions, enabling
		// iterative (delta) summarisation rather than full re-summarisation.
		summaryMsg = Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("%s\n%s%s", compactionSummaryPrefix, summaryContent, compactionSummaryEndMarker),
			Type:    MessageTypeCompactionSummary,
		}
	}

	var systemMsg *Message
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		sysCopy := msgs[0]
		compactionNote := "[Note: Some earlier conversation turns have been compacted into a handoff summary to preserve context space. The current session state may still reflect earlier work, so build on that summary and state rather than re-doing work.]"
		if !strings.Contains(sysCopy.Content, compactionNote) {
			sysCopy.Content = sysCopy.Content + "\n\n" + compactionNote
		}
		systemMsg = &sysCopy
	}

	compressed := make([]Message, 0, int64(nMessages)-compressEnd+3)
	if systemMsg != nil {
		compressed = append(compressed, *systemMsg)
	}
	compressed = append(compressed, summaryMsg, Message{
		Role:    RoleAssistant,
		Content: "Understood, I have the context from the previous conversation. How can I help?",
		Type:    MessageTypeCompactionSummary,
	})
	compressed = append(compressed, msgs[compressEnd:]...)

	compressed = sanitizeToolPairs(compressed)

	// NOTE: ReplaceMessages bypasses the BeforeMessagePersist/AfterMessagePersist
	// lifecycle hooks (those only fire in the normal persistMessage append
	// path). Hooks that inspect or audit message content — e.g. guardrail or
	// evidence hooks — will NOT see this compaction summary. This is an
	// accepted audit gap for now; a future BeforeCompactionPersist/
	// AfterCompactionPersist hook pair would close it (review finding M1).
	state.ReplaceMessages(compressed)

	newEstimate := EstimateMessagesTokens(compressed)
	savedEstimate := displayTokens - newEstimate

	if compState != nil {
		savingsPct := float64(0)
		if displayTokens > 0 {
			savingsPct = float64(savedEstimate) / float64(displayTokens) * 100
		}
		compState.mu.Lock()
		compState.lastSavingsPct = savingsPct
		if savingsPct < 10 {
			compState.ineffectiveCompactions++
			// When the breaker trips, set a cooldown so it can recover later.
			if compState.ineffectiveCompactions >= 2 {
				cooldown := p.IneffectiveCooldown
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

	return compressEnd - compressStart, nil
}

func sanitizeToolPairs(msgs []Message) []Message {
	toolCallIDs := make(map[string]bool)
	var result []Message

	for _, msg := range msgs {
		switch {
		case msg.Role == RoleAssistant && len(msg.ToolCalls) > 0:
			for _, tc := range msg.ToolCalls {
				toolCallIDs[tc.ID] = true
			}
			result = append(result, msg)
		case msg.Role == RoleTool:
			if toolCallIDs[msg.ToolCallID] {
				result = append(result, msg)
			}
		default:
			result = append(result, msg)
		}
	}

	// Fast path: no tool calls were found, return original slice (CMP-009).
	if len(toolCallIDs) == 0 {
		return msgs
	}
	return result
}

// truncateToTokenBudget truncates content to fit a token budget, using the
// message's actual token density to compute a rune-safe cut point.
//
// CJK text has ~1.5 tokens/rune; ASCII code has ~0.25 tokens/rune. Using the
// real density (derived from msgTokens / runeCount) avoids over-truncating
// ASCII or under-truncating CJK. If the content is already within budget, it
// is returned unchanged.
func truncateToTokenBudget(content string, msgTokens, tokenBudget int64, marker string) string {
	if msgTokens <= tokenBudget {
		return content
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return content
	}
	tokensPerRune := float64(msgTokens) / float64(len(runes))
	if tokensPerRune < truncateMinTokensPerRune {
		tokensPerRune = truncateMinTokensPerRune
	}
	maxRunes := int(float64(tokenBudget) / tokensPerRune)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + marker
}
