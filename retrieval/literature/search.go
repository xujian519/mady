package literature

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// SearchResult 是多源检索的合并结果。
type SearchResult struct {
	Query   string            `json:"query"`
	Sources []string          `json:"sources"`
	Papers  []Paper           `json:"papers"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// SearchAll 并发查询指定源；单个源失败只记录错误，不中断其余源。
// sources 为空时查询全部源；未知源记入 Errors。
func (r *Registry) SearchAll(ctx context.Context, query string, opts SearchOptions, sources []string) SearchResult {
	result := SearchResult{Query: query, Errors: make(map[string]string)}
	selected := sources
	if len(selected) == 0 {
		selected = append([]string(nil), r.order...)
	}
	result.Sources = append([]string(nil), selected...)

	type sourceOutcome struct {
		source string
		papers []Paper
		err    error
	}
	outcomes := make(chan sourceOutcome, len(selected))
	var wg sync.WaitGroup
	for _, id := range selected {
		connector, ok := r.Get(id)
		if !ok {
			result.Errors[id] = "未知文献源"
			continue
		}
		wg.Go(func() {
			papers, err := connector.Search(ctx, query, opts)
			outcomes <- sourceOutcome{source: id, papers: papers, err: err}
		})
	}
	wg.Wait()
	close(outcomes)

	var merged []Paper
	seen := make(map[string]bool)
	for outcome := range outcomes {
		if outcome.err != nil {
			result.Errors[outcome.source] = outcome.err.Error()
			continue
		}
		for _, paper := range outcome.papers {
			key := dedupeKey(paper)
			if key != "" && seen[key] {
				continue
			}
			if key != "" {
				seen[key] = true
			}
			merged = append(merged, paper)
		}
	}
	result.Papers = merged
	return result
}

// dedupeKey 生成跨源去重键：DOI 优先，其次规范化标题。
func dedupeKey(paper Paper) string {
	doi := strings.TrimSpace(paper.DOI)
	if doi == "" {
		doi = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(paper.ID)), "https://doi.org/")
	}
	if doi = strings.ToLower(doi); doi != "" && strings.HasPrefix(doi, "10.") {
		return "doi:" + doi
	}
	title := strings.ToLower(strings.Join(strings.Fields(paper.Title), " "))
	if len([]rune(title)) < 8 {
		return ""
	}
	return "title:" + title
}

// ValidateSources 校验源 ID 列表；返回未知源。
func (r *Registry) ValidateSources(sources []string) []string {
	var unknown []string
	for _, id := range sources {
		if _, ok := r.Get(id); !ok {
			unknown = append(unknown, id)
		}
	}
	return unknown
}

// FormatErrors 把源错误拼成一行摘要。
func FormatErrors(errors map[string]string) string {
	if len(errors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(errors))
	for source, message := range errors {
		parts = append(parts, fmt.Sprintf("%s: %s", source, message))
	}
	return strings.Join(parts, "；")
}
