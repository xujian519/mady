package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	rbrowser "github.com/xujian519/mady/retrieval/domain/browser"
	"github.com/xujian519/mady/retrieval/domain/cnipr"
)

// ToolCNIPR 是 CNIPR 通道工具的注册名。
const ToolCNIPR = "cnipr_search"

const cniprToolDescription = `通过 CNIPR（中国知识产权网 search.cnipr.com）检索中国专利、取全文 PDF 直链与查法律状态，复用 ego lite 的登录态。

mode：
- search（默认）：关键词检索，返回总数与命中（公开号、标题、公开日、申请人、摘要）；单次最多 30 条。
- pdf_links：按公开(公告)号取全文 PDF 直链（仅免费可得的单篇全文；概览页批量下载是付费功能）。非公开号按条目报错。
- legal_status：按申请号或公开(公告)号查法律状态；公开号会先解析申请号。零命中时 total 为空、记录为空。

失败分两类：通道级失败（登录态失效/未安装 ego/超时/站点结构变化）返回带分类的错误，此时请改用 patent_web_search（Google Patents / CNIPA / Espacenet）继续；号码级失败按条目回报，不中断同批其余号。
页内脚本依赖 CNIPR 当前页面结构；站点改版会报 page_state，不要据此断言专利不存在。`

// registerCNIPRTool 在 ego 可用且未被保密性门控关闭时注册 CNIPR 工具。
// add 由调用方提供（含只读包装），使本函数不依赖调用点的闭包。
func registerCNIPRTool(add func(*agentcore.Tool), cfg *PatentWebSearchConfig) {
	if !rbrowser.RetrieversEnabled() {
		return
	}
	webCfg := patentWebConfig(cfg)
	if webCfg.EgoBrowserPath == "" {
		return
	}
	add(NewCNIPRTool(webCfg))
}

// NewCNIPRTool 创建 CNIPR 工具。ego-browser 不可用时由调用方跳过注册。
func NewCNIPRTool(cfg PatentWebSearchConfig) *agentcore.Tool {
	egoPath := cfg.EgoBrowserPath
	if egoPath == "" {
		egoPath = rbrowser.FindEgoBrowser()
	}
	channel := cnipr.NewChannel(&cnipr.EgoSession{Executable: egoPath})
	return newCNIPRToolWithChannel(channel)
}

func newCNIPRToolWithChannel(channel *cnipr.Channel) *agentcore.Tool {
	t := &cniprTool{channel: channel}
	return &agentcore.Tool{
		Name:        ToolCNIPR,
		Description: cniprToolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"search", "pdf_links", "legal_status"},
					"description": "检索 / 取 PDF 直链 / 查法律状态，默认 search",
				},
				"query": map[string]any{"type": "string", "description": "检索词（mode=search）"},
				"numbers": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "公开(公告)号或申请号列表（mode=pdf_links/legal_status）",
				},
				"limit": map[string]any{
					"type": "integer", "minimum": 1, "maximum": cnipr.SearchMaxHits,
					"description": "检索条数（默认 10，最多 30）",
				},
			},
			"additionalProperties": false,
		},
		ReadOnly: true,
		Func:     t.Run,
	}
}

// CNIPRToolResult 是工具输出。
type CNIPRToolResult struct {
	Mode     string                  `json:"mode"`
	Search   *cnipr.SearchResult     `json:"search,omitempty"`
	PDFLinks []cnipr.PdfLinkItem     `json:"pdf_links,omitempty"`
	Legal    []cnipr.LegalStatusItem `json:"legal_status,omitempty"`
	Rendered string                  `json:"rendered"`
}

type cniprTool struct {
	channel *cnipr.Channel
}

type cniprArgs struct {
	Mode    string   `json:"mode"`
	Query   string   `json:"query"`
	Numbers []string `json:"numbers"`
	Limit   int      `json:"limit"`
}

