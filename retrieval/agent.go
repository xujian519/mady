package retrieval

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// RetrievalHook is a LifecycleHook that automatically performs document
// retrieval before each model call and injects relevant chunks into
// the conversation context.
//
// Usage:
//
//	searcher := retrieval.NewKeywordSearcher()
//	hook := retrieval.NewRetrievalHook(searcher, chunks, retrieval.RetrievalConfig{
//	    TopK:       5,
//	    MaxChars:   3000,
//	    DomainHint: "patent",
//	})
//	cfg.Lifecycle = agentcore.LifecycleChain{hook}
type RetrievalHook struct {
	agentcore.BaseLifecycleHook

	searcher  Searcher
	reranker  Reranker
	chunks    []Chunk
	config    RetrievalConfig
	turnCount int64 // 内部轮次计数（用于 first_n 策略）
}

// RetrievalConfig controls how retrieved chunks are injected into context.
type RetrievalConfig struct {
	// TopK is the maximum number of chunks to retrieve (default: 5).
	TopK int
	// MaxChars limits the total character count of injected context (default: 3000).
	MaxChars int
	// DomainHint is a label prepended to the retrieval context block.
	// e.g., "patent", "legal", "general".
	DomainHint string
	// Embedder, when set, enables vector/hybrid search. The hook creates
	// a HybridSearcher combining the default KeywordSearcher with a
	// VectorSearcher powered by this Embedder.
	Embedder Embedder

	// HybridWeight controls the balance of vector vs keyword in hybrid
	// search mode. 0.0 = pure keyword, 1.0 = pure vector. Default: 0.7.
	HybridWeight float64

	// Prefix is prepended to the injected context block. Use this to
	// instruct the LLM on how to use the retrieved information.
	Prefix string

	// TriggerPolicy controls when retrieval is triggered.
	// "always" (default): retrieve every turn.
	// "smart": retrieve only when complexity is Medium or High.
	// "first_n": retrieve only for the first N turns.
	TriggerPolicy TriggerPolicy `json:"trigger_policy,omitempty"`

	// FirstNTurns is used when TriggerPolicy is "first_n".
	// Retrieval fires only for the first N turns.
	FirstNTurns int `json:"first_n_turns,omitempty"`

	// ComplexityClassifier is required when TriggerPolicy is "smart".
	// Reuses agentcore.ComplexityClassifier from ReasoningRouter.
	ComplexityClassifier ComplexityClassifier `json:"-"`
}

// TriggerPolicy controls when knowledge retrieval fires.
type TriggerPolicy string

const (
	TriggerAlways   TriggerPolicy = "always"    // 每轮都检索（默认）
	TriggerSmart    TriggerPolicy = "smart"     // 按复杂度门控（复用 ReasoningRouter）
	TriggerFirstN   TriggerPolicy = "first_n"   // 仅前 N 轮
	TriggerOnDemand TriggerPolicy = "on_demand" // 仅通过工具触发
)

// ComplexityClassifier wraps agentcore.ComplexityClassifier to avoid a hard import.
type ComplexityClassifier interface {
	Classify(input string, messages []agentcore.Message) agentcore.Complexity
}

// DefaultRetrievalConfig returns sensible defaults.
func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		TopK:          5,
		MaxChars:      3000,
		Prefix:        "以下是检索到的相关参考信息，请在回答时参考：\n",
		TriggerPolicy: TriggerAlways,
		FirstNTurns:   3,
	}
}

// NewRetrievalHook creates a RetrievalHook. If config.Embedder is set,
// a HybridSearcher (keyword + vector) is used; otherwise keyword-only.
func NewRetrievalHook(chunks []Chunk, config RetrievalConfig) *RetrievalHook {
	if config.TopK <= 0 {
		config = DefaultRetrievalConfig()
	}
	if config.HybridWeight <= 0 {
		config.HybridWeight = 0.7
	}

	var searcher Searcher
	if config.Embedder != nil {
		kw := NewKeywordSearcher()
		vec := NewVectorSearcher(config.Embedder)
		hs := NewHybridSearcher(kw, vec)
		hs.Weight = config.HybridWeight
		searcher = hs
	} else {
		searcher = NewKeywordSearcher()
	}

	// Auto-inject DefaultClassifier when TriggerSmart is used without one.
	if config.TriggerPolicy == TriggerSmart && config.ComplexityClassifier == nil {
		config.ComplexityClassifier = agentcore.NewDefaultClassifier()
	}

	// Default FirstNTurns when using TriggerFirstN without explicit value.
	if config.TriggerPolicy == TriggerFirstN && config.FirstNTurns <= 0 {
		config.FirstNTurns = 3
	}

	return &RetrievalHook{
		searcher: searcher,
		reranker: NewPositionReranker(),
		chunks:   chunks,
		config:   config,
	}
}

// NewRetrievalHookWithSearcher creates a RetrievalHook with a custom Searcher and Reranker.
func NewRetrievalHookWithSearcher(searcher Searcher, reranker Reranker, chunks []Chunk, config RetrievalConfig) *RetrievalHook {
	if config.TopK <= 0 {
		config = DefaultRetrievalConfig()
	}
	return &RetrievalHook{
		searcher: searcher,
		reranker: reranker,
		chunks:   chunks,
		config:   config,
	}
}

// UpdateChunks replaces the document chunk set at runtime.
func (h *RetrievalHook) UpdateChunks(chunks []Chunk) {
	h.chunks = chunks
}

// BeforeModelCall implements LifecycleHook.BeforeModelCall.
// It checks the trigger policy, then searches the chunk index using the
// latest user message as query and injects relevant chunks into context.
func (h *RetrievalHook) BeforeModelCall(ctx context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) error {
	if len(h.chunks) == 0 || mcc == nil || mcc.Request == nil {
		return nil
	}

	// Compute the query once and reuse (RTV-018).
	query := agentcore.LastUserMessage(arc.Messages)

	// Check trigger policy using the cached query.
	if !h.shouldTrigger(arc, query) {
		return nil
	}

	h.turnCount++

	if query == "" {
		return nil
	}

	// Search and rerank.
	results := h.searcher.Search(ctx, query, h.chunks, h.config.TopK)
	if h.reranker != nil {
		results = h.reranker.Rerank(results)
	}

	if len(results) == 0 {
		return nil
	}

	// Build context block.
	contextBlock := h.buildContextBlock(results)
	if contextBlock == "" {
		return nil
	}

	// Inject into system messages.
	h.injectContext(mcc.Request, contextBlock)
	return nil
}

// shouldTrigger checks if retrieval should fire this turn.
func (h *RetrievalHook) shouldTrigger(arc *agentcore.AgentRunContext, query string) bool {
	switch h.config.TriggerPolicy {
	case TriggerSmart:
		return ShouldTriggerSmart(query, arc.Messages, h.config.ComplexityClassifier)
	default:
		return ShouldTrigger(h.config.TriggerPolicy, int(h.turnCount), h.config.FirstNTurns)
	}
}

// buildContextBlock formats retrieved chunks into a single context string.
func (h *RetrievalHook) buildContextBlock(results []ScoredChunk) string {
	return FormatContextBlock(results, h.config)
}

// injectContext prepends the retrieval context as a system message.
func (h *RetrievalHook) injectContext(req *agentcore.ProviderRequest, contextBlock string) {
	InjectContext(req, contextBlock)
}
