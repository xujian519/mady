package knowledge

import (
	"context"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

// BackendRetrievalHook is a LifecycleHook that performs knowledge retrieval
// via a KnowledgeBackend (SQLite FTS + vector RRF fusion) before each model
// call. Unlike retrieval.RetrievalHook, it does not require pre-loaded
// in-memory chunks — the backend searches the SQLite database directly.
//
// This hook delegates the actual search to KnowledgeExtension.search(),
// which dispatches to backendSearch (FTS + vector RRF) when a backend is
// configured. Context formatting and injection mirror RetrievalHook but
// are reimplemented here to avoid modifying the retrieval package.
type BackendRetrievalHook struct {
	agentcore.BaseLifecycleHook

	ext       *KnowledgeExtension
	config    retrieval.RetrievalConfig
	turnCount int64 // internal turn counter for first_n policy
}

// NewBackendRetrievalHook creates a hook that delegates search to the
// KnowledgeExtension's backend (FTS + vector RRF fusion).
// The extension must have a backend configured (via WithBackend) before
// this hook is used; otherwise search will fall back to memorySearch
// which returns nil when store is nil.
func NewBackendRetrievalHook(ext *KnowledgeExtension, cfg retrieval.RetrievalConfig) *BackendRetrievalHook {
	if cfg.TopK <= 0 {
		cfg = retrieval.DefaultRetrievalConfig()
	}
	// Auto-inject DefaultClassifier when TriggerSmart is used without one.
	if cfg.TriggerPolicy == retrieval.TriggerSmart && cfg.ComplexityClassifier == nil {
		cfg.ComplexityClassifier = agentcore.NewDefaultClassifier()
	}
	// Default FirstNTurns when using TriggerFirstN without explicit value.
	if cfg.TriggerPolicy == retrieval.TriggerFirstN && cfg.FirstNTurns <= 0 {
		cfg.FirstNTurns = 3
	}
	return &BackendRetrievalHook{
		ext:    ext,
		config: cfg,
	}
}

// BeforeModelCall implements LifecycleHook.BeforeModelCall.
// It extracts the last user message, checks trigger policy, searches the
// backend knowledge store, and injects relevant chunks into the model's
// context as a system message. When a graph enhancer is configured,
// graph-enhanced context (similar cases, citation chains) is appended
// after the chunk results.
func (h *BackendRetrievalHook) BeforeModelCall(ctx context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) error {
	if mcc == nil || mcc.Request == nil {
		return nil
	}

	// Check trigger policy before performing expensive backend search.
	if !h.shouldTrigger(arc) {
		return nil
	}
	h.turnCount++

	query := agentcore.LastUserMessage(arc.Messages)
	if query == "" {
		return nil
	}

	results := h.ext.search(ctx, query, h.config.TopK)
	if len(results) == 0 {
		return nil
	}

	contextBlock := h.buildContextBlock(results)
	if contextBlock == "" {
		return nil
	}

	// Append graph-enhanced context (similar cases, citation chains) if available.
	if graphCtx := h.ext.GraphContext(); graphCtx != "" {
		contextBlock += "\n\n" + graphCtx
	}

	h.injectContext(mcc.Request, contextBlock)
	return nil
}

// shouldTrigger checks if retrieval should fire this turn.
func (h *BackendRetrievalHook) shouldTrigger(arc *agentcore.AgentRunContext) bool {
	switch h.config.TriggerPolicy {
	case retrieval.TriggerSmart:
		query := agentcore.LastUserMessage(arc.Messages)
		return retrieval.ShouldTriggerSmart(query, arc.Messages, h.config.ComplexityClassifier)
	default:
		return retrieval.ShouldTrigger(h.config.TriggerPolicy, int(h.turnCount), h.config.FirstNTurns)
	}
}

// buildContextBlock formats retrieved chunks into a context string,
// respecting MaxChars budget.
func (h *BackendRetrievalHook) buildContextBlock(results []retrieval.ScoredChunk) string {
	return retrieval.FormatContextBlock(results, h.config)
}

// injectContext prepends the retrieval context as a system message,
// inserted after the last existing system message.
func (h *BackendRetrievalHook) injectContext(req *agentcore.ProviderRequest, contextBlock string) {
	retrieval.InjectContext(req, contextBlock)
}
