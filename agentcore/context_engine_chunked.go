package agentcore

import (
	"context"
	"fmt"
	"strings"
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
	// Identify protected and unprotected messages.
	protected, unprotected := e.splitMessages(msgs)

	// If everything is protected, skip compaction entirely.
	if len(unprotected) == 0 {
		return msgs, 0, nil
	}

	// Compress only the unprotected conversation messages.
	compressed, cut, err := e.compressUnprotected(ctx, unprotected, focusTopic)
	if err != nil {
		return msgs, 0, err
	}

	// Reassemble: protected messages first (document context),
	// then compressed conversation.
	result := make([]Message, 0, len(protected)+len(compressed))
	result = append(result, protected...)
	result = append(result, compressed...)

	// Rebuild the protection index.
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
// Messages are protected if they have the Metadata key "mady_doc_chunk"
// set to "protected", or if they have Content over a threshold and
// contain section markers typical of patent/legal documents.
func (e *ChunkedContextEngine) isProtected(msg Message) bool {
	// Explicit protection marker.
	if v, ok := msg.Metadata["mady_doc_chunk"]; ok {
		if s, ok := v.(string); ok && s == "protected" {
			return true
		}
	}

	// Implicit protection: large messages with document structure markers.
	if len(msg.Content) > 1500 && (msg.Role == RoleSystem || msg.Type == MessageTypeCustom) {
		structural := []string{
			"权利要求", "## ", "法律依据", "判例要旨",
			"技术领域", "背景技术", "发明内容", "具体实施方式",
			"法条", "司法解释", "裁判要点",
		}
		for _, marker := range structural {
			if strings.Contains(msg.Content, marker) {
				return true
			}
		}
	}

	return false
}

// compressUnprotected performs compaction on the unprotected messages only.
func (e *ChunkedContextEngine) compressUnprotected(ctx context.Context, msgs []Message, focusTopic string) ([]Message, int64, error) {
	// Create a temporary state with only the unprotected messages.
	tmpState := NewState()
	tmpState.ReplaceMessages(msgs)

	compressed, cut, err := e.base.Compress(ctx, msgs, focusTopic)
	if err != nil {
		return msgs, 0, fmt.Errorf("chunked compaction: %w", err)
	}

	_ = tmpState
	return compressed, cut, nil
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
