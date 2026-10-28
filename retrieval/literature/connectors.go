package literature

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xujian519/mady/pkg/hostlimit"
)

// 各源的礼貌限速：arXiv 要求 ≤1 req/3s；keyless Semantic Scholar 约 1s。
var (
	openAlexLimit        = hostlimit.Limit{MinInterval: 100 * time.Millisecond, MaxConcurrent: 4}
	semanticScholarLimit = hostlimit.Limit{MinInterval: time.Second, MaxConcurrent: 2}
	arxivLimit           = hostlimit.Limit{MinInterval: 3 * time.Second, MaxConcurrent: 1}
	crossrefLimit        = hostlimit.Limit{MinInterval: 100 * time.Millisecond, MaxConcurrent: 4}
)

const (
	openAlexBase        = "https://api.openalex.org/works"
	semanticScholarBase = "https://api.semanticscholar.org/graph/v1/paper"
	arxivBase           = "http://export.arxiv.org/api/query"
	crossrefBase        = "https://api.crossref.org/works"
	defaultMailto       = "mady@users.noreply.github.com"
)

func clampLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 25 {
		return 25
	}
	return limit
}

func snippet(text string, limit int) string {
	trimmed := strings.Join(strings.Fields(text), " ")
	runes := []rune(trimmed)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return trimmed
}

// ---------------------------------------------------------------------------
// OpenAlex
// ---------------------------------------------------------------------------

// OpenAlexConnector 是开放学术图谱连接器（免费无 key）。
type OpenAlexConnector struct {
	client *Client
	mailto string
}

// NewOpenAlexConnector 创建连接器；mailto 用于 polite pool，可传空。
func NewOpenAlexConnector(client *Client, mailto string) *OpenAlexConnector {
	if strings.TrimSpace(mailto) == "" {
		mailto = defaultMailto
	}
	return &OpenAlexConnector{client: client, mailto: mailto}
}

// ID 返回源标识 openalex。
func (c *OpenAlexConnector) ID() string { return "openalex" }

// Name 返回展示名。
func (c *OpenAlexConnector) Name() string { return "OpenAlex" }

func (c *OpenAlexConnector) polite() string {
	return "mailto=" + url.QueryEscape(c.mailto)
}

