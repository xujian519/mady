package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

// KnowledgeBackend provides SQLite-backed knowledge retrieval. When set on
// the extension, it takes priority over the in-memory Store. Implementations
// are expected to be the SQLiteStore in knowledge/sqlite, but the interface
// keeps the knowledge package free of import cycles.
type KnowledgeBackend interface {
	FTSSearch(query string, topK int) ([]retrieval.ScoredChunk, error)
	VectorSearch(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error)
}

// WritableBackend provides user-document search and write capabilities within
// a writable user database (user.db). Search performs its own FTS+Vector RRF
// fusion internally and returns a single ranked list that participates as a
// third RRF lane alongside knowledge FTS and knowledge Vector. AddDocument
// chunks, embeds, and persists a new document for future retrieval.
//
// This interface is defined here (not imported from knowledge/sqlite) to
// preserve the domain-layer dependency boundary (ADR-0001).
type WritableBackend interface {
	Search(ctx context.Context, query string, topK int) ([]retrieval.ScoredChunk, error)
	AddDocument(ctx context.Context, docID, title, content string) error
}

// ExtensionName is the unique identifier for the knowledge extension.
const ExtensionName = "knowledge"

// GraphEnhancer expands retrieval results with similar cases and citation
// chains from the knowledge graph.
type GraphEnhancer interface {
	Enhance(seeds []retrieval.ScoredChunk) any
}

// GraphEnhancement is the result interface returned by GraphEnhancer.Enhance().
// It avoids import cycles between knowledge and knowledge/graph packages.
type GraphEnhancement interface {
	GetContext() string
}

// LawRecord represents a single law from the laws database.
type LawRecord struct {
	ID       string
	Level    string // 法律/行政法规/司法解释/部门规章
	Name     string
	Subtitle string
	Content  string
	Category string
}

// LawSearcher is a function type for searching laws by keyword.
// Implementations typically delegate to knowledge/sqlite.SQLiteStore.SearchLaws.
type LawSearcher func(keyword string, topK int) ([]LawRecord, error)

// KnowledgeExtension integrates knowledge retrieval into the agent lifecycle.
// It supports both in-memory keyword search and SQLite-backed FTS+vector RRF
// fusion, with optional graph enhancement, cross-encoder reranking, and law
// search capabilities.
type KnowledgeExtension struct {
	agentcore.BaseLifecycleHook
	store         *Store
	backend       KnowledgeBackend
	embedder      retrieval.Embedder
	queryReranker retrieval.QueryReranker
	writable      WritableBackend
	graph         GraphEnhancer
	lawSearcher   LawSearcher
	hook          *retrieval.RetrievalHook
	evalHook      *EvalHook
	domain        string
	cfg           KnowledgeExtConfig

	// lastGraphCtx caches the most recent graph enhancement context so
	// BackendRetrievalHook can inject it alongside chunk results.
	// Protected by graphMu for concurrent access from ACP multi-client sessions.
	lastGraphCtx string
	graphMu      sync.RWMutex

	// memorySearcher and memoryReranker are cached instances to avoid
	// re-allocating NewKeywordSearcher / NewPositionReranker on each
	// memorySearch call (fix 4.4).
	memorySearcher retrieval.Searcher
	memoryReranker retrieval.Reranker

	// queryCache caches query embedding vectors to avoid redundant API calls.
	// When a query is semantically similar (cosine > threshold) to a cached
	// query, the cached vector is reused instead of calling the embedding API.
	// Created by default; disabled by setting capacity to 0.
	queryCache *queryEmbedCache

	// topKBoost is set by EvalHook when it observes persistent low faithfulness.
	// BackendRetrievalHook reads this flag to dynamically increase TopK.
	topKBoost atomic.Bool

	// extraTools holds tools injected by bootstrap from sub-packages that
	// cannot be referenced directly here due to import-cycle constraints.
	extraTools []*agentcore.Tool
}

// WithBackend injects a SQLite-backed knowledge retrieval backend and an
// optional embedder for vector search. When set, the extension uses FTS +
// vector RRF fusion instead of the in-memory keyword search.
func (e *KnowledgeExtension) WithBackend(backend KnowledgeBackend, embedder retrieval.Embedder) *KnowledgeExtension {
	e.backend = backend
	e.embedder = embedder
	return e
}

