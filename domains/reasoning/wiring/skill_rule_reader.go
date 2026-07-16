package wiring

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xujian519/mady/domains/reasoning"
)

// SkillRuleReader reads rule-like experiential knowledge from Obsidian
// patent-cards and adapts them as a reasoning RuleSkillReader. It is the
// third rule-acquisition source in Stage ② (获取规则), corresponding to the
// "Wiki 实务经验补充" authority tier in design-rule-acquisition-stage.md.
//
// Authority is intentionally the lowest of the three lanes (0.4): wiki cards
// capture review-practice tendencies and edge-case experience, never legal
// authority. They surface as Priority-3 suggestions, never as Must/Should.
type SkillRuleReader struct {
	cardDir string // absolute path to patent-cards/
}

// NewSkillRuleReader binds a wiki root. Returns nil if wikiRoot is empty so
// callers can pass an unresolved path and let a nil reader disable the lane.
func NewSkillRuleReader(wikiRoot string) *SkillRuleReader {
	if wikiRoot == "" {
		return nil
	}
	return &SkillRuleReader{cardDir: filepath.Join(wikiRoot, "patent-cards")}
}

// patentCard holds the parsed fields of one Obsidian patent-card.
type patentCard struct {
	Title   string  // first H1 line
	Concept string  // - 概念:
	Domain  string  // - 领域: ("未分类" treated as unset)
	Quality float64 // - 质量分: (default 0.5 when absent/parse error)
	Body    string  // text after the "## 卡片内容" marker
}

// ReadRules scans patent-cards/*.md and returns one RetrievedRule per card.
// domain filters by the card's 领域 field when non-empty; cards with no
// usable domain label ("未分类" or absent) pass the filter unconditionally,
// reflecting that experience notes often transcend a single sub-domain.
func (r *SkillRuleReader) ReadRules(ctx context.Context, domain string) ([]reasoning.RetrievedRule, error) {
	if r == nil || r.cardDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(r.cardDir)
	if err != nil {
		// Missing directory is non-fatal: the lane is simply empty.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skill reader: %w", err)
	}
	out := make([]reasoning.RetrievedRule, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		card, perr := parseCard(filepath.Join(r.cardDir, e.Name()))
		if perr != nil || card == nil {
			continue // skip malformed card rather than failing the whole lane
		}
		if domain != "" && card.Domain != "" && !domainMatches(card.Domain, domain) {
			continue
		}
		out = append(out, reasoning.RetrievedRule{
			Rule: reasoning.RuleConstraint{
				ArticleID:   card.Concept,
				ArticleName: card.Title,
				// Experience notes are advisory only — never Must/Should.
				Requirement: reasoning.ReqNote,
				Description: truncate(card.Body, 300),
			},
			Source:         reasoning.RuleSourceSkill,
			Priority:       3,   // experience tier, lowest precedence
			AuthorityScore: 0.4, // non-authoritative per design-doc authority table
			Confidence:     card.Quality,
			Baggage:        card.Body,
		})
	}
	return out, nil
}

// parseCard reads one .md file and extracts the H1 title, the "- key: value"
// metadata block, and the body following the "## 卡片内容" marker.
func parseCard(path string) (*patentCard, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	card := &patentCard{Quality: 0.5}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // cards can be long
	inBody := false
	var body strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Body begins at the "## 卡片内容" heading.
		if strings.HasPrefix(trimmed, "## 卡片内容") {
			inBody = true
			continue
		}
		if inBody {
			body.WriteString(line)
			body.WriteByte('\n')
			continue
		}

		// H1 title.
		if card.Title == "" && strings.HasPrefix(trimmed, "# ") {
			card.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		// Metadata: "- key: value".
		if strings.HasPrefix(trimmed, "- ") {
			parseMetaLine(card, trimmed)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	card.Body = strings.TrimSpace(body.String())

	// A card with neither a usable concept/title nor body is unusable.
	if card.Concept == "" && card.Title == "" && card.Body == "" {
		return nil, nil
	}
	return card, nil
}

// parseMetaLine extracts a single "- key: value" entry into the card.
func parseMetaLine(card *patentCard, line string) {
	// strip leading "- "
	rest := strings.TrimPrefix(line, "- ")
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return
	}
	key := strings.TrimSpace(rest[:idx])
	val := strings.TrimSpace(rest[idx+1:])
	switch key {
	case "概念":
		card.Concept = val
	case "领域":
		if val != "" && val != "未分类" {
			card.Domain = val
		}
	case "质量分":
		if q, err := strconv.ParseFloat(val, 64); err == nil {
			card.Quality = q
		}
	}
}

// domainMatches reports whether a card's domain relates to the requested
// domain. Matching is substring-based and case-insensitive to tolerate
// label drift (e.g. "专利授权" vs "专利授权/新颖性"). Either direction counts
// so a request for "patent_novelty" still finds a card tagged "专利授权".
func domainMatches(cardDomain, requested string) bool {
	cd := strings.ToLower(cardDomain)
	rq := strings.ToLower(requested)
	return strings.Contains(cd, rq) || strings.Contains(rq, cd)
}
