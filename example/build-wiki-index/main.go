package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	reH1       = regexp.MustCompile(`^#\s+(.+)`)
	reWikiLink = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
)

type wikiConceptIndex struct {
	TotalConcepts int                 `json:"total_concepts"`
	TotalFiles    int                 `json:"total_files"`
	LastUpdated   string              `json:"last_updated"`
	ConceptIndex  map[string][]string `json:"concept_index"`
	FileConcepts  map[string][]string `json:"file_concepts,omitempty"`
}

// extractConceptsFromFile returns the H1 title and all [[wikilinks]] from a markdown file.
func extractConceptsFromFile(content, filePath string) (title string, wikilinks []string) {
	if match := reH1.FindStringSubmatch(content); len(match) >= 2 {
		title = strings.TrimSpace(match[1])
	}

	seen := make(map[string]bool)
	allLinks := reWikiLink.FindAllStringSubmatch(content, -1)
	for _, match := range allLinks {
		link := strings.TrimSpace(match[1])
		if idx := strings.Index(link, "|"); idx >= 0 {
			link = link[:idx]
		}
		if !seen[link] {
			seen[link] = true
			wikilinks = append(wikilinks, link)
		}
	}
	return title, wikilinks
}

func main() {
	wikiPath := "/Users/xujian/Library/Mobile Documents/iCloud~md~obsidian/Documents/宝宸知识库"
	if len(os.Args) > 1 {
		wikiPath = os.Args[1]
	}

	fmt.Printf("📂 Wiki 路径: %s\n\n", wikiPath)

	index := wikiConceptIndex{
		ConceptIndex: make(map[string][]string),
		FileConcepts: make(map[string][]string),
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	// Walk both Wiki/ and cards/ directories.
	var totalFiles int
	for _, subDir := range []string{"Wiki", "cards"} {
		root := filepath.Join(wikiPath, subDir)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			fmt.Printf("⚠️  跳过: %s (不存在)\n", root)
			continue
		}

		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
				return nil
			}

			relPath, _ := filepath.Rel(wikiPath, path)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			title, wikilinks := extractConceptsFromFile(string(data), path)
			totalFiles++
			index.TotalFiles = totalFiles

			if title != "" {
				index.FileConcepts[relPath] = append(index.FileConcepts[relPath], title)
			} else {
				index.FileConcepts[relPath] = append(index.FileConcepts[relPath],
					strings.TrimSuffix(info.Name(), ".md"))
			}

			for _, link := range wikilinks {
				link = strings.TrimSpace(link)
				if link == "" {
					continue
				}
				seen := false
				for _, existing := range index.ConceptIndex[link] {
					if existing == relPath {
						seen = true
						break
					}
				}
				if !seen {
					index.ConceptIndex[link] = append(index.ConceptIndex[link], relPath)
				}
			}
			return nil
		})
	}

	index.TotalConcepts = len(index.ConceptIndex)

	// Write output.
	outputPath := filepath.Join(wikiPath, "wiki-concept-index.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 写入失败: %v\n", err)
		os.Exit(1)
	}

	// Statistics.
	fmt.Printf("📊 扫描统计:\n")
	fmt.Printf("  扫描文件:     %d\n", totalFiles)
	fmt.Printf("  独特概念:     %d\n", index.TotalConcepts)
	fmt.Printf("  总概念引用:   %d\n", countEntries(index.ConceptIndex))

	// Top referenced concepts.
	type kv struct {
		concept string
		count   int
	}
	var sorted []kv
	for c, paths := range index.ConceptIndex {
		sorted = append(sorted, kv{c, len(paths)})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

	fmt.Print("\n📌 最高引用概念（Top 15）:\n")
	limit := 15
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for _, kv := range sorted[:limit] {
		fmt.Printf("   %-25s %d 个引用\n", kv.concept, kv.count)
	}

	fmt.Printf("\n💾 写入: %s (%.1f KB)\n", outputPath, float64(len(data))/1024)
}

func countEntries(m map[string][]string) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}
