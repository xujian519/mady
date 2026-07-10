// Citation tracker — provenance tracking and rendering for retrieved items.
//
// Ported from @nuo/knowledge citation-tracker.ts. Provides structured citation
// metadata, grouped citation chains, and conflict detection across sources of
// varying authority. Bridges the retrieval Chunk type to the domain-agnostic
// CitableItem so that patent/legal/general documents share one citation format.
//
// Usage:
//
//	items := retrieval.ScoredChunksToCitable(results)
//	fmt.Println(retrieval.FormatCitations(items))
//	fmt.Println(retrieval.FormatCitationChain(items))
package retrieval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CitableItem is a domain-agnostic document fragment eligible for citation.
type CitableItem struct {
	Title           string
	Heading         string // section/heading within the document ("" if none)
	Source          string // source database label (e.g., "law", "case", "patent")
	DocType         string // document type label (e.g., "法条", "案例", "专利")
	Domain          string // domain hint (e.g., "patent", "legal")
	AuthorityWeight float64 // 0-1 authority weight; higher = more authoritative
	Metadata        map[string]string // extra tags (e.g., caseNumber, ipc, article)
}

// CitationMeta is the extracted citation metadata for a single item.
type CitationMeta struct {
	SourceTitle string // "标题" or "标题 > 章节"
	SourcePath  string // disambiguated path (with case number if available)
	SourceDB    string // source database
	ItemType    string // document type label
	Score       float64
}

// ExtractCitation builds citation metadata from a citable item.
func ExtractCitation(item CitableItem) CitationMeta {
	sourceTitle := item.Title
	if item.Heading != "" {
		sourceTitle = item.Title + " > " + item.Heading
	}
	sourcePath := item.Title
	if cn, ok := item.Metadata["caseNumber"]; ok && cn != "" {
		sourcePath = fmt.Sprintf("%s (%s)", item.Title, cn)
	}
	return CitationMeta{
		SourceTitle: sourceTitle,
		SourcePath:  sourcePath,
		SourceDB:    item.Source,
		ItemType:    item.DocType,
		Score:       item.AuthorityWeight,
	}
}

// CitationPrefix returns a concise "based on the following sources" prefix.
func CitationPrefix(items []CitableItem) string {
	if len(items) == 0 {
		return ""
	}
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = ExtractCitation(item).SourceTitle
	}
	return "基于以下知识来源：" + strings.Join(titles, " | ")
}

// FormatCitations renders a flat "参考来源" list with authority weights.
func FormatCitations(items []CitableItem) string {
	if len(items) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, "---", "**参考来源：**", "")
	for _, item := range items {
		c := ExtractCitation(item)
		authority := ""
		if c.Score > 0 {
			authority = fmt.Sprintf(" [权威度: %.1f]", c.Score)
		}
		lines = append(lines, fmt.Sprintf("- %s — %s/%s%s", c.SourceTitle, c.SourceDB, c.ItemType, authority))
	}
	return strings.Join(lines, "\n")
}

// citationTypeOrder defines the canonical ordering of source types in chains.
var citationTypeOrder = []string{"law", "guideline", "case", "judgment", "patent", "note"}

// citationTypeLabels maps source types to Chinese labels.
var citationTypeLabels = map[string]string{
	"law":       "法律依据",
	"guideline": "审查指南",
	"case":      "案例",
	"judgment":  "判决",
	"patent":    "专利文献",
	"note":      "笔记",
}

// FormatCitationChain renders citations grouped by source type, ordered by
// authority hierarchy (law → guideline → case → judgment → patent → note).
func FormatCitationChain(items []CitableItem) string {
	if len(items) == 0 {
		return ""
	}

	grouped := make(map[string][]CitableItem)
	for _, item := range items {
		grouped[item.Source] = append(grouped[item.Source], item)
	}

	var chains []string
	seen := make(map[string]bool)

	for _, t := range citationTypeOrder {
		group, ok := grouped[t]
		if !ok || len(group) == 0 {
			continue
		}
		label := citationTypeLabels[t]
		if label == "" {
			label = t
		}
		chains = append(chains, formatChainLine(label, items, group))
		seen[t] = true
	}

	// Append any remaining types not in the canonical order.
	var extra []string
	for t := range grouped {
		if !seen[t] {
			extra = append(extra, t)
		}
	}
	sort.Strings(extra)
	for _, t := range extra {
		chains = append(chains, formatChainLine(t, items, grouped[t]))
	}

	return strings.Join(chains, "\n")
}

