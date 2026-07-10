// Package loader provides domain-specific document loaders for the
// knowledge store. Each loader parses structured content from io.Reader
// sources, making the same code usable for files, URLs, and API responses.
package loader

import (
	"io"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// PatentClaims holds structured patent claims data extracted from a document.
type PatentClaims struct {
	Title      string // patent title
	DocID      string // unique document identifier (e.g. application number)
	IPC        string // IPC classification (e.g. "G06F17/30")
	Claims     string // full claims text
	Applicant  string // patent applicant name
	FilingDate string // filing date (YYYY-MM-DD)
}

// LoadPatentClaims reads patent claims from an io.Reader and loads them
// into the knowledge store. The reader is expected to provide plain text
// claims content (e.g. from CNIPA PDF/HTML extraction).
//
// Usage:
//
//	claims, _ := loader.LoadPatentClaims(store, loader.PatentClaims{
//	    Title: "一种基于AI的专利检索方法",
//	    DocID: "CN202410000001",
//	    IPC:   "G06F17/30",
//	}, strings.NewReader(claimsText))
func LoadPatentClaims(store *knowledge.Store, info PatentClaims, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	content := string(data)

	// Build structured content with metadata header.
	var b strings.Builder
	b.WriteString("专利名称: " + info.Title + "\n")
	b.WriteString("申请号: " + info.DocID + "\n")
	if info.IPC != "" {
		b.WriteString("IPC分类号: " + info.IPC + "\n")
	}
	if info.Applicant != "" {
		b.WriteString("申请人: " + info.Applicant + "\n")
	}
	if info.FilingDate != "" {
		b.WriteString("申请日: " + info.FilingDate + "\n")
	}
	b.WriteString("\n")
	b.WriteString(content)

	return store.LoadPatentClaims(info.DocID, info.Title, b.String(), info.IPC)
}

// ParsePatentKeywords extracts common patent analysis keywords from claims text.
// Returns a deduplicated, sorted list of technical terms that can be used for
// prior art search query construction.
func ParsePatentKeywords(claimsText string) []string {
	seen := make(map[string]bool)
	var keywords []string

	// Common Chinese patent technical markers.
	markers := []string{"系统", "方法", "装置", "模块", "单元", "步骤", "所述", "其特征在于"}
	for _, marker := range markers {
		if strings.Contains(claimsText, marker) && !seen[marker] {
			seen[marker] = true
			keywords = append(keywords, marker)
		}
	}
	return keywords
}