type openAlexWork struct {
	ID                    string           `json:"id"`
	DOI                   string           `json:"doi"`
	Title                 string           `json:"title"`
	DisplayName           string           `json:"display_name"`
	PublicationYear       int              `json:"publication_year"`
	CitedByCount          int              `json:"cited_by_count"`
	RelevanceScore        float64          `json:"relevance_score"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
	Authorships           []struct {
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	PrimaryLocation struct {
		Source struct {
			DisplayName string `json:"display_name"`
		} `json:"source"`
		LandingPageURL string `json:"landing_page_url"`
		PDFURL         string `json:"pdf_url"`
	} `json:"primary_location"`
	BestOALocation struct {
		PDFURL string `json:"pdf_url"`
	} `json:"best_oa_location"`
	OpenAccess struct {
		OAURL string `json:"oa_url"`
	} `json:"open_access"`
}

type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
	Meta    struct {
		Count int `json:"count"`
	} `json:"meta"`
}

// Search 按关键词检索 OpenAlex 作品。
func (c *OpenAlexConnector) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	limit := clampLimit(opts.Limit)
	requestURL := fmt.Sprintf("%s?search=%s&per-page=%d&%s",
		openAlexBase, url.QueryEscape(query), limit, c.polite())
	if opts.YearFrom > 0 {
		requestURL += "&filter=" + url.QueryEscape(fmt.Sprintf("from_publication_date:%d-01-01", opts.YearFrom))
	}
	var response openAlexResponse
	if err := c.client.GetJSON(ctx, requestURL, openAlexLimit, &response); err != nil {
		return nil, err
	}
	papers := make([]Paper, 0, len(response.Results))
	for _, work := range response.Results {
		papers = append(papers, toOpenAlexPaper(work))
	}
	return papers, nil
}

// Fetch 按 work id 或 DOI 取单条记录。
func (c *OpenAlexConnector) Fetch(ctx context.Context, id string) (*Paper, error) {
	path := shortOpenAlexID(id)
	if path == "" {
		return nil, fmt.Errorf("OpenAlex 需要 work id（W…）或 DOI")
	}
	if strings.HasPrefix(path, "10.") {
		path = "doi:" + path
	} else {
		path = url.PathEscape(path)
	}
	var work openAlexWork
	if err := c.client.GetJSON(ctx, openAlexBase+"/"+path+"?"+c.polite(), openAlexLimit, &work); err != nil {
		return nil, err
	}
	paper := toOpenAlexPaper(work)
	return &paper, nil
}

func shortOpenAlexID(id string) string {
	trimmed := strings.TrimSpace(id)
	trimmed = strings.TrimPrefix(trimmed, "https://openalex.org/")
	trimmed = strings.TrimPrefix(trimmed, "http://openalex.org/")
	return trimmed
}

func toOpenAlexPaper(work openAlexWork) Paper {
	authors := make([]string, 0, len(work.Authorships))
	for _, authorship := range work.Authorships {
		authors = append(authors, authorship.Author.DisplayName)
	}
	title := work.DisplayName
	if title == "" {
		title = work.Title
	}
	pdfURL := work.BestOALocation.PDFURL
	if pdfURL == "" {
		pdfURL = work.PrimaryLocation.PDFURL
	}
	if pdfURL == "" {
		pdfURL = work.OpenAccess.OAURL
	}
	landing := work.ID
	if work.PrimaryLocation.LandingPageURL != "" {
		landing = work.PrimaryLocation.LandingPageURL
	}
	return Paper{
		ID: shortOpenAlexID(work.ID), DOI: strings.TrimPrefix(strings.TrimPrefix(work.DOI, "https://doi.org/"), "http://doi.org/"),
		Source: "openalex",
		Title:  snippet(title, 300), Authors: authors, Year: work.PublicationYear,
		Venue:    work.PrimaryLocation.Source.DisplayName,
		Abstract: snippet(fromInverted(work.AbstractInvertedIndex), 1200),
		URL:      landing, PDFURL: pdfURL, CitedBy: work.CitedByCount,
	}
}

// fromInverted 把 OpenAlex 的「词 → 位置」摘要重建为纯文本。
func fromInverted(index map[string][]int) string {
	if len(index) == 0 {
		return ""
	}
	total := 0
	for _, positions := range index {
		for _, position := range positions {
			if position+1 > total {
				total = position + 1
			}
		}
	}
	words := make([]string, total)
	for word, positions := range index {
		for _, position := range positions {
			if position >= 0 && position < total {
				words[position] = word
			}
		}
	}
	return strings.Join(words, " ")
}

// ---------------------------------------------------------------------------
// Semantic Scholar
// ---------------------------------------------------------------------------

// SemanticScholarConnector 使用 Semantic Scholar Graph API。
type SemanticScholarConnector struct {
	client *Client
	apiKey string
}

// NewSemanticScholarConnector 创建连接器；apiKey 可为空。
func NewSemanticScholarConnector(client *Client, apiKey string) *SemanticScholarConnector {
	return &SemanticScholarConnector{client: client, apiKey: apiKey}
}

// ID 返回源标识 semantic-scholar。
func (c *SemanticScholarConnector) ID() string { return "semantic-scholar" }

// Name 返回展示名。
func (c *SemanticScholarConnector) Name() string { return "Semantic Scholar" }

type semanticScholarPaper struct {
	PaperID       string `json:"paperId"`
	Title         string `json:"title"`
	Abstract      string `json:"abstract"`
	Year          int    `json:"year"`
	CitationCount int    `json:"citationCount"`
	URL           string `json:"url"`
	Authors       []struct {
		Name string `json:"name"`
	} `json:"authors"`
	OpenAccessPDF *struct {
		URL string `json:"url"`
	} `json:"openAccessPdf"`
}

// Search 按关键词检索 Semantic Scholar。
func (c *SemanticScholarConnector) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	limit := clampLimit(opts.Limit)
	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("fields", "title,authors,year,abstract,citationCount,url,openAccessPdf")
	if opts.YearFrom > 0 {
		params.Set("year", fmt.Sprintf("%d-", opts.YearFrom))
	}
	var response struct {
		Data []semanticScholarPaper `json:"data"`
	}
	if err := c.client.GetJSON(ctx, semanticScholarBase+"/search?"+params.Encode(), semanticScholarLimit, &response); err != nil {
		return nil, err
	}
	papers := make([]Paper, 0, len(response.Data))
	for _, item := range response.Data {
		papers = append(papers, c.toPaper(item))
	}
	return papers, nil
}

// Fetch 按 paperId 取单条记录。
func (c *SemanticScholarConnector) Fetch(ctx context.Context, id string) (*Paper, error) {
	requestURL := semanticScholarBase + "/" + url.PathEscape(strings.TrimSpace(id)) +
		"?fields=title,authors,year,abstract,citationCount,url,openAccessPdf"
	var item semanticScholarPaper
	if err := c.client.GetJSON(ctx, requestURL, semanticScholarLimit, &item); err != nil {
		return nil, err
	}
	paper := c.toPaper(item)
	return &paper, nil
}

func (c *SemanticScholarConnector) toPaper(item semanticScholarPaper) Paper {
	authors := make([]string, 0, len(item.Authors))
	for _, author := range item.Authors {
		authors = append(authors, author.Name)
	}
	paper := Paper{
		ID: item.PaperID, Source: "semantic-scholar", Title: snippet(item.Title, 300),
		Authors: authors, Year: item.Year, Abstract: snippet(item.Abstract, 1200),
		URL: item.URL, CitedBy: item.CitationCount,
	}
	if item.OpenAccessPDF != nil {
		paper.PDFURL = item.OpenAccessPDF.URL
	}
	return paper
}

// ---------------------------------------------------------------------------
// arXiv
// ---------------------------------------------------------------------------

// ArxivConnector 使用 arXiv Atom API。
type ArxivConnector struct{ client *Client }

// NewArxivConnector 创建连接器。
func NewArxivConnector(client *Client) *ArxivConnector { return &ArxivConnector{client: client} }

// ID 返回源标识 arxiv。
func (c *ArxivConnector) ID() string { return "arxiv" }

// Name 返回展示名。
func (c *ArxivConnector) Name() string { return "arXiv" }

type arxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Published string `xml:"published"`
	Authors   []struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Links []struct {
		Rel   string `xml:"rel,attr"`
		Title string `xml:"title,attr"`
		Href  string `xml:"href,attr"`
	} `xml:"link"`
}

// Search 按关键词检索 arXiv 预印本。
func (c *ArxivConnector) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	limit := clampLimit(opts.Limit)
	requestURL := fmt.Sprintf("%s?search_query=%s&start=0&max_results=%d&sortBy=relevance",
		arxivBase, url.QueryEscape("all:"+query), limit)
	body, err := c.client.GetText(ctx, requestURL, arxivLimit)
	if err != nil {
		return nil, err
	}
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("arXiv 返回非 Atom XML：%w", err)
	}
	papers := make([]Paper, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		papers = append(papers, toArxivPaper(entry))
	}
	return papers, nil
}

// Fetch 按 arXiv ID 取单条记录。
func (c *ArxivConnector) Fetch(ctx context.Context, id string) (*Paper, error) {
	requestURL := fmt.Sprintf("%s?id_list=%s", arxivBase, url.QueryEscape(strings.TrimSpace(id)))
	body, err := c.client.GetText(ctx, requestURL, arxivLimit)
	if err != nil {
		return nil, err
	}
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("arXiv 返回非 Atom XML：%w", err)
	}
	if len(feed.Entries) == 0 {
		return nil, nil
	}
	paper := toArxivPaper(feed.Entries[0])
	return &paper, nil
}

func toArxivPaper(entry arxivEntry) Paper {
	authors := make([]string, 0, len(entry.Authors))
	for _, author := range entry.Authors {
		authors = append(authors, author.Name)
	}
	year := 0
	if len(entry.Published) >= 4 {
		year, _ = strconv.Atoi(entry.Published[:4])
	}
	paper := Paper{
		ID: entry.ID, Source: "arxiv", Title: snippet(entry.Title, 300),
		Authors: authors, Year: year, Abstract: snippet(entry.Summary, 1200), URL: entry.ID,
	}
	for _, link := range entry.Links {
		if link.Title == "pdf" || strings.HasSuffix(link.Href, ".pdf") {
			paper.PDFURL = link.Href
			break
		}
	}
	return paper
}

// ---------------------------------------------------------------------------
// Crossref
// ---------------------------------------------------------------------------

// CrossrefConnector 使用 Crossref REST API。
type CrossrefConnector struct{ client *Client }

// NewCrossrefConnector 创建连接器。
func NewCrossrefConnector(client *Client) *CrossrefConnector {
	return &CrossrefConnector{client: client}
}

// ID 返回源标识 crossref。
func (c *CrossrefConnector) ID() string { return "crossref" }

// Name 返回展示名。
func (c *CrossrefConnector) Name() string { return "Crossref" }

var xmlTagPattern = regexp.MustCompile(`<[^>]+>`)

type crossrefItem struct {
	DOI       string   `json:"DOI"`
	Title     []string `json:"title"`
	Abstract  string   `json:"abstract"`
	Container []string `json:"container-title"`
	URL       string   `json:"URL"`
	Author    []struct {
		Given  string `json:"given"`
		Family string `json:"family"`
	} `json:"author"`
	Issued struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"issued"`
	Link []struct {
		URL         string `json:"URL"`
		ContentType string `json:"content-type"`
	} `json:"link"`
}

