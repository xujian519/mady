package loader

import (
	"regexp"
	"strings"
)

// WikiMetadata holds extracted metadata from a wiki document.
type WikiMetadata struct {
	Title      string   // first H1 heading
	Source     string   // "> **来源：** ..."
	LawRefs    []string // "> **核心法条：** ..." or "> **法律依据：** ..."
	WikiLinks  []string // all [[wikilinks]] in the document
	Domain     string   // derived from directory structure (e.g. "专利侵权")
	DocType    string   // "judgment", "guideline", "law", "wiki_card", "reexam", "practice"
	Summary    string   // "## 核心要点" or "## 核心审查标准" content
	Tags       []string // "> **标签：** ..." fields (reexam docs)
	ReviewedAt string   // "> **最后审查：** ..." or "> **reviewed_at：** ..." (version tracking)
}

// wikiMetadataPatterns holds regex patterns for metadata extraction.
var (
	reSource     = regexp.MustCompile(`>\s*\*\*来源[：:]\s*\*\*\s*(.+)`)
	reLawRefs    = regexp.MustCompile(`>\s*\*\*(?:核心法条|对应法条|法律依据)[：:]\s*\*\*\s*(.+)`)
	reWikiLink   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	reTags       = regexp.MustCompile(`>\s*\*\*标签[：:]\s*\*\*\s*(.+)`)
	reH1         = regexp.MustCompile(`^#\s+(.+)`)
	reSummary    = regexp.MustCompile(`##\s*(?:核心要点|核心审查标准|核心概述)\s*\n([\s\S]*?)(?:\n##|\z)`)
	reTechField  = regexp.MustCompile(`>\s*\*\*技术领域[：:]\s*\*\*\s*(.+)`)
	reGuideSect  = regexp.MustCompile(`>\s*\*\*来源[：:]\s*\*\*\s*《专利审查指南》(.+)`)
	reReviewedAt = regexp.MustCompile(`>\s*\*\*(?:最后审查|reviewed_at)[：:]\s*\*\*\s*(.+)`)
)

// ExtractMetadata parses metadata from wiki document content and file path.
func ExtractMetadata(content, filePath string) *WikiMetadata {
	m := &WikiMetadata{}

	// Title: first H1 heading.
	if match := reH1.FindStringSubmatch(content); len(match) >= 2 {
		m.Title = strings.TrimSpace(match[1])
	}

	// Source.
	if match := reSource.FindStringSubmatch(content); len(match) >= 2 {
		m.Source = strings.TrimSpace(match[1])
	}

	// Law references.
	if match := reLawRefs.FindStringSubmatch(content); len(match) >= 2 {
		refs := strings.TrimSpace(match[1])
		m.LawRefs = splitRefs(refs)
	}

	// Wiki links.
	allLinks := reWikiLink.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	for _, match := range allLinks {
		link := strings.TrimSpace(match[1])
		// Skip image/embed links.
		if idx := strings.Index(link, "|"); idx >= 0 {
			link = link[:idx]
		}
		if !seen[link] {
			seen[link] = true
			m.WikiLinks = append(m.WikiLinks, link)
		}
	}

	// Tags (reexam documents).
	if match := reTags.FindStringSubmatch(content); len(match) >= 2 {
		tags := strings.TrimSpace(match[1])
		for _, tag := range strings.Split(tags, "；") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				m.Tags = append(m.Tags, tag)
			}
		}
	}

	// Summary.
	if match := reSummary.FindStringSubmatch(content); len(match) >= 2 {
		summary := strings.TrimSpace(match[1])
		// Truncate to ~300 chars for the summary field.
		if len([]rune(summary)) > 300 {
			summary = string([]rune(summary)[:300]) + "..."
		}
		m.Summary = summary
	}

	// Domain and DocType from path.
	m.Domain, m.DocType = classifyWikiPath(filePath)

	// Additional metadata from content patterns.
	if m.DocType == "guideline" {
		if match := reGuideSect.FindStringSubmatch(content); len(match) >= 2 {
			// Extract section from "第X部分第Y章"
			m.Source = strings.TrimSpace(match[1])
		}
	}
	if techMatch := reTechField.FindStringSubmatch(content); len(techMatch) >= 2 {
		_ = techMatch // technical field for reexam docs
	}

	// Reviewed/last-reviewed date (version tracking for obsolescence detection).
	if match := reReviewedAt.FindStringSubmatch(content); len(match) >= 2 {
		m.ReviewedAt = strings.TrimSpace(match[1])
	}

	return m
}

// splitRefs splits a law reference string into individual references.
func splitRefs(refs string) []string {
	var result []string
	// Split by Chinese/English semicolons.
	for _, part := range strings.FieldsFunc(refs, func(r rune) bool {
		return r == '；' || r == ';' || r == '、'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// classifyWikiPath derives domain and document type from the file path.
// Path example: Wiki/专利侵权/侵权判定/侵权判定-全面覆盖原则.md
func classifyWikiPath(path string) (domain, docType string) {
	normalized := strings.ReplaceAll(path, "\\", "/")

	switch {
	case strings.Contains(normalized, "/cards/"):
		return "patent", "wiki_card"
	case strings.Contains(normalized, "/专利侵权/"):
		return "patent", "judgment"
	case strings.Contains(normalized, "/专利判决/"):
		return "patent", "judgment"
	case strings.Contains(normalized, "/复审无效/"):
		return "patent", "reexam"
	case strings.Contains(normalized, "/审查指南/"):
		return "patent", "guideline"
	case strings.Contains(normalized, "/专利实务/"):
		return "patent", "practice"
	case strings.Contains(normalized, "/法律法规/"):
		return "legal", "law"
	case strings.Contains(normalized, "/书籍/"):
		return "reference", "book"
	default:
		return "patent", "wiki_card"
	}
}
