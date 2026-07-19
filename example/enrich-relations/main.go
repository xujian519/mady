package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type cardIndex struct {
	TotalCards   int                 `json:"total_cards"`
	LastUpdated  string              `json:"last_updated"`
	Cards        []cardEntry         `json:"cards"`
	ConceptIndex map[string][]string `json:"concept_index,omitempty"`
	DomainIndex  map[string][]string `json:"domain_index,omitempty"`
}

type cardEntry struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Concept         string   `json:"concept"`
	Quality         float64  `json:"quality"`
	Domain          string   `json:"domain"`
	FilePath        string   `json:"file_path"`
	RelatedConcepts []string `json:"related_concepts"`
	GeneratedAt     string   `json:"generated_at"`
	Version         int      `json:"version"`
}

// enrichRelations fills empty related_concepts for all cards.
// Strategy 1: same-concept cards become mutual relations.
// Strategy 2: same-domain, different-concept cards → up to 3 cross-references.
func enrichRelations(idx *cardIndex) int {
	conceptToIDs := make(map[string][]int)
	for i, c := range idx.Cards {
		if c.Concept != "" {
			conceptToIDs[c.Concept] = append(conceptToIDs[c.Concept], i)
		}
	}

	domainToIDs := make(map[string][]int)
	for i, c := range idx.Cards {
		if c.Domain != "" {
			domainToIDs[c.Domain] = append(domainToIDs[c.Domain], i)
		}
	}

	var filled int
	for i := range idx.Cards {
		if len(idx.Cards[i].RelatedConcepts) > 0 {
			continue
		}

		var relations []string
		seen := make(map[string]bool)

		if idx.Cards[i].Concept != "" {
			for _, j := range conceptToIDs[idx.Cards[i].Concept] {
				if i == j {
					continue
				}
				if !seen[idx.Cards[j].ID] {
					seen[idx.Cards[j].ID] = true
					relations = append(relations, idx.Cards[j].ID)
				}
			}
		}

		if idx.Cards[i].Domain != "" {
			var candidates []int
			for _, j := range domainToIDs[idx.Cards[i].Domain] {
				if i == j {
					continue
				}
				if idx.Cards[i].Concept != "" && idx.Cards[j].Concept == idx.Cards[i].Concept {
					continue
				}
				if !seen[idx.Cards[j].ID] {
					seen[idx.Cards[j].ID] = true
					candidates = append(candidates, j)
				}
			}
			limit := 3
			for _, j := range candidates {
				if limit <= 0 {
					break
				}
				relations = append(relations, idx.Cards[j].ID)
				limit--
			}
		}

		idx.Cards[i].RelatedConcepts = relations
		if len(relations) > 0 {
			filled++
		}
	}
	return filled
}

func rebuildIndexes(idx *cardIndex) {
	idx.ConceptIndex = make(map[string][]string)
	idx.DomainIndex = make(map[string][]string)
	for _, c := range idx.Cards {
		if c.Concept != "" {
			idx.ConceptIndex[c.Concept] = append(idx.ConceptIndex[c.Concept], c.ID)
		}
		if c.Domain != "" {
			idx.DomainIndex[c.Domain] = append(idx.DomainIndex[c.Domain], c.ID)
		}
	}
}

func main() {
	wikiPath := "/Users/xujian/Library/Mobile Documents/iCloud~md~obsidian/Documents/宝宸知识库"
	if len(os.Args) > 1 {
		wikiPath = os.Args[1]
	}

	indexPath := filepath.Join(wikiPath, "card-index.json")
	fmt.Printf("📂 卡片索引: %s\n\n", indexPath)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 读取失败: %v\n", err)
		os.Exit(1)
	}

	var idx cardIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 解析失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("📊 读取卡片: %d 张\n", idx.TotalCards)

	alreadyFilled := 0
	for _, c := range idx.Cards {
		if len(c.RelatedConcepts) > 0 {
			alreadyFilled++
		}
	}
	fmt.Printf("   已存关联: %d 张\n", alreadyFilled)

	backupPath := filepath.Join(wikiPath,
		fmt.Sprintf("card-index.json.bak.%s", time.Now().Format("20060102-150405")))
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  备份失败: %v\n", err)
	} else {
		fmt.Printf("💾 备份: %s\n", backupPath)
	}

	filled := enrichRelations(&idx)
	rebuildIndexes(&idx)
	idx.LastUpdated = time.Now().Format(time.RFC3339)

	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(indexPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 写入失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ 补全完成\n")
	fmt.Printf("   新填充:    %d 张\n", filled)

	maxRel := 0
	emptyAfter := 0
	for _, c := range idx.Cards {
		if len(c.RelatedConcepts) > maxRel {
			maxRel = len(c.RelatedConcepts)
		}
		if len(c.RelatedConcepts) == 0 {
			emptyAfter++
		}
	}
	fmt.Printf("   最大关联数: %d\n", maxRel)
	fmt.Printf("   概念覆盖:  %d 个\n", len(idx.ConceptIndex))
	fmt.Printf("   领域覆盖:  %d 个\n", len(idx.DomainIndex))

	fmt.Print("\n📌 热门概念（按卡片数）:\n")
	type cv struct {
		name  string
		count int
	}
	var concepts []cv
	for c, ids := range idx.ConceptIndex {
		concepts = append(concepts, cv{c, len(ids)})
	}
	for i := 0; i < len(concepts); i++ {
		for j := i + 1; j < len(concepts); j++ {
			if concepts[j].count > concepts[i].count {
				concepts[i], concepts[j] = concepts[j], concepts[i]
			}
		}
	}
	n := 10
	if len(concepts) < n {
		n = len(concepts)
	}
	for _, cv := range concepts[:n] {
		fmt.Printf("   %-20s %d 张\n", cv.name, cv.count)
	}
}