func (t *cniprTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input cniprArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	mode := input.Mode
	if mode == "" {
		mode = "search"
	}
	switch mode {
	case "search":
		if strings.TrimSpace(input.Query) == "" {
			return nil, fmt.Errorf("mode=search 需要 query")
		}
		result, err := t.channel.Search(strings.TrimSpace(input.Query), input.Limit)
		if err != nil {
			return nil, cniprFallbackError(err)
		}
		return CNIPRToolResult{Mode: mode, Search: result, Rendered: renderCNIPRSearch(result)}, nil
	case "pdf_links":
		if len(input.Numbers) == 0 {
			return nil, fmt.Errorf("mode=pdf_links 需要 numbers")
		}
		items, err := t.channel.PDFLinks(input.Numbers)
		if err != nil {
			return nil, cniprFallbackError(err)
		}
		return CNIPRToolResult{Mode: mode, PDFLinks: items, Rendered: renderCNIPRPDFLinks(items)}, nil
	case "legal_status":
		if len(input.Numbers) == 0 {
			return nil, fmt.Errorf("mode=legal_status 需要 numbers")
		}
		items, err := t.channel.LegalStatus(input.Numbers)
		if err != nil {
			return nil, cniprFallbackError(err)
		}
		return CNIPRToolResult{Mode: mode, Legal: items, Rendered: renderCNIPRLegal(items)}, nil
	default:
		return nil, fmt.Errorf("未知 mode %q（可选 search/pdf_links/legal_status）", mode)
	}
}

// cniprFallbackError 给通道级失败补上去向：告诉调用方改用哪条检索通道。
func cniprFallbackError(err error) error {
	return fmt.Errorf("%w；可改用 patent_web_search（Google Patents / CNIPA / Espacenet）继续检索", err)
}

func renderCNIPRSearch(result *cnipr.SearchResult) string {
	if result == nil {
		return "CNIPR 检索没有返回结果。"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "**CNIPR 检索**：全部 %d 件，返回 %d 条\n", result.Total, len(result.Hits))
	for i, hit := range result.Hits {
		fmt.Fprintf(&builder, "\n[%d] %s\n", i+1, firstNonEmpty(hit.Title, hit.Number))
		if hit.Number != "" {
			fmt.Fprintf(&builder, "- 公开(公告)号：%s\n", hit.Number)
		}
		if hit.PublicationDate != "" {
			fmt.Fprintf(&builder, "- 公开日：%s\n", hit.PublicationDate)
		}
		if hit.Assignee != "" {
			fmt.Fprintf(&builder, "- 申请人：%s\n", hit.Assignee)
		}
		if hit.Abstract != "" {
			fmt.Fprintf(&builder, "- 摘要：%s\n", hit.Abstract)
		}
	}
	if len(result.Hits) == 0 {
		builder.WriteString("\n零命中：换检索词，或改用 patent_web_search 的多源检索；零命中不证明该技术不存在。\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func renderCNIPRPDFLinks(items []cnipr.PdfLinkItem) string {
	var builder strings.Builder
	builder.WriteString("**CNIPR 全文 PDF 直链**\n")
	for _, item := range items {
		if item.Error != "" {
			fmt.Fprintf(&builder, "- %s：%s\n", item.Number, item.Error)
			continue
		}
		status := ""
		if item.Unexamined {
			status = "（未审查）"
		}
		if item.Authorized {
			status += "（已授权）"
		}
		if len(item.URLs) == 0 {
			fmt.Fprintf(&builder, "- %s%s：未取到直链\n", item.Number, status)
			continue
		}
		fmt.Fprintf(&builder, "- %s%s：%s\n", item.Number, status, strings.Join(item.URLs, "、"))
	}
	return strings.TrimRight(builder.String(), "\n")
}

func renderCNIPRLegal(items []cnipr.LegalStatusItem) string {
	var builder strings.Builder
	builder.WriteString("**CNIPR 法律状态**\n")
	for _, item := range items {
		if item.Error != "" {
			fmt.Fprintf(&builder, "- %s：%s\n", item.Number, item.Error)
			continue
		}
		total := "未给出记录数"
		if item.Total != nil {
			total = fmt.Sprintf("共 %d 条记录", *item.Total)
		}
		appNumber := item.AppNumber
		if appNumber == "" {
			appNumber = "未解析"
		}
		fmt.Fprintf(&builder, "- %s（申请号 %s，%s）\n", item.Number, appNumber, total)
		for _, record := range item.Records {
			fmt.Fprintf(&builder, "  - %s %s：%s\n", record.AnnounceDate, record.Status, record.StatusInfo)
		}
		if len(item.Records) == 0 {
			builder.WriteString("  - 未检索到法律状态记录：不代表该专利不存在，可能是号码形态或号单口径不一致。\n")
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}
