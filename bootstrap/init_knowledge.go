// Package bootstrap 提供所有 mady 入口（tui/serve/acp/desktop）共享的装配逻辑。
// 注意：bootstrap 是全局装配器，已知会跨层引用 domains/mcp/guardrails 等上层包。
// 这是设计上接受的"必要之恶"，不应被其他基础设施层包导入。
package bootstrap

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/knowledge/loader"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/retrieval"
)

// LoadWikiStore initializes the knowledge retrieval system.
//
//nolint:gocognit // 原因：知识库初始化，含多组件装配和条件分支
func LoadWikiStore(madyHome string) (*knowledge.Store, agentcore.LifecycleHook, agentcore.Extension, knowledge.KnowledgeBackend) { //nolint:staticcheck // legacy hook type retained for backward compat
	embedder := BuildEmbedder()
	backend, knowledgeDBPath := LoadKnowledgeBackend(madyHome)
	if backend != nil {
		ext := knowledge.NewExtension(nil, nil, "patent", knowledge.KnowledgeExtConfig{
			Enabled:    true,
			ExposeTool: true,
			RetrievalConfig: retrieval.RetrievalConfig{
				TopK:          5,
				MaxChars:      4000,
				TriggerPolicy: retrieval.TriggerSmart,
				Prefix:        "以下是从知识库中检索到的相关法条、判例和审查指南。请在回答时优先参考这些信息，并核实引用的法条编号与检索结果一致：\n",
			},
		})
		ext.WithBackend(backend, embedder)
		if reranker := BuildReranker(); reranker != nil {
			ext.WithReranker(reranker)
			slog.Info("knowledge: cross-encoder rerank enabled")
		}
		if ws := openWritableStore(madyHome, embedder, knowledgeDBPath); ws != nil {
			ext.WithWritableStore(ws)
		}

		if store, ok := backend.(*ksqlite.SQLiteStore); ok {
			dbDir := filepath.Dir(knowledgeDBPath)

			lawsPath := filepath.Join(dbDir, "laws-full-local.db")
			if _, err := os.Stat(lawsPath); os.IsNotExist(err) {
				lawsPath = filepath.Join(dbDir, "laws-full.db")
			}
			if _, err := os.Stat(lawsPath); err == nil {
				if err := store.OpenLawsDB(lawsPath); err != nil {
					slog.Error("knowledge: laws-full.db open failed", "error", err)
				} else {
					ext.WithLawSearcher(func(keyword string, topK int) ([]knowledge.LawRecord, error) {
						sqliteResults, err := store.SearchLaws(keyword, topK)
						if err != nil {
							return nil, err
						}
						out := make([]knowledge.LawRecord, len(sqliteResults))
						for i, r := range sqliteResults {
							out[i] = knowledge.LawRecord{
								ID: r.ID, Level: r.Level, Name: r.Name,
								Subtitle: r.Subtitle, Content: r.Content, Category: r.Category,
							}
						}
						return out, nil
					})
					mode := "FTS5"
					if !store.HasLawFTS() {
						mode = "LIKE"
					}
					lawsLabel := filepath.Base(lawsPath)
					slog.Info("knowledge: laws-full.db active", "file", lawsLabel, "mode", mode)
				}
			}

			if gs, err := store.LoadGraph(); err != nil {
				slog.Error("knowledge: graph load failed", "error", err)
			} else if gs.NodeCount() > 0 {
				enhancer := kgwgraph.NewGraphEnhancer(gs, kgwgraph.DefaultEnhanceConfig())
				ext.WithGraph(enhancer)
				typeCounts := gs.NodeTypeCounts()
				lawCount := typeCounts[kgwgraph.NodeLawArticle]
				caseCount := typeCounts[kgwgraph.NodeCase] + typeCounts[kgwgraph.NodeJudgment]
				ipcCount := typeCounts[kgwgraph.NodeIPC]
				evidenceCount := typeCounts[kgwgraph.NodeEvidence]
				slog.Info("knowledge: 图谱已加载",
					"nodes", gs.NodeCount(), "edges", gs.EdgeCount(),
					"law", lawCount, "case", caseCount, "ipc", ipcCount, "evidence", evidenceCount)
			}
		}

		hook := ext.LifecycleHook()
		if hook != nil {
			return nil, hook, ext, backend
		}
	}

	wikiPath := os.Getenv("WIKI_PATH")
	if wikiPath == "" {
		return nil, nil, nil, nil
	}
	store := knowledge.NewStore()
	wikiLoader := loader.NewWikiLoader(store, wikiPath)
	stats, err := wikiLoader.ImportWiki()
	if err != nil {
		slog.Error("wiki import failed", "error", err)
		return nil, nil, nil, nil
	}
	slog.Info("wiki imported", "docs", stats.Imported, "chunks", store.Stats().TotalChunks)
	hook := store.RetrievalHook("patent", retrieval.RetrievalConfig{
		TopK:          5,
		MaxChars:      4000,
		TriggerPolicy: retrieval.TriggerSmart,
		Prefix:        "以下是从知识库中检索到的相关法条、判例和审查指南。请在回答时优先参考这些信息，并核实引用的法条编号与检索结果一致：\n",
	})
	return store, hook, nil, nil
}

