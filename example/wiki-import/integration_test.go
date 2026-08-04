package main

import (
	"context"
	"os"
	"testing"
	"time"

	patentwf "github.com/xujian519/mady/domains/workflows/patent"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/loader"
	"github.com/xujian519/mady/retrieval"
)

func TestIntegration_FullPipeline(t *testing.T) {
	// 该测试依赖本地 Obsidian 知识库目录（真实数据验证）。
	// 构建机/CI 上目录缺失时跳过，而不是失败——仅在有数据的环境跑真实验证。
	wikiPath := "/Users/xujian/Library/Mobile Documents/iCloud~md~obsidian/Documents/宝宸知识库"
	if _, err := os.Stat(wikiPath); err != nil {
		t.Skipf("wiki 数据目录不存在（%s），跳过集成测试", wikiPath)
	}
	// Import wiki.
	t.Log("Step 1: Importing wiki...")
	store := knowledge.NewStore()
	l := loader.NewWikiLoader(store, wikiPath)
	start := time.Now()
	stats, err := l.ImportWiki()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ImportWiki: %v", err)
	}
	t.Logf("  Imported %d docs in %v (skipped %d, short %d, errors %d)",
		stats.Imported, elapsed, stats.SkippedFilter, stats.SkippedShort, stats.SkippedError)
	if stats.Imported == 0 {
		t.Fatal("no documents imported")
	}

	// Test keyword search.
	t.Log("Step 2: Testing keyword search...")
	chunks := store.SearchableChunksForDomain("patent")
	t.Logf("  Total chunks: %d", len(chunks))
	keyword := retrieval.NewKeywordSearcher()
	results := keyword.Search(context.Background(), "全面覆盖原则 等同侵权", chunks, 5)
	if len(results) == 0 {
		t.Error("keyword search returned no results")
	} else {
		t.Logf("  Top result: %s (score %.3f)", results[0].DocID, results[0].Score)
	}

	// Run Pregel patent workflow.
	t.Log("Step 3: Running patent workflow...")
	wf, err := patentwf.BuildNoveltyGraph()
	if err != nil {
		t.Fatalf("BuildNoveltyGraph: %v", err)
	}
	state := graph.PregelState{"input": "一种基于深度学习的图像识别系统，包括图像采集模块、特征提取模块和分类模块，其特征在于所述特征提取模块使用改进的卷积神经网络结构。"}
	finalState, err := wf.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("Pregel workflow: %v", err)
	}
	if finalState.GetString("output") == "" {
		t.Error("workflow output is empty")
	} else {
		t.Logf("  Workflow output: %d chars", len(finalState.GetString("output")))
	}

	// Store statistics.
	t.Log("Step 4: Store statistics...")
	storeStats := store.Stats()
	t.Logf("  Docs: %d, Chunks: %d, Avg: %.1f/doc",
		storeStats.TotalDocs, storeStats.TotalChunks,
		float64(storeStats.TotalChunks)/float64(storeStats.TotalDocs))
	for domain, count := range storeStats.ByDomain {
		t.Logf("  %s: %d docs", domain, count)
	}
	t.Log("Full pipeline integration test passed")
}