// WithReranker injects a query-aware cross-encoder reranker. When set,
// backendSearch applies the reranker after RRF fusion to re-score
// candidates against the user's query, improving precision for the
// top-K results.
func (e *KnowledgeExtension) WithReranker(reranker retrieval.QueryReranker) *KnowledgeExtension {
	e.queryReranker = reranker
	return e
}

// WithWritableStore injects a user-document writable backend (user.db).
// When set, backendSearch adds a third RRF lane alongside knowledge FTS
// and knowledge Vector, and the add_document tool is exposed to the agent.
func (e *KnowledgeExtension) WithWritableStore(w WritableBackend) *KnowledgeExtension {
	e.writable = w
	return e
}

// WithGraph injects a knowledge-graph enhancer. When set, backendSearch
// calls Enhance() after RRF fusion to expand results with similar cases
// and citation chains from the knowledge graph.
func (e *KnowledgeExtension) WithGraph(g GraphEnhancer) *KnowledgeExtension {
	e.graph = g
	return e
}

// RegisterTools appends additional tools to the extension's tool list.
// Used by bootstrap to inject patent-specific tools (e.g. patent_kg_query)
// that are defined in sub-packages to avoid import cycles.
func (e *KnowledgeExtension) RegisterTools(tools ...*agentcore.Tool) *KnowledgeExtension {
	e.extraTools = append(e.extraTools, tools...)
	return e
}

// LawSearcher returns the configured law search function, or nil if not set.
// Exposed for downstream consumers (e.g., enablement trigger) that need to
// reuse the same law database connection for domain-specific retrieval.
func (e *KnowledgeExtension) LawSearcher() LawSearcher {
	return e.lawSearcher
}

// WithLawSearcher injects a law search function. When set, the search_laws
// tool is exposed to the agent for full-text law retrieval.
func (e *KnowledgeExtension) WithLawSearcher(fn LawSearcher) *KnowledgeExtension {
	e.lawSearcher = fn
	return e
}

// KnowledgeExtConfig configures the knowledge extension behavior.
type KnowledgeExtConfig struct {
	Enabled         bool                      `json:"enabled"`
	Domain          string                    `json:"domain"`
	ExposeTool      bool                      `json:"expose_tool"`
	RetrievalConfig retrieval.RetrievalConfig `json:"-"`
}

// Validate checks that the knowledge extension configuration is valid.
func (c KnowledgeExtConfig) Validate() error {
	if c.RetrievalConfig.TopK <= 0 {
		c.RetrievalConfig.TopK = 10
	}
	return nil
}

// DefaultKnowledgeExtConfig returns a default configuration with knowledge
// retrieval enabled and the tool exposed.
func DefaultKnowledgeExtConfig() KnowledgeExtConfig {
	return KnowledgeExtConfig{
		Enabled:         true,
		ExposeTool:      true,
		RetrievalConfig: retrieval.DefaultRetrievalConfig(),
	}
}

// NewExtension creates a KnowledgeExtension with the given store, graph
// enhancer, domain, and configuration.
func NewExtension(store *Store, g GraphEnhancer, domain string, cfg KnowledgeExtConfig) *KnowledgeExtension {
	if cfg.RetrievalConfig.TopK <= 0 {
		cfg.RetrievalConfig = retrieval.DefaultRetrievalConfig()
	}
	var chunks []retrieval.Chunk
	if store != nil {
		chunks = store.SearchableChunksForDomain(domain)
		if len(chunks) == 0 {
			chunks = store.AllChunks()
		}
	}
	cfg.RetrievalConfig.DomainHint = domain
	cfg.Domain = domain
	evalCfg := DefaultEvalConfig()
	ext := &KnowledgeExtension{
		store:      store,
		graph:      g,
		hook:       retrieval.NewRetrievalHook(chunks, cfg.RetrievalConfig),
		domain:     domain,
		cfg:        cfg,
		queryCache: newQueryEmbedCache(64),
	}
	ext.evalHook = NewEvalHookWithExt(evalCfg, func(enable bool) {
		ext.topKBoost.Store(enable)
	})
	return ext
}

var (
	_ agentcore.Extension                = (*KnowledgeExtension)(nil)
	_ agentcore.LifecycleProvider        = (*KnowledgeExtension)(nil)
	_ agentcore.ToolProvider             = (*KnowledgeExtension)(nil)
	_ agentcore.TransformContextProvider = (*KnowledgeExtension)(nil)
)

