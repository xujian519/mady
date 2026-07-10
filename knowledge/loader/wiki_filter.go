package loader

import (
	"path/filepath"
	"strings"
)

// WikiFilter decides which wiki files to import and whether they are searchable.
// Directory/index pages, log files, and tiny fragment files are excluded or
// marked non-searchable to keep retrieval results clean.
type WikiFilter struct {
	// ExcludeNames are exact file names to skip (e.g. "index.md", "log.md").
	ExcludeNames []string

	// ExcludePatterns are substring patterns that disqualify a file.
	// Matched against the full file path.
	ExcludePatterns []string

	// MinContentChars is the minimum content length in characters.
	// Files shorter than this are excluded. Default: 500.
	MinContentChars int

	// IncludeNonSearchable controls whether files that would be marked
	// non-searchable (like All-Concepts.md) are still imported.
	// When true, they are added with Searchable=false.
	IncludeNonSearchable bool
}

// DefaultWikiFilter returns sensible defaults for Obsidian wiki import.
func DefaultWikiFilter() *WikiFilter {
	return &WikiFilter{
		ExcludeNames: []string{
			"index.md",
			"log.md",
			"CLAUDE.md",
			"_orphan_analysis.md",
			"Concept-Hierarchy.md",
			"Concept-Index.md",
		},
		ExcludePatterns: []string{
			"-分拆目录",       // "专利法-2020-拆分-01-分拆目录.md"
			"-拆分-01-分拆目录", // alternate pattern
		},
		MinContentChars:      500,
		IncludeNonSearchable: false,
	}
}

// ShouldImport returns whether a file should be imported.
func (f *WikiFilter) ShouldImport(path string) bool {
	name := filepath.Base(path)

	// Exclude by exact file name.
	for _, exclude := range f.ExcludeNames {
		if strings.EqualFold(name, exclude) {
			return false
		}
	}

	// Exclude by path pattern.
	for _, pattern := range f.ExcludePatterns {
		if strings.Contains(path, pattern) {
			return false
		}
	}

	return true
}

// IsSearchable returns whether imported content should be included in retrieval.
// Index/concept pages are imported (for reference) but excluded from search.
func (f *WikiFilter) IsSearchable(path string) bool {
	name := filepath.Base(path)

	// All-Concepts and similar index pages are useful but shouldn't appear in
	// retrieval results since they're table-of-contents, not substantive content.
	nonSearchable := []string{
		"All-Concepts.md",
		"All-Concepts-拆分-",
		"Concept-Index.md",
		"Concept-Hierarchy.md",
	}
	for _, ns := range nonSearchable {
		if strings.Contains(name, ns) {
			return false
		}
	}
	return true
}

// ContentTooShort checks whether content meets the minimum length requirement.
func (f *WikiFilter) ContentTooShort(content string) bool {
	minChars := f.MinContentChars
	if minChars <= 0 {
		minChars = 500
	}
	return len([]rune(content)) < minChars
}

// SplitFragmentName checks if a file name matches the split-fragment pattern
// with repeated markers like (1)(1)(1) indicating excessive splitting.
func IsSplitFragment(path string) bool {
	name := filepath.Base(path)
	// Files with too many nested split markers are likely noise.
	count := strings.Count(name, "(1)")
	return count >= 3
}
