// Command wiki-import imports Obsidian wiki documents into Mady's knowledge store
// and reports detailed statistics. This serves as the integration test for the
// WikiLoader against real data.
//
// Usage:
//
//	go run ./example/wiki-import/ /path/to/宝宸知识库
package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/loader"
)

func main() {
	wikiPath := "/Users/xujian/Library/Mobile Documents/iCloud~md~obsidian/Documents/宝宸知识库"
	if len(os.Args) > 1 {
		wikiPath = os.Args[1]
	}

	fmt.Printf("Wiki 导入器 - 真实数据验证\n")
	fmt.Printf("数据源: %s\n\n", wikiPath)

	store := knowledge.NewStore()
	l := loader.NewWikiLoader(store, wikiPath)

	start := time.Now()
	stats, err := l.ImportWiki()
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "导入错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("⏱  耗时: %v\n\n", elapsed)
	fmt.Printf("📊 导入统计:\n")
	fmt.Printf("  扫描文件:     %d\n", stats.TotalScanned)
	fmt.Printf("  成功导入:     %d\n", stats.Imported)
	fmt.Printf("  过滤(元数据): %d\n", stats.SkippedFilter)
	fmt.Printf("  过滤(太短):   %d\n", stats.SkippedShort)
	fmt.Printf("  错误:         %d\n", stats.SkippedError)

	if len(stats.Errors) > 0 {
		fmt.Printf("\n⚠️  错误详情 (前 10 条):\n")
		for _, e := range stats.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	fmt.Printf("\n📂 按文档类型:\n")
	printSortedMap(stats.ByType)

	fmt.Printf("\n🌐 按领域:\n")
	printSortedMap(stats.ByDomain)

	// Store statistics
	storeStats := store.Stats()
	fmt.Printf("\n💾 存储状态:\n")
	fmt.Printf("  总文档:   %d\n", storeStats.TotalDocs)
	fmt.Printf("  总分块:   %d\n", storeStats.TotalChunks)
	if storeStats.TotalChunks > 0 && storeStats.TotalDocs > 0 {
		fmt.Printf("  平均分块: %.1f/篇\n", float64(storeStats.TotalChunks)/float64(storeStats.TotalDocs))
	}
	for domain, count := range storeStats.ByDomain {
		fmt.Printf("  %s: %d 篇\n", domain, count)
	}

	// Verify searchable vs non-searchable
	searchable := 0
	nonSearchable := 0
	for _, docID := range store.AllDocIDs() {
		if doc, ok := store.GetDocument(docID); ok {
			if doc.Searchable {
				searchable++
			} else {
				nonSearchable++
			}
		}
	}
	fmt.Printf("\n🔍 可检索: %d 篇, 不可检索: %d 篇\n", searchable, nonSearchable)
}

func printSortedMap(m map[string]int) {
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	for _, p := range pairs {
		fmt.Printf("  %-15s %d 篇\n", p.k+":", p.v)
	}
}

// allDocIDs is a helper that returns all document IDs from a store.
// In production, this would be a store method, but for now we iterate domains.