// Search 按关键词检索 Crossref 元数据。
func (c *CrossrefConnector) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	limit := clampLimit(opts.Limit)
	requestURL := fmt.Sprintf("%s?query=%s&rows=%d", crossrefBase, url.QueryEscape(query), limit)
	if opts.YearFrom > 0 {
		requestURL += "&filter=" + url.QueryEscape(fmt.Sprintf("from-pub-date:%d-01-01", opts.YearFrom))
	}
	var response struct {
		Message struct {
			Items []crossrefItem `json:"items"`
		} `json:"message"`
	}
	if err := c.client.GetJSON(ctx, requestURL, crossrefLimit, &response); err != nil {
		return nil, err
	}
	papers := make([]Paper, 0, len(response.Message.Items))
	for _, item := range response.Message.Items {
		papers = append(papers, toCrossrefPaper(item))
	}
	return papers, nil
}

// Fetch 按 DOI 取单条记录。
func (c *CrossrefConnector) Fetch(ctx context.Context, id string) (*Paper, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(id), "https://doi.org/")
	var item crossrefItem
	if err := c.client.GetJSON(ctx, crossrefBase+"/"+url.PathEscape(trimmed), crossrefLimit, &item); err != nil {
		return nil, err
	}
	paper := toCrossrefPaper(item)
	return &paper, nil
}

