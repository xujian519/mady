package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

const ExtensionName = "knowledge"

type GraphEnhancer interface {
	Enhance(seeds []retrieval.ScoredChunk) interface{}
}

type KnowledgeExtension struct {
	agentcore.BaseLifecycleHook
	store  *Store
	graph  GraphEnhancer
	hook   *retrieval.RetrievalHook
	domain string
	cfg    KnowledgeExtConfig
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
	return &KnowledgeExtension{
		store:  store,
		graph:  g,
		hook:   retrieval.NewRetrievalHook(chunks, cfg.RetrievalConfig),
		domain: domain,
		cfg:    cfg,
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

func (e *KnowledgeExtension) TransformContext(_ context.Context, msgs []agentcore.Message) []agentcore.Message {
	return msgs
}

func (e *KnowledgeExtension) Tools() []*agentcore.Tool {
	if !e.cfg.ExposeTool {
		return nil
	}
	return []*agentcore.Tool{
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

	var results []retrieval.ScoredChunk
	if e.store != nil {
		chunks := e.store.SearchableChunksForDomain(e.domain)
		if len(chunks) == 0 {
			chunks = e.store.AllChunks()
		}
		if len(chunks) > 0 {
			searcher := retrieval.NewKeywordSearcher()
			results = searcher.Search(p.Query, chunks, p.TopK)
			reranker := retrieval.NewPositionReranker()
			results = reranker.Rerank(results)
		}
	}
	if len(results) == 0 {
		return "未找到相关文档", nil
	}
	return formatToolResults(results), nil
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

func (e *KnowledgeExtension) Provide(_ context.Context, input agentcore.BuildInput, _ agentcore.LayerConfig) ([]agentcore.Message, error) {
	if !e.cfg.Enabled || e.store == nil {
		return nil, nil
	}
	query := lastUserMsg(input.Messages)
	if query == "" {
		return nil, nil
	}
	chunks := e.store.SearchableChunksForDomain(e.domain)
	if len(chunks) == 0 {
		chunks = e.store.AllChunks()
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	searcher := retrieval.NewKeywordSearcher()
	results := searcher.Search(query, chunks, 5)
	if len(results) == 0 {
		return nil, nil
	}
	reranker := retrieval.NewPositionReranker()
	results = reranker.Rerank(results)

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
