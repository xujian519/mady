package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval/literature"
)

// 文献工具名。
const (
	ToolPaperSearch      = "paper_search"
	ToolPaperListSources = "paper_list_sources"
	ToolPaperDownload    = "paper_download"
)

// PaperLiteratureConfig 配置文献检索与下载。
type PaperLiteratureConfig struct {
	// WorkingDir 是下载目录的沙箱边界（项目目录，缺省工作区）。
	WorkingDir string
	// OpenAlexMailto 用于 OpenAlex polite pool；空则用默认标识。
	OpenAlexMailto string
	// SemanticScholarAPIKey 提高 Semantic Scholar 限额；可空。
	SemanticScholarAPIKey string
}

// addPaperTools 把文献工具交给注册闭包（与 registerCNIPRTool 同一模式）。
func addPaperTools(add func(*agentcore.Tool), cfg ExtensionConfig) {
	for _, paperTool := range NewPaperLiteratureTools(paperLiteratureConfig(cfg)) {
		add(paperTool)
	}
}

// paperLiteratureConfig 合并显式配置与 BuildTools 注入的工作目录。
func paperLiteratureConfig(cfg ExtensionConfig) PaperLiteratureConfig {
	if cfg.PaperLiterature == nil {
		return PaperLiteratureConfig{WorkingDir: cfg.WorkingDir}
	}
	merged := *cfg.PaperLiterature
	if merged.WorkingDir == "" {
		merged.WorkingDir = cfg.WorkingDir
	}
	return merged
}

// paperTools 持有共享的注册表与下载器。
type paperTools struct {
	registry   *literature.Registry
	downloader *literature.Downloader
	workDir    string
}

// NewPaperLiteratureTools 创建文献检索、源清单与下载三个工具。
func NewPaperLiteratureTools(cfg PaperLiteratureConfig) []*agentcore.Tool {
	// 检索与下载都面向外部/模型提供的地址，统一走项目的 SSRF 防护客户端。
	client := literature.NewClient(literature.ClientOptions{
		Doer: newSSRFSafeHTTPClient(literature.DefaultTimeout),
	})
	registry := literature.NewRegistry(client, cfg.SemanticScholarAPIKey, cfg.OpenAlexMailto)
	downloader := literature.NewDownloader(literature.DownloadOptions{
		Doer: newSSRFSafeHTTPClient(literature.DefaultDownloadTimeout),
	})
	return newPaperTools(registry, downloader, cfg.WorkingDir).all()
}

// newPaperTools 用注入的注册表与下载器装配工具（测试用）。
func newPaperTools(registry *literature.Registry, downloader *literature.Downloader, workDir string) *paperTools {
	return &paperTools{registry: registry, downloader: downloader, workDir: workDir}
}

func (t *paperTools) all() []*agentcore.Tool {
	return []*agentcore.Tool{t.searchTool(), t.sourcesTool(), t.downloadTool()}
}

const paperSearchDescription = `在学术文献库中检索论文（现有技术、非专利文献）。

源：openalex（开放学术图谱）、semantic-scholar、arxiv、crossref；默认查询全部源并并发执行，
单源失败只记入 errors，不影响其余源。跨源按 DOI（其次标题）去重。
返回标题、作者、年份、期刊/会议、摘要片段、引用数、原文链接与开放获取 PDF 链接。

结果用于现有技术检索与论证；命中为空不证明该技术没有公开文献。需下载全文时，把命中的
source 与 id（或 pdf_url）交给 paper_download。`

