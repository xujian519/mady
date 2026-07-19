package agentcore

import (
	"context"
)

// TieredEngine implements a four-level progressive context compression pipeline.
//
// Instead of a single "summarize everything" approach, it applies increasingly
// aggressive compression as context usage grows:
//
//	0.5 — Soft Notice:  log warning, no modification
//	0.6 — Snip:         truncate old tool results to head+tail summary
//	0.8 — Prune:        replace old tool results with short placeholder
//	0.9 — Force Fold:   LLM summarization (delegates to CompressorEngine)
//
// This maximizes prefix cache hits by avoiding full summarization until
// absolutely necessary, and preserves tool_calls/result message pairs.
type TieredEngine struct {
	// Embedded compressor for force-fold level
	compressor *CompressorEngine

	// Configuration
	contextLength    int64
	thresholdTokens  int64
	keepRecentTokens int64
	protectFirstN    int

	// Level thresholds (ratios of contextLength)
	snipRatio  float64 // default 0.6
	pruneRatio float64 // default 0.8
	forceRatio float64 // default 0.9

	// Snip parameters
	snipHeadChars int // chars to keep from head of tool result (default 500)
	snipTailChars int // chars to keep from tail of tool result (default 200)

	// State
	compressionCnt int64
	lastSavingsPct float64

	// Tracks which messages have already been snipped/pruned to avoid
	// re-processing. Keyed by message index in the original slice.
	processed map[int]string // index → "snipped" | "pruned"
}

// NewTieredEngine creates a progressive tiered context engine.
func NewTieredEngine(cfg ContextEngineConfig) ContextEngine {
	e := &TieredEngine{
		contextLength:    cfg.ContextWindow,
		keepRecentTokens: cfg.KeepRecentTokens,
		protectFirstN:    cfg.ProtectFirstN,
		snipRatio:        0.6,
		pruneRatio:       0.8,
		forceRatio:       0.9,
		snipHeadChars:    500,
		snipTailChars:    200,
		processed:        map[int]string{},
	}
	// Create embedded compressor for force-fold
	e.compressor = &CompressorEngine{
		model:               cfg.Model,
		provider:            cfg.Provider,
		contextLength:       cfg.ContextWindow,
		thresholdPercent:    cfg.CompressionThreshold,
		protectFirstN:       cfg.ProtectFirstN,
		keepRecentTokens:    cfg.KeepRecentTokens,
		structured:          cfg.StructuredCompaction,
		autoCompactLimit:    cfg.AutoCompactLimit,
		compressionModel:    cfg.CompressionModel,
		compressionProvider: cfg.CompressionProvider,
		compressionBaseURL:  cfg.CompressionBaseURL,
		compressionAPIKey:   cfg.CompressionAPIKey,
		state:               newCompactionState(),
	}
	return e
}

func (e *TieredEngine) Name() string { return "tiered" }

func (e *TieredEngine) OnSessionStart(ctx context.Context, model string, contextLength int64) {
	e.contextLength = contextLength
	e.compressor.OnSessionStart(ctx, model, contextLength)
}

func (e *TieredEngine) OnSessionReset() {
	e.compressionCnt = 0
	e.processed = map[int]string{}
	e.compressor.OnSessionReset()
}

func (e *TieredEngine) OnSessionEnd() {
	e.compressor.OnSessionEnd()
}

func (e *TieredEngine) UpdateFromResponse(usage TokenUsage) {
	e.compressor.UpdateFromResponse(usage)
}

func (e *TieredEngine) ShouldCompact(msgs []Message, toolDefs []ToolDefinition, contextWindow int64) bool {
	if contextWindow <= 0 {
		return false
	}
	estimated := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)
	ratio := float64(estimated) / float64(contextWindow)
	return ratio >= e.snipRatio
}

func (e *TieredEngine) Compress(ctx context.Context, msgs []Message, focusTopic string) ([]Message, int64, error) {
	if len(msgs) <= 3 {
		return msgs, 0, nil
	}

	estimated := EstimateMessagesTokens(msgs)
	ratio := float64(estimated) / float64(e.contextLength)

	switch {
	case ratio >= e.forceRatio:
		return e.forceFold(ctx, msgs, focusTopic, estimated)

	case ratio >= e.pruneRatio:
		result := e.pruneToolResults(msgs)
		newTokens := EstimateMessagesTokens(result)
		saved := estimated - newTokens
		if newTokens > int64(float64(e.contextLength)*e.pruneRatio) {
			return e.forceFold(ctx, result, focusTopic, newTokens)
		}
		e.updateSavings(saved, estimated)
		return result, saved, nil

	case ratio >= e.snipRatio:
		result := e.snipToolResults(msgs)
		newTokens := EstimateMessagesTokens(result)
		saved := estimated - newTokens
		e.updateSavings(saved, estimated)
		return result, saved, nil

	default:
		return msgs, 0, nil
	}
}

func (e *TieredEngine) forceFold(ctx context.Context, msgs []Message, focusTopic string, displayTokens int64) ([]Message, int64, error) {
	result, saved, err := e.compressor.Compress(ctx, msgs, focusTopic)
	if err != nil {
		return msgs, 0, err
	}
	e.compressionCnt++
	e.processed = map[int]string{} // reset tracking after full fold
	e.updateSavings(saved, displayTokens)
	return result, saved, nil
}

