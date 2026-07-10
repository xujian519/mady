package retrieval

import (
	"context"
	"fmt"
	"strings"

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

	searcher Searcher
	reranker Reranker
	chunks   []Chunk
	config   RetrievalConfig
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
}

// DefaultRetrievalConfig returns sensible defaults.
func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		TopK:     5,
		MaxChars: 3000,
		Prefix:   "以下是检索到的相关参考信息，请在回答时参考：\n",
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
// It searches the chunk index using the latest user message as query and
// injects relevant chunks into the system prompt via TransformContext.
func (h *RetrievalHook) BeforeModelCall(_ context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) error {
	if len(h.chunks) == 0 || mcc == nil || mcc.Request == nil {
		return nil
	}

	// Use the last user message as the search query.
	query := lastUserMessage(arc.Messages)
	if query == "" {
		return nil
	}

	// Search and rerank.
	results := h.searcher.Search(query, h.chunks, h.config.TopK)
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

// lastUserMessage extracts the content of the last user message.
func lastUserMessage(msgs []agentcore.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agentcore.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

// buildContextBlock formats retrieved chunks into a single context string.
func (h *RetrievalHook) buildContextBlock(results []ScoredChunk) string {
	var b strings.Builder

	prefix := h.config.Prefix
	if prefix == "" {
		prefix = "以下是检索到的相关参考信息，请在回答时参考：\n"
	}
	b.WriteString(prefix)

	totalChars := 0
	for i, r := range results {
		if totalChars >= h.config.MaxChars {
			break
		}

		chunkText := r.Content
		if totalChars+len(chunkText) > h.config.MaxChars {
			chunkText = chunkText[:h.config.MaxChars-totalChars] + "..."
		}

		fmt.Fprintf(&b, "\n--- 参考片段 %d (相关度: %.2f) ---\n", i+1, r.Score)
		if h.config.DomainHint != "" {
			fmt.Fprintf(&b, "[来源: %s/%s]\n", h.config.DomainHint, r.DocID)
		}
		b.WriteString(chunkText)
		b.WriteString("\n")
		totalChars += len(chunkText) + 100 // 100 for header overhead
	}

	return b.String()
}

// injectContext prepends the retrieval context as a system message.
func (h *RetrievalHook) injectContext(req *agentcore.ProviderRequest, contextBlock string) {
	if contextBlock == "" {
		return
	}

	// Insert as the last system message so it appears right before the
	// conversation history, giving the LLM immediate access to the
	// retrieved context.
	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}

	// Insert before the last system message (if any) or at the beginning.
	insertIdx := 0
	for i, msg := range req.Messages {
		if msg.Role == agentcore.RoleSystem {
			insertIdx = i + 1
		}
	}

	req.Messages = append(
		req.Messages[:insertIdx],
		append([]agentcore.Message{sysMsg}, req.Messages[insertIdx:]...)...,
	)
}