// ResolveWikiRoot resolves the Obsidian wiki root for patent-cards access.
func ResolveWikiRoot(madyHome string) string {
	if p := os.Getenv("WIKI_PATH"); p != "" {
		p = filepath.Clean(p)
		if info, err := os.Stat(p); err == nil && info.IsDir() { // #nosec G703 -- path cleaned above
			return p
		}
	}
	if madyHome == "" {
		return ""
	}
	candidate := filepath.Join(madyHome, "knowledge", "wiki")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() { // #nosec G703 -- joined from trusted MadyHome
		return candidate
	}
	return ""
}

// BuildEmbedder creates an APIEmbedder from environment variables.
func BuildEmbedder() retrieval.Embedder {
	baseURL := os.Getenv("OMLX_BASE_URL")
	if baseURL == "" {
		baseURL = agentconfig.DefaultOMLXBaseURL
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return nil
	}
	model := os.Getenv("OMLX_EMBED_MODEL")
	if model == "" {
		model = agentconfig.DefaultEmbedModel
	}
	return retrieval.NewAPIEmbedder(baseURL, apiKey, model)
}

// BuildReranker creates a ModelReranker from environment variables.
func BuildReranker() retrieval.QueryReranker {
	flag := strings.ToLower(os.Getenv("KNOWLEDGE_RERANK"))
	if flag != "on" && flag != "true" && flag != "1" {
		return nil
	}
	baseURL := os.Getenv("OMLX_BASE_URL")
	if baseURL == "" {
		baseURL = agentconfig.DefaultOMLXBaseURL
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return nil
	}
	model := os.Getenv("OMLX_RERANK_MODEL")
	if model == "" {
		model = agentconfig.DefaultRerankModel
	}
	return retrieval.NewModelReranker(baseURL, apiKey, model)
}

// LoadKnowledgeBackend opens the SQLite knowledge database read-only.
func LoadKnowledgeBackend(madyHome string) (knowledge.KnowledgeBackend, string) {
	dbDir := os.Getenv("KNOWLEDGE_DB_DIR")
	if dbDir == "" {
		if madyHome != "" {
			dbDir = filepath.Join(madyHome, "knowledge")
		} else {
			return nil, ""
		}
	}
	dbPath := filepath.Join(dbDir, "knowledge.db")
	if _, err := os.Stat(dbPath); err != nil { // #nosec G703 -- dbDir resolved from MadyHome or cleaned env
		return nil, ""
	}
	store, err := ksqlite.NewSQLiteStore(dbPath)
	if err != nil {
		slog.Error("knowledge: failed to open SQLite store", "error", err)
		return nil, ""
	}
	if err := store.PreloadVectors(); err != nil {
		slog.Warn("knowledge: vector preload failed, using SQL batch fallback", "error", err)
	} else {
		stats := store.Stats()
		slog.Info("knowledge: SQLite backend active", "path", dbPath)
		slog.Info("knowledge: stats",
			"docs", stats.Documents, "chunks", stats.Chunks,
			"embeddings", stats.Embeddings, "dims", stats.Dim, "vector_mb", stats.VectorMemoryMB)
	}
	return store, dbPath
}

// openWritableStore opens or creates the user database (user.db).