// snipToolResults truncates old tool result messages to head+tail summaries.
// It preserves the last keepRecentTokens worth of messages untouched.
//
// Head/tail lengths are measured in RUNES (not bytes) so that multi-byte
// UTF-8 content (e.g., Chinese) is never split in the middle of a character,
// which would produce invalid UTF-8 and corrupt the provider request.
func (e *TieredEngine) snipToolResults(msgs []Message) []Message {
	tailStart := e.findTailBoundary(msgs)

	result := deepCopyMessages(msgs)

	for i := 0; i < tailStart && i < len(result); i++ {
		if result[i].Role != RoleTool {
			continue
		}
		if e.processed[i] == "snipped" || e.processed[i] == "pruned" {
			continue
		}
		runes := []rune(result[i].Content)
		if len(runes) <= e.snipHeadChars+e.snipTailChars+50 {
			continue // too short to snip
		}
		headEnd := min(e.snipHeadChars, len(runes))
		head := string(runes[:headEnd])
		tail := ""
		if len(runes) > e.snipHeadChars+e.snipTailChars {
			tail = string(runes[len(runes)-e.snipTailChars:])
		}
		result[i].Content = head + "\n[...已截断以节省上下文空间...]\n" + tail
		e.processed[i] = "snipped"
	}

	return sanitizeToolPairs(result)
}

// pruneToolResults replaces old tool results with a short placeholder.
// More aggressive than snip: the entire content is replaced.
func (e *TieredEngine) pruneToolResults(msgs []Message) []Message {
	tailStart := e.findTailBoundary(msgs)

	result := deepCopyMessages(msgs)

	for i := 0; i < tailStart && i < len(result); i++ {
		if result[i].Role != RoleTool {
			continue
		}
		if e.processed[i] == "pruned" {
			continue
		}
		content := result[i].Content
		if len(content) <= 100 {
			continue // too short to prune
		}
		result[i].Content = "[旧工具输出已清除以节省上下文空间]"
		e.processed[i] = "pruned"
	}

	return sanitizeToolPairs(result)
}

// findTailBoundary returns the index where the "recent" message zone begins.
// Messages at or after this index are protected from snip/prune.
func (e *TieredEngine) findTailBoundary(msgs []Message) int {
	tailBudget := e.keepRecentTokens
	if tailBudget <= 0 {
		tailBudget = 16384
	}

	accum := int64(0)
	for i := len(msgs) - 1; i >= 0; i-- {
		msgTokens := EstimateMessageTokens(msgs[i])
		if accum+msgTokens > tailBudget && accum > 0 {
			return i + 1
		}
		accum += msgTokens
	}
	return 0 // entire message list fits in tail budget
}

func (e *TieredEngine) updateSavings(saved, original int64) {
	if original > 0 {
		e.lastSavingsPct = float64(saved) / float64(original) * 100
	}
}

func (e *TieredEngine) GetToolSchemas() []ToolDefinition { return nil }

func (e *TieredEngine) ContextLength() int64 { return e.contextLength }

func (e *TieredEngine) ThresholdTokens() int64 {
	if e.thresholdTokens > 0 {
		return e.thresholdTokens
	}
	return int64(float64(e.contextLength) * e.snipRatio)
}

func (e *TieredEngine) CompressionCount() int64 { return e.compressionCnt }

func (e *TieredEngine) LastSavingsPct() float64 { return e.lastSavingsPct }

func (e *TieredEngine) CheckFeasibility(mainModelContextLength int64) string {
	return e.compressor.CheckFeasibility(mainModelContextLength)
}

// TierLevel reports the current compression level based on the ratio.
func (e *TieredEngine) TierLevel(msgs []Message, contextWindow int64) string {
	if contextWindow <= 0 {
		return "none"
	}
	estimated := EstimateMessagesTokens(msgs)
	ratio := float64(estimated) / float64(contextWindow)
	switch {
	case ratio >= e.forceRatio:
		return "force-fold"
	case ratio >= e.pruneRatio:
		return "prune"
	case ratio >= e.snipRatio:
		return "snip"
	case ratio >= 0.5:
		return "soft-notice"
	default:
		return "none"
	}
}

// SnipMessageContent truncates content to head+tail with a marker.
// Lengths are measured in runes so multi-byte UTF-8 is never split mid-character.
// Exported for testing.
func SnipMessageContent(content string, headChars, tailChars int) string {
	runes := []rune(content)
	if len(runes) <= headChars+tailChars+50 {
		return content
	}
	head := string(runes[:headChars])
	tail := string(runes[len(runes)-tailChars:])
	return head + "\n[...已截断以节省上下文空间...]\n" + tail
}

// PruneMessageContent replaces content with a short placeholder.
// Exported for testing.
func PruneMessageContent(_ string) string {
	return "[旧工具输出已清除以节省上下文空间]"
}

// IsToolResultProtected reports whether a message is in the protected tail zone.
// Exported for testing.
func (e *TieredEngine) IsToolResultProtected(msgs []Message, idx int) bool {
	tailStart := e.findTailBoundary(msgs)
	return idx >= tailStart
}

// deepCopyMessages creates a deep copy of a Message slice.
// Uses Message.Clone() to ensure all reference-type fields
// (Metadata, ToolCalls, Blocks, CacheControl) are independently copied.
func deepCopyMessages(msgs []Message) []Message {
	result := make([]Message, len(msgs))
	for i, msg := range msgs {
		result[i] = msg.Clone()
	}
	return result
}