func toCrossrefPaper(item crossrefItem) Paper {
	authors := make([]string, 0, len(item.Author))
	for _, author := range item.Author {
		name := strings.TrimSpace(author.Given + " " + author.Family)
		if name != "" {
			authors = append(authors, name)
		}
	}
	year := 0
	if len(item.Issued.DateParts) > 0 && len(item.Issued.DateParts[0]) > 0 {
		year = item.Issued.DateParts[0][0]
	}
	title := ""
	if len(item.Title) > 0 {
		title = item.Title[0]
	}
	venue := ""
	if len(item.Container) > 0 {
		venue = item.Container[0]
	}
	paper := Paper{
		ID: item.DOI, DOI: item.DOI, Source: "crossref", Title: snippet(title, 300),
		Authors: authors, Year: year, Venue: venue,
		Abstract: snippet(xmlTagPattern.ReplaceAllString(item.Abstract, " "), 1200),
		URL:      item.URL,
	}
	for _, link := range item.Link {
		if strings.Contains(link.ContentType, "pdf") {
			paper.PDFURL = link.URL
			break
		}
	}
	if paper.URL == "" && item.DOI != "" {
		paper.URL = "https://doi.org/" + item.DOI
	}
	return paper
}

// ---------------------------------------------------------------------------
// 注册表
// ---------------------------------------------------------------------------

// Registry 持有全部连接器。
type Registry struct {
	order      []string
	connectors map[string]Connector
}

// NewRegistry 创建默认注册表（OpenAlex / Semantic Scholar / arXiv / Crossref）。
func NewRegistry(client *Client, semanticScholarAPIKey, mailto string) *Registry {
	registry := &Registry{connectors: make(map[string]Connector, 4)}
	registry.register(NewOpenAlexConnector(client, mailto))
	registry.register(NewSemanticScholarConnector(client, semanticScholarAPIKey))
	registry.register(NewArxivConnector(client))
	registry.register(NewCrossrefConnector(client))
	return registry
}

func (r *Registry) register(connector Connector) {
	r.order = append(r.order, connector.ID())
	r.connectors[connector.ID()] = connector
}

// Get 按 ID 取连接器。
func (r *Registry) Get(id string) (Connector, bool) {
	connector, ok := r.connectors[id]
	return connector, ok
}

// Sources 返回源清单（按注册顺序）。
func (r *Registry) Sources() []SourceInfo {
	descriptions := map[string]SourceInfo{
		"openalex":         {Description: "开放学术图谱（作品/作者/机构/概念），无 key 可用", Homepage: "https://openalex.org"},
		"semantic-scholar": {Description: "语义学者图（引用数、开放获取 PDF）", Homepage: "https://www.semanticscholar.org"},
		"arxiv":            {Description: "预印本文库（物理/计算机/数学等）", Homepage: "https://arxiv.org"},
		"crossref":         {Description: "DOI 注册机构元数据（期刊/会议论文）", Homepage: "https://www.crossref.org"},
	}
	sources := make([]SourceInfo, 0, len(r.order))
	for _, id := range r.order {
		connector := r.connectors[id]
		info := descriptions[id]
		info.ID = id
		info.Name = connector.Name()
		sources = append(sources, info)
	}
	return sources
}

// SortedIDs 返回按字典序排序的源 ID（便于稳定输出）。
func (r *Registry) SortedIDs() []string {
	ids := append([]string(nil), r.order...)
	sort.Strings(ids)
	return ids
}
