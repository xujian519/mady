package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// WikiLoader imports Obsidian wiki documents into a knowledge.Store.
// It handles directory traversal, metadata extraction, filtering, and
// card-index.json integration.
type WikiLoader struct {
	Store    *knowledge.Store
	WikiPath string // root path of the Obsidian wiki
	Filter   *WikiFilter
	Idx      *CardIndex // loaded from card-index.json (may be nil)
}

// NewWikiLoader creates a WikiLoader with default filter settings.
// card-index.json is loaded automatically if present at wikiPath/card-index.json.
func NewWikiLoader(store *knowledge.Store, wikiPath string) *WikiLoader {
	filter := DefaultWikiFilter()
	l := &WikiLoader{
		Store:    store,
		WikiPath: wikiPath,
		Filter:   filter,
	}
	// Try to load card-index.json (non-fatal if missing).
	if idx, err := LoadCardIndex(wikiPath); err == nil {
		l.Idx = idx
	}
	return l
}

// WikiImportStats holds statistics from a wiki import operation.
type WikiImportStats struct {
	TotalScanned  int            // total .md files scanned
	Imported      int            // successfully imported
	SkippedFilter int            // excluded by filter rules
	SkippedShort  int            // excluded for too-short content
	SkippedError  int            // excluded due to read/parse error
	ByType        map[string]int // imported count per DocType
	ByDomain      map[string]int // imported count per domain
	Errors        []string       // non-fatal error messages (first few)
}

// ImportWiki scans the wiki directory tree and imports all qualifying documents.
// It traverses the root directory, Wiki/, cards/, and patent-cards/ directories,
// applying the filter to exclude meta pages, index pages, fragment files, and
// short content.
//
// For XiaoNuo-style wikis the root itself contains the main content folders
// (审查指南, 专利实务, 复审无效, etc.) alongside cards/ and patent-cards/.
func (l *WikiLoader) ImportWiki() (*WikiImportStats, error) {
	if l.Store == nil {
		return nil, fmt.Errorf("wiki: store is nil")
	}
	if l.WikiPath == "" {
		return nil, fmt.Errorf("wiki: path is empty")
	}

	stats := &WikiImportStats{
		ByType:   make(map[string]int),
		ByDomain: make(map[string]int),
	}

	// Walk the root directory first, skipping card/Wiki subdirs so they can be
	// handled separately (cards may have card-index enrichment; Wiki is the
	// legacy Obsidian main-content dir walked below). Without this exclusion a
	// root-level walk would re-import Wiki/ files that the dedicated walk also
	// imports, doubling every docID in the store.
	if info, err := os.Stat(l.WikiPath); err == nil && info.IsDir() {
		if err := l.importDirectory(l.WikiPath, stats, []string{"Wiki", "cards", "patent-cards"}); err != nil {
			return stats, fmt.Errorf("wiki: walk root: %w", err)
		}
	}

	// Walk the Wiki/ directory (legacy Obsidian main content).
	wikiDir := filepath.Join(l.WikiPath, "Wiki")
	if info, err := os.Stat(wikiDir); err == nil && info.IsDir() {
		if err := l.importDirectory(wikiDir, stats, nil); err != nil {
			return stats, fmt.Errorf("wiki: walk Wiki/: %w", err)
		}
	}

	// Walk the cards/ directory (indexed cards).
	cardsDir := filepath.Join(l.WikiPath, "cards")
	if info, err := os.Stat(cardsDir); err == nil && info.IsDir() {
		if err := l.importDirectory(cardsDir, stats, nil); err != nil {
			return stats, fmt.Errorf("wiki: walk cards/: %w", err)
		}
	}

	// Walk the patent-cards/ directory (XiaoNuo indexed cards).
	patentCardsDir := filepath.Join(l.WikiPath, "patent-cards")
	if info, err := os.Stat(patentCardsDir); err == nil && info.IsDir() {
		if err := l.importDirectory(patentCardsDir, stats, nil); err != nil {
			return stats, fmt.Errorf("wiki: walk patent-cards/: %w", err)
		}
	}

	return stats, nil
}

