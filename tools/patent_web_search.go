package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval/domain"
	rbrowser "github.com/xujian519/mady/retrieval/domain/browser"
)

// PatentWebSearchConfig 配置 patent_web_search 工具（ego-browser 驱动，
// 在线专利数据库实时检索）。EgoBrowserPath 为空时自动检测。
type PatentWebSearchConfig struct {
	// EgoBrowserPath 是 ego-browser CLI 路径。为空时自动解析。
	EgoBrowserPath string
}

// NewPatentWebSearchTool 基于 ego-browser 实时检索在线专利数据库
// （Google Patents / CNIPA 中国专利 / Espacenet）。
//
// 与 patent_lookup（nuo-patent，按专利号查询元数据）互补：本工具按关键词
// 检索专利并返回结构化列表，适合现有技术检索、查新、侵权风险排查等场景。
// ego-browser 不可用时返回 nil（工具不注册，静默降级）。
func NewPatentWebSearchTool(cfg *PatentWebSearchConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &PatentWebSearchConfig{}
	}
	bcfg := rbrowser.DefaultConfig()
	if cfg.EgoBrowserPath != "" {
		bcfg.EgoBrowserPath = cfg.EgoBrowserPath
	}
	google := rbrowser.NewGooglePatentsRetriever(*bcfg)
	cnipa := rbrowser.NewCNIPARetriever(*bcfg)
	espacenet := rbrowser.NewEspacenetRetriever(*bcfg)
	composite := rbrowser.NewCompositeRetriever(google, cnipa, espacenet)
	if composite == nil {
		return nil
	}

	return &agentcore.Tool{
		Name: "patent_web_search",
		Description: "实时检索在线专利数据库（Google Patents 全球 / 中国专利 / Espacenet 欧洲），" +
			"返回专利号、标题、申请人、日期、摘要与 PDF 链接。适用于现有技术检索、查新、侵权排查等" +
			"需要最新专利数据的场景。按关键词检索，区别于 patent_lookup 的按专利号查询。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "检索关键词（支持中文），如 '深度学习图像识别'",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "返回结果条数（默认 10，最大 100）",
				},
				"source": map[string]any{
					"type":        "string",
					"enum":        []any{"auto", "google", "cnipa", "espacenet"},
					"description": "数据源：auto 合并全部（默认），google 全球，cnipa 中国专利，espacenet 欧洲",
				},
			},
			"required": []any{"query"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Query      string `json:"query"`
				MaxResults int    `json:"max_results"`
				Source     string `json:"source"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid input: %w", err)
			}
			if input.Query == "" {
				return nil, fmt.Errorf("query 不能为空")
			}
			// 与 schema 声明一致：钳制到 [1,100]。
			if input.MaxResults <= 0 {
				input.MaxResults = 10
			}
			if input.MaxResults > 100 {
				input.MaxResults = 100
			}
			dq := domain.DomainQuery{Text: input.Query, MaxResults: input.MaxResults}

			var retriever domain.DomainRetriever = composite
			switch input.Source {
			case "google":
				retriever = google
			case "cnipa":
				retriever = cnipa
			case "espacenet":
				retriever = espacenet
			}
			// 三源共用同一 ego-browser 配置，可用性同生共死：
			// composite != nil（工具注册门槛）已隐含三者均非 nil。

			results, err := retriever.Search(ctx, dq)
			if err != nil {
				return nil, fmt.Errorf("专利检索失败: %w", err)
			}
			return results, nil
		},
		ReadOnly: true,
	}
}
