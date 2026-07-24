package reasoning

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/knowledge/standards"
)

// IPCStandardAdapter adapts the knowledge/standards IPCStandardSource to the
// reasoning framework's IPCStandardSource interface, enabling IPC examination
// standards to be queried during Stage ② rule acquisition.
//
// It wraps LoadStandards/FindByIPCSection/FindByArticle and converts results
// into RetrievedRule objects that integrate seamlessly with the existing
// rule aggregation and deduplication pipeline.
type IPCStandardAdapter struct{}

// NewIPCStandardAdapter creates a new adapter. It verifies that standards are
// loadable during construction; if the embedded YAML is missing or malformed,
// the adapter returns nil and an error.
func NewIPCStandardAdapter() (*IPCStandardAdapter, error) {
	// Verify the standards are loadable.
	_, err := standards.LoadStandards()
	if err != nil {
		return nil, fmt.Errorf("IPC standards: %w", err)
	}
	return &IPCStandardAdapter{}, nil
}

// MustIPCStandardAdapter creates a new adapter and panics on error.
// Use during startup when IPC standards are required.
func MustIPCStandardAdapter() *IPCStandardAdapter {
	a, err := NewIPCStandardAdapter()
	if err != nil {
		panic(err)
	}
	return a
}

// MatchByIPC returns examination standards matching the given IPC section and
// legal article. The results are returned as RetrievedRule objects.
//
// Parameters:
//   - ipcSection: IPC section letter (e.g. "G", "A") or a tech field keyword
//     that may contain an IPC code (e.g. "G06", "computing").
//   - article: legal article ID (e.g. "patent-law-a22.3") or empty for all.
//   - queryCtx: context map that may contain additional filtering hints.
func (a *IPCStandardAdapter) MatchByIPC(_ context.Context, ipcSection, article string, queryCtx map[string]string) ([]RetrievedRule, error) {
	var matched []standards.IPCStandard

	// Try matching as an IPC detail code first (e.g., "G06", "H04", "A61").
	if isIPCDetail(ipcSection) {
		found, err := standards.FindByIPCDetail(ipcSection)
		if err != nil {
			return nil, fmt.Errorf("FindByIPCDetail: %w", err)
		}
		matched = found
	} else if len(ipcSection) == 1 && ipcSection >= "A" && ipcSection <= "H" {
		// Treat single uppercase letter as an IPC section.
		found, err := standards.FindByIPCSection(ipcSection)
		if err != nil {
			return nil, fmt.Errorf("FindByIPCSection: %w", err)
		}
		matched = found
	} else {
		// Treat as a keyword search.
		found, err := standards.Search(ipcSection)
		if err != nil {
			return nil, fmt.Errorf("Search: %w", err)
		}
		matched = found
		// If no results, fall back to "ALL" (cross-field standards).
		if len(matched) == 0 {
			all, err := standards.FindByIPCSection("ALL")
			if err == nil {
				matched = all
			}
		}
	}

	// Filter by article if specified.
	if article != "" {
		var filtered []standards.IPCStandard
		for _, s := range matched {
			if s.Article == article || s.Article == "" {
				filtered = append(filtered, s)
			}
		}
		matched = filtered
	}

	// Convert to RetrievedRule objects.
	rules := make([]RetrievedRule, 0, len(matched))
	for _, s := range matched {
		baggage := standards.FormatAsContext([]standards.IPCStandard{s})
		rules = append(rules, RetrievedRule{
			Rule: RuleConstraint{
				ArticleID:   s.Article,
				ArticleName: s.Name,
				Requirement: ReqNote,
				Description: fmt.Sprintf("IPC 审查标准 [%s/%s]: %s", s.IPCSection, s.IPCDetail, s.Name),
			},
			Source:         RuleSourceIPC,
			Priority:       3, // IPC standards are reference material, not hard rules
			AuthorityScore: 0.7,
			Confidence:     0.8,
			Baggage:        baggage,
		})
	}

	// Apply max-per-source limit from query context.
	if maxStr := queryCtx["max_rules"]; maxStr != "" {
		var max int
		if _, err := fmt.Sscanf(maxStr, "%d", &max); err == nil && max > 0 && len(rules) > max {
			rules = rules[:max]
		}
	}

	return rules, nil
}

// isIPCDetail checks if a string looks like an IPC detail code (e.g., "G06", "A61").
func isIPCDetail(s string) bool {
	if len(s) != 3 {
		return false
	}
	if s[0] < 'A' || s[0] > 'H' {
		return false
	}
	return s[1] >= '0' && s[1] <= '9' && s[2] >= '0' && s[2] <= '9'
}

// ensure interface compliance
var _ IPCStandardSource = (*IPCStandardAdapter)(nil)
