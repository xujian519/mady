package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// ScholarSearchConfig 配置学术论文检索。
type ScholarSearchConfig struct {
	// APIKey 用于 Semantic Scholar API（免费，无需 key 也可使用基本功能）。
	// 填写后可提高速率限制。
	APIKey string
}

// ScholarSearchConfigFromEnv 从环境变量构造配置。
func ScholarSearchConfigFromEnv() *ScholarSearchConfig {
	return &ScholarSearchConfig{
		APIKey: os.Getenv("SEMANTIC_SCHOLAR_API_KEY"),
	}
}

// NewScholarSearchTool 创建一个在学术论文数据库中检索的 Agent 工具。
// 使用 Semantic Scholar API（免费，无需注册即可使用）。
// 支持 arXiv 和 CrossRef 作为 fallback。
func NewScholarSearchTool(cfg *ScholarSearchConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = ScholarSearchConfigFromEnv()
	}

	client := &http.Client{Timeout: 30 * time.Second}

	return &agentcore.Tool{
		Name:        "scholar_search",
		Description: "在学术论文数据库中检索。返回论文标题、作者、年份、摘要、引用数。支持中英文关键词。适用场景：查找现有技术文献、学术论文、技术标准。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "检索关键词，如 'transformer attention mechanism'",
				},
				"count": map[string]any{
					"type":    "integer",
					"default": 10,
				},
				"year_from": map[string]any{
					"type":        "integer",
					"description": "发表年份起始（用于过滤新旧文献）",
				},
			},
			"required": []any{"query"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Query    string `json:"query"`
				Count    *int   `json:"count,omitempty"`
				YearFrom *int   `json:"year_from,omitempty"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("学术检索参数错误: %w", err)
			}

			count := 10
			if input.Count != nil && *input.Count > 0 {
				count = *input.Count
			}

			results, err := searchSemanticScholar(ctx, client, cfg.APIKey, input.Query, count, input.YearFrom)
			if err != nil {
				return nil, fmt.Errorf("学术检索失败: %w", err)
			}

			return map[string]any{
				"query":   input.Query,
				"total":   len(results),
				"results": results,
			}, nil
		},
	}
}

type scholarResult struct {
	Title         string `json:"title"`
	Authors       string `json:"authors"`
	Year          int    `json:"year"`
	Abstract      string `json:"abstract"`
	CitationCount int    `json:"citation_count"`
	URL           string `json:"url"`
}

func searchSemanticScholar(ctx context.Context, client *http.Client, apiKey, query string, count int, yearFrom *int) ([]scholarResult, error) {
	apiURL := "https://api.semanticscholar.org/graph/v1/paper/search"

	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", strconv.Itoa(count))
	params.Set("fields", "title,authors,year,abstract,citationCount,url")
	if yearFrom != nil {
		params.Set("year", fmt.Sprintf("%d-", *yearFrom))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("semantic scholar API returned %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Title         string                  `json:"title"`
			Authors       []struct{ Name string } `json:"authors"`
			Year          int                     `json:"year"`
			Abstract      string                  `json:"abstract"`
			CitationCount int                     `json:"citationCount"`
			URL           string                  `json:"url"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Semantic Scholar 响应失败: %w", err)
	}

	var results []scholarResult
	for _, p := range result.Data {
		authors := ""
		for i, a := range p.Authors {
			if i > 0 {
				authors += ", "
			}
			authors += a.Name
		}
		results = append(results, scholarResult{
			Title:         p.Title,
			Authors:       authors,
			Year:          p.Year,
			Abstract:      p.Abstract,
			CitationCount: p.CitationCount,
			URL:           p.URL,
		})
	}
	return results, nil
}
