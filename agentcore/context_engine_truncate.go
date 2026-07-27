package agentcore

import (
	"context"
)

// TruncateEngine is a simple context engine that drops old messages
// without LLM summarization. Useful for testing or when you want
// fast context management without the cost of a summary LLM call.
//
// It preserves:
//   - System message
//   - First N messages (ProtectFirstN)
//   - Last M tokens (KeepRecentTokens)
//
// Everything in the middle is dropped.
type TruncateEngine struct {
	contextLength    int64
	thresholdTokens  int64
	thresholdPercent float64
	protectFirstN    int
	keepRecentTokens int64
	compressionCnt   int64
}

// NewTruncateEngine creates a truncate-only context engine.
func NewTruncateEngine(cfg ContextEngineConfig) ContextEngine {
	return &TruncateEngine{
		contextLength:    cfg.ContextWindow,
		thresholdPercent: cfg.CompressionThreshold,
		protectFirstN:    cfg.ProtectFirstN,
		keepRecentTokens: cfg.KeepRecentTokens,
	}
}

// Name returns the engine identifier.
func (e *TruncateEngine) Name() string {
	return "truncate"
}

// OnSessionStart initializes per-session state.
func (e *TruncateEngine) OnSessionStart(_ context.Context, _ string, contextLength int64) {
	e.contextLength = contextLength
	if e.thresholdPercent > 0 {
		e.thresholdTokens = int64(float64(contextLength) * e.thresholdPercent)
	}
}

// OnSessionReset clears all per-session state.
func (e *TruncateEngine) OnSessionReset() {
	e.compressionCnt = 0
}

// OnSessionEnd is called at session termination.
func (e *TruncateEngine) OnSessionEnd() {}

// UpdateFromResponse is a no-op for the truncate engine.
func (e *TruncateEngine) UpdateFromResponse(_ TokenUsage) {}

// ShouldCompact returns true if compaction should fire this turn.
func (e *TruncateEngine) ShouldCompact(msgs []Message, toolDefs []ToolDefinition, contextWindow int64) bool {
	if contextWindow <= 0 {
		return false
	}
	// Respect the configured thresholdPercent (e.g., 0.8 triggers at 80% of
	// the window), falling back to a 75% default (== window - window/4) so
	// behavior matches CompressorEngine.shouldCompact when unset.
	triggerThreshold := int64(0)
	if e.thresholdPercent > 0 && e.thresholdPercent < 1.0 {
		triggerThreshold = int64(float64(contextWindow) * e.thresholdPercent)
	} else {
		triggerThreshold = contextWindow - contextWindow/4
	}
	estimated := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)
	return estimated > triggerThreshold
}

// Compress truncates the message list to fit within the context window.
func (e *TruncateEngine) Compress(_ context.Context, msgs []Message, _ string) ([]Message, int64, error) {
	if len(msgs) <= 3 {
		return msgs, 0, nil
	}

	headProtect := int64(e.protectFirstN)
	if headProtect <= 0 {
		headProtect = 3
	}

	// Find head boundary
	headEnd := int64(0)
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		headEnd = 1
	}
	nonSystemCount := int64(0)
	for i := headEnd; i < int64(len(msgs)); i++ {
		if msgs[i].Role != RoleSystem {
			nonSystemCount++
			if nonSystemCount >= headProtect {
				headEnd = i + 1
				break
			}
		}
	}

	// Fallback for unset keepRecentTokens (CMP-012).
	keepRecentTokens := e.keepRecentTokens
	if keepRecentTokens <= 0 {
		keepRecentTokens = 16384
	}

	// Find tail boundary by token budget
	tailStart := int64(len(msgs))
	accum := int64(0)
	for i := len(msgs) - 1; i >= int(headEnd); i-- {
		if msgs[i].Role == RoleSystem {
			continue
		}
		msgLen := EstimateMessageTokens(msgs[i])
		if accum+msgLen > keepRecentTokens && accum > 0 {
			tailStart = int64(i + 1)
			break
		}
		accum += msgLen
	}
	tailStart = alignBoundaryForward(msgs, tailStart)

	if headEnd >= tailStart {
		return msgs, 0, nil
	}

	tokensBefore := EstimateMessagesTokens(msgs)

	// Build truncated message list: head + tail
	result := make([]Message, 0, headEnd+int64(len(msgs))-tailStart+1)
	result = append(result, msgs[:headEnd]...)
	result = append(result, Message{
		Role:    RoleSystem,
		Content: "[CONTEXT TRUNCATION] Earlier messages were dropped to free context space. Continue based on the messages below.",
		Type:    MessageTypeCompactionSummary,
	})
	result = append(result, msgs[tailStart:]...)

	result = sanitizeToolPairs(result)

	e.compressionCnt++

	tokensAfter := EstimateMessagesTokens(result)
	saved := tokensBefore - tokensAfter

	return result, saved, nil
}

// GetToolSchemas returns nil — the truncate engine does not expose additional tools.
func (e *TruncateEngine) GetToolSchemas() []ToolDefinition {
	return nil
}

// ContextLength returns the model's context window size.
func (e *TruncateEngine) ContextLength() int64 {
	return e.contextLength
}

// ThresholdTokens returns the token count at which truncation triggers.
func (e *TruncateEngine) ThresholdTokens() int64 {
	return e.thresholdTokens
}

// CompressionCount returns the number of successful compressions.
func (e *TruncateEngine) CompressionCount() int64 {
	return e.compressionCnt
}

// LastSavingsPct returns the savings percentage of the last compression.
func (e *TruncateEngine) LastSavingsPct() float64 {
	return 0
}

// CheckFeasibility validates that the compression model can handle summarization.
func (e *TruncateEngine) CheckFeasibility(_ int64) string {
	return ""
}
