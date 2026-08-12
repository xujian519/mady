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

//nolint:gocognit // 原因：上下文压缩执行，含多策略和重试逻辑
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
