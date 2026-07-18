package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

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

const ExtensionName = "knowledge"

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

// WithLawSearcher injects a law search function. When set, the search_laws
// tool is exposed to the agent for full-text law retrieval.
func (e *KnowledgeExtension) WithLawSearcher(fn LawSearcher) *KnowledgeExtension {
	e.lawSearcher = fn
	return e
}

type KnowledgeExtConfig struct {
	Enabled         bool                      `json:"enabled"`
	Domain          string                    `json:"domain"`
	ExposeTool      bool                      `json:"expose_tool"`
	RetrievalConfig retrieval.RetrievalConfig `json:"-"`
}

func DefaultKnowledgeExtConfig() KnowledgeExtConfig {
	return KnowledgeExtConfig{
		Enabled:         true,
		ExposeTool:      true,
		RetrievalConfig: retrieval.DefaultRetrievalConfig(),
	}
}

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
	return &KnowledgeExtension{
		store:    store,
		graph:    g,
		hook:     retrieval.NewRetrievalHook(chunks, cfg.RetrievalConfig),
		evalHook: NewEvalHook(evalCfg),
		domain:   domain,
		cfg:      cfg,
	}
}

var (
	_ agentcore.Extension                = (*KnowledgeExtension)(nil)
	_ agentcore.LifecycleProvider        = (*KnowledgeExtension)(nil)
	_ agentcore.ToolProvider             = (*KnowledgeExtension)(nil)
	_ agentcore.TransformContextProvider = (*KnowledgeExtension)(nil)
)

func (e *KnowledgeExtension) Name() string                                     { return ExtensionName }
func (e *KnowledgeExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *KnowledgeExtension) Dispose() error                                   { return nil }

func (e *KnowledgeExtension) LifecycleHook() agentcore.LifecycleHook { return e.hook }

// BackendHook returns a LifecycleHook that performs retrieval via the
// configured backend (SQLite FTS + vector RRF fusion). Unlike LifecycleHook
// (which returns a RetrievalHook that requires pre-loaded in-memory chunks),
// this hook searches the backend database directly on each model call.
// When the default eval config is enabled, an EvalHook is composed into
// the returned lifecycle for retrieval quality measurement.
// Returns nil if no backend is configured.
func (e *KnowledgeExtension) BackendHook(cfg retrieval.RetrievalConfig) agentcore.LifecycleHook {
	if e.backend == nil {
		return nil
	}
	h := NewBackendRetrievalHook(e, cfg)
	if e.evalHook != nil {
		return agentcore.AppendLifecycle(h, e.evalHook)
	}
	return h
}

func (e *KnowledgeExtension) TransformContext(_ context.Context, msgs []agentcore.Message) []agentcore.Message {
	return msgs
}

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
	return tools
}

func (e *KnowledgeExtension) handleSearch(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Query == "" {
		return "请提供搜索查询", nil
	}
	if p.TopK <= 0 {
		p.TopK = 5
	}

	results := e.search(ctx, p.Query, p.TopK)
	if len(results) == 0 {
		return "未找到相关文档", nil
	}
	return formatToolResults(results), nil
}

// Search performs knowledge retrieval and returns scored chunks.
// When a SQLite backend is configured, this uses FTS + vector RRF fusion
// (with optional cross-encoder reranking). Otherwise it falls back to
// in-memory keyword search.
func (e *KnowledgeExtension) Search(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	return e.search(ctx, query, topK)
}

// GraphContext returns the graph-enhanced context block from the most recent
// search. Returns empty string when no graph enhancer is configured or the
// last search produced no enhancement.
func (e *KnowledgeExtension) GraphContext() string {
	e.graphMu.RLock()
	defer e.graphMu.RUnlock()
	return e.lastGraphCtx
}

// handleSearchLaws processes the search_laws tool call. It delegates to
// the configured LawSearcher function.
func (e *KnowledgeExtension) handleSearchLaws(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Query == "" {
		return "请提供搜索查询", nil
	}
	if p.TopK <= 0 {
		p.TopK = 5
	}
	if e.lawSearcher == nil {
		return "法律法规搜索功能未启用", nil
	}

	results, err := e.lawSearcher(p.Query, p.TopK)
	if err != nil {
		return fmt.Sprintf("搜索法律法规失败: %v", err), nil
	}
	if len(results) == 0 {
		return fmt.Sprintf("未找到与 \"%s\" 相关的法律法规", p.Query), nil
	}

	var b strings.Builder
	b.WriteString("法律法规搜索结果:\n")
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] %s (%s)\n", i+1, r.Name, r.Level)
		if r.Subtitle != "" {
			fmt.Fprintf(&b, "    %s\n", r.Subtitle)
		}
		fmt.Fprintf(&b, "    分类: %s\n", r.Category)
		// Truncate content to avoid overwhelming the model.
		content := r.Content
		if len(content) > 2000 {
			content = content[:2000] + "..."
		}
		fmt.Fprintf(&b, "    %s\n", content)
	}
	fmt.Fprintf(&b, "\n共 %d 条结果", len(results))
	return b.String(), nil
}