func (t *paperTools) searchTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        ToolPaperSearch,
		Description: paperSearchDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "检索词（英文或中文）"},
				"sources": map[string]any{
					"type": "array", "items": map[string]any{"type": "string"},
					"description": "限定源（openalex/semantic-scholar/arxiv/crossref）；省略查全部",
				},
				"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 25, "description": "每个源的条数上限（默认 10）"},
				"year_from": map[string]any{"type": "integer", "minimum": 1900, "description": "最早发表年份"},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Query    string   `json:"query"`
				Sources  []string `json:"sources"`
				Limit    int      `json:"limit"`
				YearFrom int      `json:"year_from"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("参数无效: %w", err)
			}
			if strings.TrimSpace(input.Query) == "" {
				return nil, fmt.Errorf("query 不能为空")
			}
			if unknown := t.registry.ValidateSources(input.Sources); len(unknown) > 0 {
				return nil, fmt.Errorf("未知文献源：%s（可选 %s）",
					strings.Join(unknown, "、"), strings.Join(t.registry.SortedIDs(), "/"))
			}
			result := t.registry.SearchAll(ctx, strings.TrimSpace(input.Query),
				literature.SearchOptions{Limit: input.Limit, YearFrom: input.YearFrom}, input.Sources)
			return PaperSearchResult{SearchResult: result, Rendered: renderPaperSearch(result)}, nil
		},
	}
}

// PaperSearchResult 是检索工具输出。
type PaperSearchResult struct {
	literature.SearchResult
	Rendered string `json:"rendered"`
}

func renderPaperSearch(result literature.SearchResult) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "**文献检索**：%s（源：%s）— %d 篇\n",
		result.Query, strings.Join(result.Sources, "、"), len(result.Papers))
	if summary := literature.FormatErrors(result.Errors); summary != "" {
		builder.WriteString("部分源失败：" + summary + "\n")
	}
	if len(result.Papers) == 0 {
		builder.WriteString("\n无命中：换检索词或扩大源；零命中不证明该技术没有公开文献。\n")
		return strings.TrimRight(builder.String(), "\n")
	}
	for i, paper := range result.Papers {
		fmt.Fprintf(&builder, "\n[%d] %s\n", i+1, paper.Title)
		fmt.Fprintf(&builder, "- 源：%s · id：%s\n", paper.Source, paper.ID)
		if len(paper.Authors) > 0 {
			fmt.Fprintf(&builder, "- 作者：%s\n", strings.Join(paper.Authors, ", "))
		}
		if paper.Year > 0 || paper.Venue != "" || paper.CitedBy > 0 {
			fmt.Fprintf(&builder, "- 年份：%d · 期刊/会议：%s · 引用：%d\n", paper.Year, paper.Venue, paper.CitedBy)
		}
		if paper.URL != "" {
			fmt.Fprintf(&builder, "- 原文：%s\n", paper.URL)
		}
		if paper.PDFURL != "" {
			fmt.Fprintf(&builder, "- 开放获取 PDF：%s\n", paper.PDFURL)
		}
		if paper.Abstract != "" {
			fmt.Fprintf(&builder, "- 摘要：%s\n", paper.Abstract)
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}

func (t *paperTools) sourcesTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        ToolPaperListSources,
		Description: "列出可用的学术文献源及其 id、说明与主页（用于 paper_search 的 sources 参数）。",
		Parameters: map[string]any{
			"type": "object", "properties": map[string]any{}, "additionalProperties": false,
		},
		ReadOnly: true,
		Func: func(context.Context, json.RawMessage) (any, error) {
			sources := t.registry.Sources()
			var builder strings.Builder
			builder.WriteString("**可用文献源**\n")
			for _, source := range sources {
				fmt.Fprintf(&builder, "- %s（%s）：%s — %s\n", source.ID, source.Name, source.Description, source.Homepage)
			}
			return map[string]any{"sources": sources, "rendered": strings.TrimRight(builder.String(), "\n")}, nil
		},
	}
}

const paperDownloadDescription = `下载一篇论文的开放获取 PDF，落盘到工作目录内并做三检：Content-Type 不得为 HTML、
文件头必须是 %PDF-、大小在 [1 KB, 50 MB] 内。

