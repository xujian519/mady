package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval/domain"
	rbrowser "github.com/xujian519/mady/retrieval/domain/browser"
)

// PatentWebSearchConfig 配置 patent_web_search / patent_document 工具
// （ego-browser 驱动，在线专利数据库实时检索）。EgoBrowserPath 为空时自动检测。
type PatentWebSearchConfig struct {
	// EgoBrowserPath 是 ego-browser CLI 路径。为空时自动解析。
	EgoBrowserPath string
}

// patentRetrieverBundle 持有三源检索器，供 patent_web_search（按源检索）与
// patent_document（按号取全文）共用。三源共用同一 ego-browser 配置，可用性
// 同生共死：bundle != nil（工具注册门槛）已隐含三源均非 nil。
type patentRetrieverBundle struct {
	google    *rbrowser.BrowserRetriever
	cnipa     *rbrowser.BrowserRetriever
	espacenet *rbrowser.BrowserRetriever
}

// buildPatentRetrievers 构建 ego-browser 驱动的三源检索器
// （Google Patents / CNIPA / Espacenet）。
//
// MADY_BROWSER_RETRIEVERS=off（保密性隔离）或 ego-browser 不可用时返回 nil，
// 使工具不注册（静默降级）。门控与 bootstrap/init_reasoning.go、cmd/mady 的
// buildEgoCompositeRetriever 共用 rbrowser.RetrieversEnabled()。
func buildPatentRetrievers(cfg *PatentWebSearchConfig) *patentRetrieverBundle {
	if !rbrowser.RetrieversEnabled() {
		return nil
	}
	bcfg := rbrowser.DefaultConfig()
	if cfg != nil && cfg.EgoBrowserPath != "" {
		bcfg.EgoBrowserPath = cfg.EgoBrowserPath
	}
	g, c, e := rbrowser.NewDefaultPatentRetrievers(*bcfg)
	if g == nil && c == nil && e == nil {
		return nil
	}
	return &patentRetrieverBundle{google: g, cnipa: c, espacenet: e}
}

// composite 返回三源合并检索器（Search 并发查询全部源）。
func (b *patentRetrieverBundle) composite() domain.DomainRetriever {
	return rbrowser.NewCompositeRetriever(b.google, b.cnipa, b.espacenet)
}

// documentSources 返回全文取文档用检索器。跳过 CNIPA：其 detailURL/detailJS
// 与 Google 完全相同（country:CN 仅作用于搜索过滤，详情页仍指向
// patents.google.com），GetDocument 属冗余尝试；Google 未收录时由 Espacenet
// 提供 biblio 兜底。
func (b *patentRetrieverBundle) documentSources() domain.DomainRetriever {
	return rbrowser.NewCompositeRetriever(b.google, b.espacenet)
}

// bySource 按数据源名返回单源检索器；未知源返回三源合并。
func (b *patentRetrieverBundle) bySource(name string) domain.DomainRetriever {
	switch name {
	case "google":
		if b.google != nil {
			return b.google
		}
	case "cnipa":
		if b.cnipa != nil {
			return b.cnipa
		}
	case "espacenet":
		if b.espacenet != nil {
			return b.espacenet
		}
	}
	return b.composite()
}

// NewPatentWebSearchTool 基于 ego-browser 实时检索在线专利数据库
// （Google Patents / CNIPA 中国专利 / Espacenet）。
//
// 与 patent_lookup（nuo-patent，按专利号查询元数据）互补：本工具按关键词
// 检索专利并返回结构化列表，适合现有技术检索、查新、侵权风险排查等场景。
// ego-browser 不可用或 MADY_BROWSER_RETRIEVERS=off 时返回 nil（工具不注册，
// 静默降级）。
func NewPatentWebSearchTool(cfg *PatentWebSearchConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &PatentWebSearchConfig{}
	}
	bundle := buildPatentRetrievers(cfg)
	if bundle == nil {
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

			retriever := bundle.bySource(input.Source)
			results, err := retriever.Search(ctx, dq)
			if err != nil {
				return nil, fmt.Errorf("专利检索失败: %w", err)
			}
			return results, nil
		},
		ReadOnly: true,
	}
}

