package novelty

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// NoveltyOption 是 NewNoveltyTool 的函数式选项。
type NoveltyOption func(*noveltyConfig)

type noveltyConfig struct {
	provider agentcore.Provider
}

// WithProvider 设置 LLM provider。
func WithProvider(p agentcore.Provider) NoveltyOption {
	return func(c *noveltyConfig) { c.provider = p }
}

// NewNoveltyTool 创建 A22.2 新颖性评估工具。
// 工具通过 Pregel 子图运行专利法第 22 条第 2 款的新颖性判断，
// 返回结构化的 NoveltyResult。
func NewNoveltyTool(opts ...NoveltyOption) *agentcore.Tool {
	cfg := &noveltyConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &agentcore.Tool{
		Name: "evaluate_novelty",
		Description: strings.Join([]string{
			"评估技术方案是否具备专利法第22条第2款规定的新颖性。",
			"基于权利要求和对比文件，通过单独对比原则判断：",
			"现有技术审查→相同或实质相同判断→抵触申请审查→宽限期/优先权例外。",
			"支持 A22.2 / 22.2 / 新颖性 / 单独对比 / novelty 相关分析。",
		}, " "),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claims": map[string]any{
					"type":        "array",
					"description": "权利要求文本列表，每个包含 id/text/type",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
							"type": map[string]any{"type": "string"},
						},
					},
				},
				"prior_art_docs": map[string]any{
					"type":        "array",
					"description": "对比文件列表，每个包含 doc_id/title/snippet/pub_date/pub_type/score",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"doc_id":   map[string]any{"type": "string"},
							"title":    map[string]any{"type": "string"},
							"snippet":  map[string]any{"type": "string"},
							"pub_date": map[string]any{"type": "string"},
							"pub_type": map[string]any{"type": "string"},
							"score":    map[string]any{"type": "number"},
						},
					},
				},
				"conflict_apps": map[string]any{
					"type":        "array",
					"description": "抵触申请列表（可选）",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"app_id":      map[string]any{"type": "string"},
							"title":       map[string]any{"type": "string"},
							"filing_date": map[string]any{"type": "string"},
							"pub_date":    map[string]any{"type": "string"},
							"full_text":   map[string]any{"type": "string"},
						},
					},
				},
				"filing_date": map[string]any{
					"type":        "string",
					"description": "申请日",
				},
				"priority_date": map[string]any{
					"type":        "string",
					"description": "优先权日（可选）",
				},
				"tech_domain": map[string]any{
					"type":        "string",
					"description": "技术领域（如 mechanical/chemistry/computer 等）",
				},
				"evidence_coverage": map[string]any{
					"type":        "string",
					"description": "证据覆盖状态：full/partial/none。none 时跳过评估",
				},
			},
			"required": []string{"claims", "prior_art_docs", "filing_date"},
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return runNoveltyTool(ctx, cfg, args)
		},
	}
}

// runNoveltyTool 是 evaluate_novelty 工具的实际执行函数。
func runNoveltyTool(ctx context.Context, cfg *noveltyConfig, args json.RawMessage) (any, error) {
	if cfg.provider == nil {
		return map[string]any{"error": "provider 未配置，无法运行新颖性评估"}, nil
	}

	input := parseNoveltyArgs(args)
	if input == nil {
		return map[string]any{"error": "参数解析失败，请提供 claims 和 prior_art_docs"}, nil
	}

	compiled, err := BuildNoveltyGraph(cfg.provider)
	if err != nil {
		return map[string]any{"error": "构建评估图失败: " + err.Error()}, nil
	}

	state := graph.PregelState{}
	state[StateKeyNoveltyInput] = input

	state, runErr := compiled.Run(ctx, state)

	if raw, ok := state[StateKeyNoveltyResult]; ok {
		if result, ok := raw.(*NoveltyResult); ok {
			if runErr != nil {
				return map[string]any{
					"result": result,
					"error":  runErr.Error(),
				}, nil
			}
			return result, nil
		}
	}

	if runErr != nil {
		return map[string]any{"error": runErr.Error()}, nil
	}
	return map[string]any{"error": "评估图执行后未找到结果"}, nil
}

// parseNoveltyArgs 从 JSON 参数解析 NoveltyInput。
func parseNoveltyArgs(args json.RawMessage) *NoveltyInput {
	input := &NoveltyInput{}

	var raw struct {
		Claims []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"claims"`
		PriorArtDocs []struct {
			DocID   string  `json:"doc_id"`
			Title   string  `json:"title"`
			Snippet string  `json:"snippet"`
			PubDate string  `json:"pub_date"`
			PubType string  `json:"pub_type"`
			Score   float64 `json:"score"`
		} `json:"prior_art_docs"`
		ConflictApps []struct {
			AppID      string `json:"app_id"`
			Title      string `json:"title"`
			FilingDate string `json:"filing_date"`
			PubDate    string `json:"pub_date"`
			FullText   string `json:"full_text"`
		} `json:"conflict_apps"`
		FilingDate       string `json:"filing_date"`
		PriorityDate     string `json:"priority_date"`
		TechDomain       string `json:"tech_domain"`
		EvidenceCoverage string `json:"evidence_coverage"`
	}
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil
	}

	for _, c := range raw.Claims {
		input.Claims = append(input.Claims, ClaimText{
			ID:   c.ID,
			Text: c.Text,
			Type: c.Type,
		})
	}
	for _, d := range raw.PriorArtDocs {
		input.PriorArtDocs = append(input.PriorArtDocs, PriorArtDoc{
			DocID:   d.DocID,
			Title:   d.Title,
			Snippet: d.Snippet,
			PubDate: d.PubDate,
			PubType: d.PubType,
			Score:   d.Score,
		})
	}
	for _, ca := range raw.ConflictApps {
		input.ConflictApps = append(input.ConflictApps, ConflictApp{
			AppID:      ca.AppID,
			Title:      ca.Title,
			FilingDate: ca.FilingDate,
			PubDate:    ca.PubDate,
			FullText:   ca.FullText,
		})
	}
	input.FilingDate = raw.FilingDate
	input.PriorityDate = raw.PriorityDate
	input.TechDomain = raw.TechDomain
	input.EvidenceCoverage = raw.EvidenceCoverage
	if input.EvidenceCoverage == "" {
		input.EvidenceCoverage = "partial"
	}
	if len(input.Claims) > 0 && len(input.PriorArtDocs) > 0 && input.EvidenceCoverage == "partial" {
		input.EvidenceCoverage = "full"
	}

	return input
}
