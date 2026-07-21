package main

// 本文件负责知识库装配：SQLite 知识后端（向量 + FTS RRF 融合）、
// laws-full.db 法规全文库、知识图谱增强器、用户可写库（user.db），
// 以及 WIKI_PATH 内存库回退与 embedder/reranker 的环境变量构建。
// 以上能力由 setupFrameworkContext（framework.go）统一调用。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/knowledge/loader"
	"github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/retrieval"
)

// loadWikiStore initializes the knowledge retrieval system.
// It tries the SQLite backend first (vector + FTS RRF fusion via
// KNOWLEDGE_DB_DIR), falling back to the in-memory wiki store
// (WIKI_PATH) for backward compatibility.
// Returns the in-memory store (nil when using SQLite), a retrieval hook,
// the knowledge extension, and the raw backend interface (nil when using
// the in-memory store or when no backend is configured). The backend is
// exposed so other subsystems (e.g. reasoning Stage ② rule retrieval) can
// reuse the already-opened SQLite FTS index without reopening the database.
func loadWikiStore(madyHome string) (*knowledge.Store, agentcore.LifecycleHook, agentcore.Extension, knowledge.KnowledgeBackend) {
	// 1. Try SQLite backend (vector + FTS RRF fusion).
	embedder := buildEmbedder()
	backend, knowledgeDBPath := loadKnowledgeBackend(madyHome)
	if backend != nil {
		ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
		ext.WithBackend(backend, embedder)
		if reranker := buildReranker(); reranker != nil {
			ext.WithReranker(reranker)
			fmt.Fprintf(os.Stderr, "knowledge: cross-encoder rerank enabled\n")
		}
		if ws := openWritableStore(madyHome, embedder, knowledgeDBPath); ws != nil {
			ext.WithWritableStore(ws)
		}

		// Wire laws-full.db + knowledge graph enhancer (same directory as knowledge.db).
		if store, ok := backend.(*sqlite.SQLiteStore); ok {
			dbDir := filepath.Dir(knowledgeDBPath)

			// Open laws-full.db for law full-text search.
			// Try laws-full-local.db first (has FTS5 index), fall back to
			// laws-full.db (original, LIKE-only search).
			lawsPath := filepath.Join(dbDir, "laws-full-local.db")
			if _, err := os.Stat(lawsPath); os.IsNotExist(err) {
				lawsPath = filepath.Join(dbDir, "laws-full.db")
			}
			if _, err := os.Stat(lawsPath); err == nil {
				if err := store.OpenLawsDB(lawsPath); err != nil {
					fmt.Fprintf(os.Stderr, "knowledge: laws-full.db open failed: %v\n", err)
				} else {
					// Wrap SearchLaws as knowledge.LawSearcher function type.
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
					fmt.Fprintf(os.Stderr, "knowledge: %s active (%s search)\n", lawsLabel, mode)
				}
			}

			// Load knowledge graph and wire graph enhancer.
			if gs, err := store.LoadGraph(); err != nil {
				fmt.Fprintf(os.Stderr, "knowledge: graph load failed: %v\n", err)
			} else if gs.NodeCount() > 0 {
				enhancer := kgwgraph.NewGraphEnhancer(gs, kgwgraph.DefaultEnhanceConfig())
				ext.WithGraph(enhancer)
				// Compute node type breakdown for diagnostics.
				typeCounts := gs.NodeTypeCounts()
				lawCount := typeCounts[kgwgraph.NodeLawArticle]
				caseCount := typeCounts[kgwgraph.NodeCase] + typeCounts[kgwgraph.NodeJudgment]
				ipcCount := typeCounts[kgwgraph.NodeIPC]
				evidenceCount := typeCounts[kgwgraph.NodeEvidence]
				fmt.Fprintf(os.Stderr, "knowledge:   图谱 %d 节点 / %d 边 (法律%d, 案例%d, IPC%d, 证据%d)\n",
					gs.NodeCount(), gs.EdgeCount(), lawCount, caseCount, ipcCount, evidenceCount)
			}
		}

		hook := ext.BackendHook(retrieval.RetrievalConfig{
			TopK:          5,
			MaxChars:      4000,
			TriggerPolicy: retrieval.TriggerSmart,
			Prefix:        "以下是从知识库中检索到的相关法条、判例和审查指南。请在回答时优先参考这些信息，并核实引用的法条编号与检索结果一致：\n",
		})
		if hook != nil {
			return nil, hook, ext, backend
		}
	}

	// 2. Fallback: in-memory wiki store (WIKI_PATH).
	wikiPath := os.Getenv("WIKI_PATH")
	if wikiPath == "" {
		return nil, nil, nil, nil
	}
	store := knowledge.NewStore()
	wikiLoader := loader.NewWikiLoader(store, wikiPath)
	stats, err := wikiLoader.ImportWiki()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wiki: import failed: %v\n", err)
		return nil, nil, nil, nil
	}
	fmt.Fprintf(os.Stderr, "wiki: imported %d docs, %d chunks\n",
		stats.Imported, store.Stats().TotalChunks)
	hook := store.RetrievalHook("patent", retrieval.RetrievalConfig{
		TopK:          5,
		MaxChars:      4000,
		TriggerPolicy: retrieval.TriggerSmart,
		Prefix:        "以下是从知识库中检索到的相关法条、判例和审查指南。请在回答时优先参考这些信息，并核实引用的法条编号与检索结果一致：\n",
	})
	return store, hook, nil, nil
}

