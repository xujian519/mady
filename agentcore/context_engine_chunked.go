package agentcore

import (
	"context"
	"fmt"
)

const chunkedEngineName = "chunked"

// ChunkedContextEngine wraps a base ContextEngine with document protection.
// It distinguishes between two types of messages in the conversation:
//
//   - Conversation messages: normal LLM dialog, tool calls, results.
//     These CAN be compacted by the base engine when token limits are hit.
//   - Protected document chunks: long documents (patent claims, statutes,
//     case law, etc.) injected as system messages with a special marker.
//     These are PRESERVED during compaction — only conversation messages
//     are compressed.
//
// This ensures that critical legal/patent text is never lost or summarized
// away, while the surrounding conversation can still be compacted normally.
//
// Usage:
//
//	engine := NewChunkedEngine(NewCompressorEngine(cfg))
//	// Mark a message as protected document content:
//	msg := Message{Role: RoleSystem, Content: patentText, Type: MessageTypeCustom}
//	msg.Metadata = map[string]any{"mady_doc_chunk": "protected"}
type ChunkedContextEngine struct {
	base ContextEngine

	// protectedIndices tracks which messages in the current conversation
	// are protected document chunks that should survive compaction.
	protectedIndices map[int]bool
}

// NewChunkedEngine creates a ChunkedContextEngine wrapping the given base engine.
// The factory is compatible with ContextEngineFactory for EngineRegistry.
func NewChunkedEngine(cfg ContextEngineConfig) ContextEngine {
	base := NewCompressorEngine(cfg)
	return &ChunkedContextEngine{
		base:             base,
		protectedIndices: make(map[int]bool),
	}
}

func (e *ChunkedContextEngine) Name() string { return chunkedEngineName }

func (e *ChunkedContextEngine) OnSessionStart(ctx context.Context, model string, contextLength int64) {
	e.base.OnSessionStart(ctx, model, contextLength)
	e.protectedIndices = make(map[int]bool)
}

func (e *ChunkedContextEngine) OnSessionReset() {
	e.base.OnSessionReset()
	e.protectedIndices = make(map[int]bool)
}

func (e *ChunkedContextEngine) OnSessionEnd() {
	e.base.OnSessionEnd()
}

func (e *ChunkedContextEngine) UpdateFromResponse(usage TokenUsage) {
	e.base.UpdateFromResponse(usage)
}

func (e *ChunkedContextEngine) ShouldCompact(msgs []Message, toolDefs []ToolDefinition, contextWindow int64) bool {
	return e.base.ShouldCompact(msgs, toolDefs, contextWindow)
}

// Compress performs compaction while preserving protected document chunks.
// It first identifies which messages are protected, compresses only the
// unprotected conversation messages via the base engine, then reassembles
// the message list with protected chunks in place.
func (e *ChunkedContextEngine) Compress(ctx context.Context, msgs []Message, focusTopic string) ([]Message, int64, error) {
	protected, unprotected := e.splitMessages(msgs)

	if len(unprotected) == 0 {
		return msgs, 0, nil
	}

	compressed, cut, err := e.compressUnprotected(ctx, unprotected, focusTopic)
	if err != nil {
		return msgs, 0, err
	}

	// Reconstruct in original order: walk original messages by index.
	// If msgs[i] was protected, place the next protected chunk at position i;
	// otherwise place the next compressed conversation message.
	// Surplus compressed messages (when compression expands) are appended
	// after the loop; unprotected slots without counterparts are skipped.
	result := make([]Message, 0, len(msgs))
	compIdx := 0
	protectedIdx := 0
	for i := range msgs {
		if e.protectedIndices[i] {
			result = append(result, protected[protectedIdx])
			protectedIdx++
		} else if compIdx < len(compressed) {
			result = append(result, compressed[compIdx])
			compIdx++
		}
	}
	// Append any surplus compressed messages beyond unprotected slots.
	for ; compIdx < len(compressed); compIdx++ {
		result = append(result, compressed[compIdx])
	}

	e.rebuildProtection(result)
	return result, cut, nil
}

// splitMessages separates messages into protected document chunks and
// unprotected conversation messages.
func (e *ChunkedContextEngine) splitMessages(msgs []Message) (protected, unprotected []Message) {
	e.protectedIndices = make(map[int]bool)

	for i, msg := range msgs {
		if e.isProtected(msg) {
			protected = append(protected, msg)
			e.protectedIndices[i] = true
		} else {
			unprotected = append(unprotected, msg)
		}
	}
	return
}

// isProtected checks if a message is a protected document chunk.
// Only messages with the explicit Metadata key "mady_doc_chunk" set to
// "protected" are considered protected. We intentionally do NOT use
// content-based heuristics (e.g. detecting "## " or "权利要求" in the text)
// because LLM-generated compaction summaries routinely contain those
// markers, which would cause summaries to be misclassified as protected
// and never compressed again — leading to unbounded context growth.
func (e *ChunkedContextEngine) isProtected(msg Message) bool {
	if v, ok := msg.Metadata["mady_doc_chunk"]; ok {
		if s, ok := v.(string); ok && s == "protected" {
			return true
		}
	}
	return false
}

// compressUnprotected performs compaction on the unprotected messages only.
// System prompt detection uses role-based lookup (msg.Role == RoleSystem)
// rather than assuming index 0, so system prompts are preserved regardless
// of their position in the message list (fix 4.7).
func (e *ChunkedContextEngine) compressUnprotected(ctx context.Context, msgs []Message, focusTopic string) ([]Message, int64, error) {
	// Extract system prompts by role, not by index position.
	var systemMsgs []Message
	nonSystem := make([]Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role == RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			nonSystem = append(nonSystem, msg)
		}
	}

	compressed, cut, err := e.base.Compress(ctx, nonSystem, focusTopic)
	if err != nil {
		return msgs, 0, fmt.Errorf("chunked compaction: %w", err)
	}

	// Re-insert system prompts at the front, preserving their original order.
	result := make([]Message, 0, len(systemMsgs)+len(compressed))
	result = append(result, systemMsgs...)
	result = append(result, compressed...)
	return result, cut, nil
}

// rebuildProtection scans the result messages and rebuilds the protection index.
func (e *ChunkedContextEngine) rebuildProtection(msgs []Message) {
	e.protectedIndices = make(map[int]bool)
	for i, msg := range msgs {
		if e.isProtected(msg) {
			e.protectedIndices[i] = true
		}
	}
}

// --- Passthrough methods ---

func (e *ChunkedContextEngine) GetToolSchemas() []ToolDefinition {
	return e.base.GetToolSchemas()
}

func (e *ChunkedContextEngine) ContextLength() int64 {
	return e.base.ContextLength()
}

func (e *ChunkedContextEngine) ThresholdTokens() int64 {
	return e.base.ThresholdTokens()
}

func (e *ChunkedContextEngine) CompressionCount() int64 {
	return e.base.CompressionCount()
}

func (e *ChunkedContextEngine) LastSavingsPct() float64 {
	return e.base.LastSavingsPct()
}

func (e *ChunkedContextEngine) CheckFeasibility(mainModelContextLength int64) string {
	return e.base.CheckFeasibility(mainModelContextLength)
}

// MarkAsProtected marks a message as a protected document chunk.
// This is the public API for domain code to protect document content from
// being lost during context compaction.
func MarkAsProtected(msg *Message) {
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]any)
	}
	msg.Metadata["mady_doc_chunk"] = "protected"
}