调用方式二选一：
1. 给 source + id：按文献源解析该记录的开放获取 PDF 链接；
2. 直接给 pdf_url：跳过源解析（来源未提供 PDF 时用这个）。
output_dir 省略时写入 <工作目录>/论文原文/YYYY-MM-DD；显式目录必须位于工作目录内。
只下载开放获取全文；付费墙文献请改用原文链接人工获取。`

func (t *paperTools) downloadTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        ToolPaperDownload,
		Description: paperDownloadDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type": "string", "enum": []string{"openalex", "semantic-scholar", "arxiv", "crossref"},
					"description": "文献源 id（与 id 一起使用）",
				},
				"id":      map[string]any{"type": "string", "description": "paper_search 命中的 id（DOI/arXiv ID 等）"},
				"pdf_url": map[string]any{"type": "string", "description": "直接给 PDF 链接（跳过源解析）"},
				"title":   map[string]any{"type": "string", "description": "文件名（可选；缺省用源记录标题或链接文件名）"},
				"output_dir": map[string]any{
					"type": "string", "description": "输出目录（相对工作目录或工作目录内的绝对路径）",
				},
			},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Source    string `json:"source"`
				ID        string `json:"id"`
				PDFURL    string `json:"pdf_url"`
				Title     string `json:"title"`
				OutputDir string `json:"output_dir"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("参数无效: %w", err)
			}
			if strings.TrimSpace(input.PDFURL) == "" && (input.Source == "" || input.ID == "") {
				return nil, fmt.Errorf("需要 pdf_url，或同时给出 source 与 id")
			}
			pdfURL, title, err := t.resolveDownloadTarget(ctx, input.Source, input.ID, input.PDFURL, input.Title)
			if err != nil {
				return nil, err
			}
			absWork, relDir, err := t.resolveOutputDir(input.OutputDir)
			if err != nil {
				return nil, err
			}
			data, err := t.downloader.Fetch(ctx, pdfURL)
			if err != nil {
				return nil, fmt.Errorf("下载 PDF 失败：%w", err)
			}
			root, err := os.OpenRoot(absWork)
			if err != nil {
				return nil, fmt.Errorf("打开工作目录失败：%w", err)
			}
			defer func() { _ = root.Close() }()
			rel, err := literature.SavePDF(root, relDir, title, pdfURL, data)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"file": rel, "bytes": len(data), "pdf_url": pdfURL,
				"rendered": fmt.Sprintf("已下载 %d 字节 PDF → %s（来源：%s）", len(data), rel, pdfURL),
			}, nil
		},
	}
}

// resolveDownloadTarget 得到 PDF 链接与文件名：直链优先，否则按源记录解析。
func (t *paperTools) resolveDownloadTarget(ctx context.Context, source, id, pdfURL, title string) (string, string, error) {
	if trimmed := strings.TrimSpace(pdfURL); trimmed != "" {
		return trimmed, title, nil
	}
	connector, ok := t.registry.Get(source)
	if !ok {
		return "", "", fmt.Errorf("未知文献源 %q（可选 %s）", source, strings.Join(t.registry.SortedIDs(), "/"))
	}
	paper, err := connector.Fetch(ctx, strings.TrimSpace(id))
	if err != nil {
		return "", "", fmt.Errorf("解析文献记录失败：%w", err)
	}
	if paper == nil {
		return "", "", fmt.Errorf("该源没有找到记录 %s", id)
	}
	if title == "" {
		title = paper.Title
	}
	if paper.PDFURL == "" {
		return "", "", fmt.Errorf("该记录没有开放获取 PDF；可改用 pdf_url 或人工获取原文：%s", paper.URL)
	}
	return paper.PDFURL, title, nil
}

// resolveOutputDir 返回工作目录绝对路径与相对它的输出目录：
// 默认 <工作目录>/论文原文/YYYY-MM-DD。符号链接复核是前置校验；
// 真正的写入边界由 os.Root 保证。
func (t *paperTools) resolveOutputDir(outputDir string) (string, string, error) {
	if t.workDir == "" {
		return "", "", fmt.Errorf("未配置工作目录，无法下载论文")
	}
	absWork, err := filepath.Abs(t.workDir)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录失败：%w", err)
	}
	target := strings.TrimSpace(outputDir)
	if target == "" {
		target = filepath.Join("论文原文", time.Now().Format("2006-01-02"))
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(absWork, target)
	}
	target = filepath.Clean(target)
	if !withinDir(absWork, target) {
		return "", "", fmt.Errorf("输出目录必须位于工作目录 %s 内：%s", absWork, outputDir)
	}
	resolvedWork, err := filepath.EvalSymlinks(absWork)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录符号链接失败：%w", err)
	}
	// 目录可能尚不存在：解析最近的已存在祖先，再拼回尾部。
	resolvedTarget, err := resolveExistingPrefix(target)
	if err != nil {
		return "", "", fmt.Errorf("解析输出目录失败：%w", err)
	}
	if !withinDir(resolvedWork, resolvedTarget) {
		return "", "", fmt.Errorf("输出目录经符号链接后越出工作目录：%s", resolvedTarget)
	}
	relDir, err := filepath.Rel(absWork, target)
	if err != nil {
		return "", "", fmt.Errorf("计算相对输出目录失败：%w", err)
	}
	return absWork, relDir, nil
}

// withinDir 判断 target 是否位于 base 之内（含 base 本身）。
func withinDir(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExistingPrefix 从 dir 向上找到第一个存在的路径并解析其符号链接，
// 再把尚不存在的尾部拼回。
func resolveExistingPrefix(dir string) (string, error) {
	current := dir
	var tail []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{resolved}, tail...)...), nil
}