// Name returns the extension identifier.
func (e *KnowledgeExtension) Name() string { return ExtensionName }

// Init performs no-op initialization for the knowledge extension.
func (e *KnowledgeExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }

// Dispose performs no-op cleanup for the knowledge extension.
func (e *KnowledgeExtension) Dispose() error { return nil }

// LifecycleHook returns the retrieval lifecycle hook (backend or in-memory).
func (e *KnowledgeExtension) LifecycleHook() agentcore.LifecycleHook { //nolint:staticcheck
	if e.backend != nil {
		h := NewBackendRetrievalHook(e, e.cfg.RetrievalConfig)
		if e.evalHook != nil {
			return agentcore.AppendLifecycle(h, e.evalHook)
		}
		return h
	}
	return e.hook
}

// TransformContext is a pass-through; the knowledge extension does not modify
// the message context.
func (e *KnowledgeExtension) TransformContext(_ context.Context, msgs []agentcore.Message) []agentcore.Message {
	return msgs
}

// Tools returns the list of tools exposed by the knowledge extension
// (search_knowledge, optionally search_laws and add_document).
func (e *KnowledgeExtension) Tools() []*agentcore.Tool {
	if !e.cfg.ExposeTool {
		return nil
	}
	tools := []*agentcore.Tool{
		{
			Name:        "search_knowledge",
			Description: "搜索知识库，获取与当前问题相关的文档、法律条文、案例等信息。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "搜索查询"},
					"top_k": map[string]any{"type": "integer", "default": 5},
				},
				"required": []string{"query"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleSearch(ctx, args)
			},
		},
	}
	if e.lawSearcher != nil {
		tools = append(tools, &agentcore.Tool{
			Name:        "search_laws",
			Description: "搜索法律法规数据库（9121部法律），按法律名称或条文内容关键词匹配，返回法律全文。适用于查找具体法律条文、核实法律依据。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "法律法规名称或条文关键词"},
					"top_k": map[string]any{"type": "integer", "default": 5},
				},
				"required": []string{"query"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleSearchLaws(ctx, args)
			},
		})
	}
	if e.writable != nil {
		tools = append(tools, &agentcore.Tool{
			Name:        "add_document",
			Description: "将用户文档添加到知识库。文档会被自动分块、向量化并入库，之后可通过 search_knowledge 检索到。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"doc_id":  map[string]any{"type": "string", "description": "文档唯一标识（如 user-001）"},
					"title":   map[string]any{"type": "string", "description": "文档标题"},
					"content": map[string]any{"type": "string", "description": "文档正文内容"},
				},
				"required": []string{"doc_id", "title", "content"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleAddDocument(ctx, args)
			},
		})
	}

	if len(e.extraTools) > 0 {
		tools = append(tools, e.extraTools...)
	}
	return tools
}

// GraphContext returns the graph-enhanced context block from the most recent
// search. Returns empty string when no graph enhancer is configured or the
// last search produced no enhancement.
func (e *KnowledgeExtension) GraphContext() string {
	e.graphMu.RLock()
	defer e.graphMu.RUnlock()
	return e.lastGraphCtx
}

// Layer returns the context layer identifier for knowledge retrieval.
func (e *KnowledgeExtension) Layer() agentcore.ContextLayer { return agentcore.LayerKnowledge }

// Provide injects knowledge context as system messages into the agent's
// message list. It retrieves chunks relevant to the last user message.
func (e *KnowledgeExtension) Provide(ctx context.Context, input agentcore.BuildInput, _ agentcore.LayerConfig) ([]agentcore.Message, error) {
	if !e.cfg.Enabled {
		return nil, nil
	}
	if e.store == nil && e.backend == nil {
		return nil, nil
	}
	query := lastUserMsg(input.Messages)
	if query == "" {
		return nil, nil
	}

	topK := e.cfg.RetrievalConfig.TopK
	if topK <= 0 {
		topK = 5
	}
	results := e.search(ctx, query, topK)
	if len(results) == 0 {
		return nil, nil
	}

	var b strings.Builder
	b.WriteString("### 参考文档\n")
	for i, r := range results {
		fmt.Fprintf(&b, "--- [%d] (%.2f) ---\n%s\n", i+1, r.Score, r.Content)
	}
	return []agentcore.Message{{Role: agentcore.RoleSystem, Content: b.String()}}, nil
}