// resolveWikiRoot resolves the Obsidian wiki root for patent-cards access.
// Priority: $WIKI_PATH > $MADY_HOME/knowledge/wiki. Returns "" when neither
// resolves to an existing directory, so callers can leave the skill lane
// disabled without pointing at a non-existent path.
func resolveWikiRoot(madyHome string) string {
	if p := os.Getenv("WIKI_PATH"); p != "" {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	if madyHome == "" {
		return ""
	}
	candidate := filepath.Join(madyHome, "knowledge", "wiki")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}

// buildEmbedder creates an APIEmbedder from environment variables.
// Returns nil if OMLX_API_KEY is not set (vector search disabled, FTS-only).
func buildEmbedder() retrieval.Embedder {
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

// buildReranker creates a ModelReranker from environment variables.
// Returns nil if KNOWLEDGE_RERANK is not "on"/"true"/"1" or if
// OMLX_API_KEY is not set (reranker requires the same auth as embedder).
func buildReranker() retrieval.QueryReranker {
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

// loadKnowledgeBackend opens the SQLite knowledge database read-only.
// Returns nil if the database file is not found or cannot be opened.
// The second return value is the resolved knowledge.db path (empty when nil).
func loadKnowledgeBackend(madyHome string) (knowledge.KnowledgeBackend, string) {
	dbDir := os.Getenv("KNOWLEDGE_DB_DIR")
	if dbDir == "" {
		if madyHome != "" {
			dbDir = filepath.Join(madyHome, "knowledge")
		} else {
			return nil, ""
		}
	}
	dbPath := filepath.Join(dbDir, "knowledge.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, ""
	}
	store, err := sqlite.NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "knowledge: failed to open SQLite store: %v\n", err)
		return nil, ""
	}
	if err := store.PreloadVectors(); err != nil {
		fmt.Fprintf(os.Stderr, "knowledge: vector preload failed, using SQL batch fallback: %v\n", err)
	} else {
		stats := store.Stats()
		fmt.Fprintf(os.Stderr, "knowledge: SQLite backend active (%s)\n", dbPath)
		fmt.Fprintf(os.Stderr, "knowledge:   文档 %d | 分块 %d | 向量 %d (%d维, %.0f MB)\n",
			stats.Documents, stats.Chunks, stats.Embeddings, stats.Dim, stats.VectorMemoryMB)
	}
	return store, dbPath
}

// openWritableStore opens or creates the user database (user.db) for
// user-added documents. Returns nil if the embedder is not configured
// (vector search disabled), or if opening fails (non-fatal — the system
// continues without user document support).
//
// The knowledgeDBPath is passed to OpenWritable for path-conflict
// detection: user.db must not point to the same file as knowledge.db.
func openWritableStore(madyHome string, embedder retrieval.Embedder, knowledgeDBPath string) *sqlite.WritableStore {
	if embedder == nil {
		return nil // writable store requires an embedder for vectorisation
	}
	userDBPath := os.Getenv("USER_DB_PATH")
	if userDBPath == "" {
		if madyHome == "" {
			return nil
		}
		userDBPath = filepath.Join(madyHome, "knowledge", "user.db")
	}
	// Ensure parent directory exists.
	if dir := filepath.Dir(userDBPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "knowledge: user.db dir create failed: %v\n", err)
			return nil
		}
	}
	ws, err := sqlite.OpenWritable(userDBPath, embedder, knowledgeDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "knowledge: user.db open failed: %v\n", err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "knowledge: user.db writable store active (%s)\n", userDBPath)
	return ws
}