// handleAddDocument processes the add_document tool call. It delegates to
// the configured WritableBackend to chunk, embed, and persist the document.
func (e *KnowledgeExtension) handleAddDocument(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		DocID   string `json:"doc_id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.DocID == "" {
		return "请提供文档标识 (doc_id)", nil
	}
	if p.Content == "" {
		return "请提供文档内容 (content)", nil
	}
	if e.writable == nil {
		return "文档写入功能未启用", nil
	}
	if err := e.writable.AddDocument(ctx, p.DocID, p.Title, p.Content); err != nil {
		return fmt.Sprintf("文档添加失败: %v", err), nil
	}
	return fmt.Sprintf("文档 \"%s\" 已成功添加到知识库（%d 字符）", p.DocID, len(p.Content)), nil
}

// search dispatches to the SQLite backend (FTS + vector RRF fusion) when
// available, falling back to the in-memory keyword search otherwise.
func (e *KnowledgeExtension) search(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	if e.backend != nil {
		return e.backendSearch(ctx, query, topK)
	}
	return e.memorySearch(query, topK)
}

// backendSearch performs FTS + vector RRF fusion via the SQLite backend.
// When a writable user store is configured, a third lane (user documents)
// is added to the RRF fusion.
func (e *KnowledgeExtension) backendSearch(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	candidateK := topK * 2
	if candidateK < 20 {
		candidateK = 20
	}

	var lists [][]retrieval.ScoredChunk

	// Lane 1: knowledge.db FTS (BM25).
	if ftsResults, err := e.backend.FTSSearch(query, candidateK); err == nil && len(ftsResults) > 0 {
		lists = append(lists, ftsResults)
	} else if err != nil {
		log.Printf("[knowledge] FTS search error: %v", err)
	}

	// Lane 2: knowledge.db vector (cosine similarity).
	if e.embedder != nil {
		vecs, err := e.embedder.Embed(ctx, []string{query})
		if err == nil && len(vecs) > 0 && len(vecs[0]) > 0 {
			if vecResults, vErr := e.backend.VectorSearch(vecs[0], candidateK); vErr == nil && len(vecResults) > 0 {
				lists = append(lists, vecResults)
			} else if vErr != nil {
				log.Printf("[knowledge] vector search error: %v", vErr)
			}
		} else if err != nil {
			log.Printf("[knowledge] embed error: %v", err)
		}
	}

	// Lane 3: user.db (FTS + Vector RRF, self-contained).
	if e.writable != nil {
		if userResults, err := e.writable.Search(ctx, query, candidateK); err == nil && len(userResults) > 0 {
			lists = append(lists, userResults)
		} else if err != nil {
			log.Printf("[knowledge] user store search error: %v", err)
		}
	}

	if len(lists) == 0 {
		return nil
	}

	fuser := retrieval.NewRRFFuser()

	// If a cross-encoder reranker is configured, fuse more candidates
	// then rerank down to topK for better precision.
	var fused []retrieval.ScoredChunk
	if e.queryReranker != nil {
		fused = fuser.Fuse(lists, candidateK)
		reranked := e.queryReranker.RerankWithQuery(ctx, query, fused)
		if len(reranked) > topK {
			reranked = reranked[:topK]
		}
		fused = reranked
	} else {
		fused = fuser.Fuse(lists, topK)
	}

	// Graph-enhanced retrieval: expand results with similar cases and
	// citation chains from the knowledge graph. The context is cached for
	// BackendRetrievalHook to inject alongside chunk results.
	e.graphMu.Lock()
	if e.graph != nil && len(fused) > 0 {
		result := e.graph.Enhance(fused)
		if ge, ok := result.(GraphEnhancement); ok {
			e.lastGraphCtx = ge.GetContext()
		}
	} else {
		e.lastGraphCtx = ""
	}
	e.graphMu.Unlock()

	return fused
}

// memorySearch uses the in-memory Store with keyword search + reranking.
func (e *KnowledgeExtension) memorySearch(query string, topK int) []retrieval.ScoredChunk {
	if e.store == nil {
		return nil
	}
	chunks := e.store.SearchableChunksForDomain(e.domain)
	if len(chunks) == 0 {
		chunks = e.store.AllChunks()
	}
	if len(chunks) == 0 {
		return nil
	}
	searcher := retrieval.NewKeywordSearcher()
	results := searcher.Search(query, chunks, topK)
	reranker := retrieval.NewPositionReranker()
	return reranker.Rerank(results)
}

func formatToolResults(results []retrieval.ScoredChunk) string {
	var b strings.Builder
	b.WriteString("搜索结果:\n")
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] (相关度: %.2f) %s\n", i+1, r.Score, r.Content)
	}
	fmt.Fprintf(&b, "\n共 %d 条结果", len(results))
	return b.String()
}

func (e *KnowledgeExtension) Layer() agentcore.ContextLayer { return agentcore.LayerKnowledge }

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

func lastUserMsg(msgs []agentcore.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agentcore.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}