func formatChainLine(label string, all []CitableItem, group []CitableItem) string {
	var parts []string
	for _, g := range group {
		globalIdx := indexOfItem(all, g) + 1
		parts = append(parts, fmt.Sprintf("[%d] %s", globalIdx, g.Title))
	}
	return label + ": " + strings.Join(parts, " → ")
}

func indexOfItem(items []CitableItem, target CitableItem) int {
	for i, item := range items {
		if item.Title == target.Title && item.Source == target.Source {
			return i
		}
	}
	return -1
}

// DetectConflicts scans items for the same keyword appearing in sources of
// significantly different authority levels (delta > 0.3), which may indicate
// conflicting interpretations.
func DetectConflicts(items []CitableItem) []string {
	if len(items) < 2 {
		return nil
	}

	var warnings []string
	titleKeywords := make(map[string][]CitableItem)

	for _, item := range items {
		keywords := splitKeywords(item.Title)
		for _, kw := range keywords {
			titleKeywords[kw] = append(titleKeywords[kw], item)
		}
	}

	for kw, related := range titleKeywords {
		if len(related) < 2 {
			continue
		}
		minAuth, maxAuth := related[0].AuthorityWeight, related[0].AuthorityWeight
		for _, r := range related[1:] {
			if r.AuthorityWeight < minAuth {
				minAuth = r.AuthorityWeight
			}
			if r.AuthorityWeight > maxAuth {
				maxAuth = r.AuthorityWeight
			}
		}
		if maxAuth-minAuth > 0.3 {
			typeSet := make(map[string]bool)
			for _, r := range related {
				typeSet[r.DocType] = true
			}
			var types []string
			for t := range typeSet {
				types = append(types, t)
			}
			sort.Strings(types)
			warnings = append(warnings, fmt.Sprintf(
				"注意：关键词「%s」存在不同层级的解释 (%s)", kw, strings.Join(types, "/")))
		}
	}

	sort.Strings(warnings)
	return warnings
}

// splitKeywords extracts significant keywords (length >= 2 runes) from a title
// by splitting on common delimiters.
func splitKeywords(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	// Split on whitespace, commas (full/half width), and enumeration marks.
	replacer := strings.NewReplacer(" ", "|", ",", "|", "，", "|", "、", "|", "。", "|")
	parts := strings.Split(replacer.Replace(title), "|")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len([]rune(p)) >= 2 {
			result = append(result, p)
		}
	}
	return result
}

// ChunkToCitable converts a retrieval Chunk into a CitableItem.
// The score is the retrieval relevance score; the authority weight is derived
// from the chunk's "authority" metadata if present, else defaults to the score.
func ChunkToCitable(chunk Chunk, score float64) CitableItem {
	item := CitableItem{
		Title:    chunk.DocID,
		Source:   chunk.Metadata["source"],
		DocType:  chunk.Metadata["type"],
		Domain:   chunk.Metadata["domain"],
		Metadata: chunk.Metadata,
	}
	if heading, ok := chunk.Metadata["section"]; ok {
		item.Heading = heading
	}
	if claim, ok := chunk.Metadata["claim"]; ok && item.Heading == "" {
		item.Heading = claim
	}
	item.AuthorityWeight = authorityFromMetadata(chunk.Metadata, score)
	return item
}

// ScoredChunksToCitable converts a slice of ScoredChunk results into citable items.
func ScoredChunksToCitable(results []ScoredChunk) []CitableItem {
	items := make([]CitableItem, 0, len(results))
	for _, r := range results {
		items = append(items, ChunkToCitable(r.Chunk, r.Score))
	}
	return items
}

func authorityFromMetadata(meta map[string]string, fallback float64) float64 {
	if v, ok := meta["authority"]; ok && v != "" {
		w, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return w
		}
	}
	return fallback
}