// importDirectory walks a directory tree and imports .md files.
// excludes lists top-level subdirectory names (relative to root) to skip.
//
//nolint:gocognit // 原因：Wiki 文件导入遍历，多条件文件类型判断和处理分支
func (l *WikiLoader) importDirectory(root string, stats *WikiImportStats, excludes []string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			stats.SkippedError++
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, fmt.Sprintf("walk %s: %v", path, err))
			}
			return nil
		}
		if info.IsDir() {
			// Skip excluded top-level subdirectories to avoid double-walking
			// card directories that are handled separately.
			if path != root {
				rel, relErr := filepath.Rel(root, path)
				if relErr == nil && filepath.Dir(rel) == "." {
					for _, ex := range excludes {
						if strings.EqualFold(filepath.Base(rel), ex) {
							return filepath.SkipDir
						}
					}
				}
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		stats.TotalScanned++

		// Apply filter.
		if !l.Filter.ShouldImport(path) {
			stats.SkippedFilter++
			return nil
		}

		// Read file.
		data, err := os.ReadFile(path) //nolint:gosec // path is from filepath.Walk over wiki dir
		if err != nil {
			stats.SkippedError++
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, fmt.Sprintf("read %s: %v", path, err))
			}
			return nil
		}
		content := string(data)

		// Check minimum content length.
		if l.Filter.ContentTooShort(content) {
			stats.SkippedShort++
			return nil
		}

		// Extract metadata.
		meta := ExtractMetadata(content, path)

		// Build doc ID from relative path (unique and readable).
		relPath, _ := filepath.Rel(l.WikiPath, path)
		docID := sanitizeDocID(relPath)

		// Check card-index for quality metadata.
		searchable := l.Filter.IsSearchable(path)
		metadata := make(map[string]string)

		// Annotate excessively split fragments (3+ nested (1) markers)
		// with is_split=true and mark non-searchable to keep retrieval clean.
		if IsSplitFragment(path) {
			metadata["is_split"] = "true"
			searchable = false
		}

		if meta.Domain != "" {
			metadata["domain"] = meta.Domain
		}
		if meta.DocType != "" {
			metadata["doc_type"] = meta.DocType
		}
		if meta.Source != "" {
			metadata["source"] = meta.Source
		}
		if len(meta.LawRefs) > 0 {
			metadata["law_refs"] = strings.Join(meta.LawRefs, "; ")
		}
		if len(meta.WikiLinks) > 0 {
			metadata["wiki_links"] = strings.Join(meta.WikiLinks, "; ")
		}
		if meta.Summary != "" {
			metadata["summary"] = meta.Summary
		}
		if meta.ReviewedAt != "" {
			metadata["reviewed_at"] = meta.ReviewedAt
		}

		// Enrich from card index.
		if l.Idx != nil {
			if card := l.Idx.LookupCard(path); card != nil {
				metadata["card_concept"] = card.Concept
				metadata["card_domain"] = card.Domain
				metadata["card_quality"] = fmt.Sprintf("%.2f", card.Quality)
				if card.Quality >= 0.9 {
					metadata["featured"] = "true"
				}
			}
		}

		// Add document to store.
		domain := meta.Domain
		if domain == "" {
			domain = "patent"
		}
		title := meta.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), ".md")
		}

		if err := l.Store.AddDocument(domain, docID, title, content, path); err != nil {
			stats.SkippedError++
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, fmt.Sprintf("add %s: %v", path, err))
			}
			return nil
		}

		// Update searchable flag and metadata.
		if doc, ok := l.Store.GetDocument(docID); ok {
			doc.Searchable = searchable
			if doc.Metadata == nil {
				doc.Metadata = make(map[string]string)
			}
			for k, v := range metadata {
				doc.Metadata[k] = v
			}
		}

		stats.Imported++
		stats.ByType[meta.DocType]++
		stats.ByDomain[domain]++
		return nil
	})
}

// sanitizeDocID converts a relative file path to a valid document ID.
func sanitizeDocID(relPath string) string {
	// Remove .md extension.
	id := strings.TrimSuffix(relPath, ".md")
	// Replace path separators with forward slash.
	id = strings.ReplaceAll(id, string(filepath.Separator), "/")
	// Remove leading source-directory prefixes for cleaner IDs.
	id = strings.TrimPrefix(id, "Wiki/")
	id = strings.TrimPrefix(id, "cards/")
	id = strings.TrimPrefix(id, "patent-cards/")
	return id
}