// patentDocumentResult 是 patent_document 的统一响应结构。命中与未命中共用
// 同一 schema，LLM 通过 found 判断结果状态，避免此前命中返回 *DomainDocument、
// 未命中返回 map 的双形状带来的解析困难。
type patentDocumentResult struct {
	PatentNumber string `json:"patent_number"`
	Found        bool   `json:"found"`
	Title        string `json:"title,omitempty"`
	Abstract     string `json:"abstract,omitempty"` // 摘要（约前 300 字符）
	Content      string `json:"content,omitempty"`  // 全文（abstract+claims+description）
	URL          string `json:"url,omitempty"`
	Source       string `json:"source,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"` // max_chars 截断时置位
	Note         string `json:"note,omitempty"`
}

// NewPatentDocumentTool 基于 ego-browser 提取专利详情页全文
// （标题 / 摘要 / 权利要求 / 说明书）。
//
// 与 patent_lookup（nuo-patent，结构化元数据）、patent_download（nuo-patent，
// PDF 下载）互补：本工具返回网页全文，适合权利要求逐条分析、技术方案理解等
// 需要完整文本的场景。全文提取以 Google Patents 收录的专利（CN/US/JP/WO 等）
// 为主；Espacenet 仅返回目录信息（无 claims/description），本工具会以
// found=true + note 明确提示而非冒充全文。max_chars 可限制返回全文长度。
// ego-browser 不可用或 MADY_BROWSER_RETRIEVERS=off 时返回 nil（工具不注册，
// 静默降级）。
func NewPatentDocumentTool(cfg *PatentWebSearchConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &PatentWebSearchConfig{}
	}
	bundle := buildPatentRetrievers(cfg)
	if bundle == nil {
		return nil
	}

	return &agentcore.Tool{
		Name: "patent_document",
		Description: "提取专利详情页全文（标题、摘要、权利要求、说明书）。输入专利号（如 US11452699B2、" +
			"CN114526990A），返回权利要求与说明书全文；Espacenet 收录的专利可能仅返回目录信息" +
			"（found=true 但无 content）。max_chars 可限制返回长度。区别于 patent_lookup（元数据）" +
			"与 patent_download（PDF 下载）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patent_number": map[string]any{
					"type":        "string",
					"description": "专利号（如 US11452699B2、CN114526990A）",
				},
				"max_chars": map[string]any{
					"type":        "integer",
					"description": "返回全文的最大字符数（默认不限；超出部分截断并在 truncated 字段标注）",
				},
			},
			"required": []any{"patent_number"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				PatentNumber string `json:"patent_number"`
				MaxChars     int    `json:"max_chars"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid input: %w", err)
			}
			if input.PatentNumber == "" {
				return nil, fmt.Errorf("patent_number 不能为空")
			}
			doc, err := bundle.documentSources().GetDocument(ctx, input.PatentNumber)
			if err != nil {
				// 全源取文档均失败（区别于"未命中"）：如实上报错误，
				// 由上层决定重试或降级，而非吞掉故障。
				return nil, fmt.Errorf("专利全文提取失败: %w", err)
			}
			res := patentDocumentResult{PatentNumber: input.PatentNumber}
			if doc == nil {
				// 复合检索器全源未命中（如专利未被 Google Patents 收录）。
				// 返回结构化提示而非裸 nil，避免 LLM 误以为调用失败。
				res.Note = "未找到该专利的详情页全文（可能未被 Google Patents / Espacenet 收录）。可改用 patent_lookup 查询元数据或 patent_download 下载 PDF。"
				return res, nil
			}
			res.Found = true
			res.Title = doc.Title
			res.Abstract = doc.Snippet
			res.URL = doc.URL
			res.Source = doc.Metadata["source"]
			// 契约保护：仅目录信息（Espacenet biblio 只有 title+abstract，
			// 无 claims/description）时不冒充全文，返回提示供 LLM 判断。
			if doc.Metadata["full_text"] == "false" {
				res.Note = "仅检索到该专利的目录信息（标题/摘要），未提取到权利要求与说明书全文（Espacenet 收录范围有限）。可改用 patent_lookup 查询元数据或 patent_download 下载 PDF。"
				return res, nil
			}
			res.Content = doc.Content
			if input.MaxChars > 0 && len([]rune(res.Content)) > input.MaxChars {
				res.Content = string([]rune(res.Content)[:input.MaxChars]) + "…"
				res.Truncated = true
			}
			return res, nil
		},
		ReadOnly: true,
	}
}
