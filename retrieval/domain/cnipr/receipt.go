package cnipr

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// 纯函数层：号码分类与规范化、脚本收据解析、页面文本解析。
// 脚本只驱动页面并回传原文；所有判定都在这里，便于单测覆盖。

// SiteBaseURL 是 CNIPR 站点根地址（pic.cnipr.com 仅 HTTP 可达）。
const SiteBaseURL = "http://search.cnipr.com"

// 页面模式。Go RE2 没有前瞻，需要「到下一个标签为止」的字段先按标签切段再解析。
var (
	pdfURLPattern          = regexp.MustCompile(`pic\.cnipr\.com/pdfcn/[^"'<>\s]+\.PDF`)
	legalTotalPattern      = regexp.MustCompile(`共\s*(\d+)\s*条记录`)
	legalRecordHeadPattern = regexp.MustCompile(`^((?:CN)?\d{12}\.[0-9A-Z])\s+法律状态公告日[：:]\s*([\d.]+)\s+法律状态[：:]\s*(\S+)\s+法律状态信息[：:]\s*([\s\S]*)$`)
	searchTotalPattern     = regexp.MustCompile(`全部\s*(\d+)\s*件`)
	searchPubNoPattern     = regexp.MustCompile(`公开\(公告\)号[：:]\s*(\S+)`)
	searchDatePattern      = regexp.MustCompile(`公开\(公告\)日[：:]\s*([\d.]+)`)
	pubNumberPattern       = regexp.MustCompile(`^(?:CN|ZL)\d{7,}[A-Z]$`)
	appNumberPattern       = regexp.MustCompile(`^(?:CN)?\d{12}\.[0-9A-Z]$`)
	bareApp13Pattern       = regexp.MustCompile(`^\d{13}$`)
	bareApp12APattern      = regexp.MustCompile(`^\d{12}[A-Z]$`)
)

// 检索分页常量（站点账号等级决定每页 10 条）。
const (
	SearchPageSize = 10
	SearchMaxHits  = 30
)

// NormalizeAppNumber 把申请号整成 CNIPR 认的「校验位前带小数点」写法。
func NormalizeAppNumber(raw string) string {
	trimmed := strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw))
	if strings.Contains(trimmed, ".") {
		return trimmed
	}
	bare := strings.TrimPrefix(trimmed, "CN")
	if bareApp13Pattern.MatchString(bare) || bareApp12APattern.MatchString(bare) {
		dotted := bare[:12] + "." + bare[12:]
		if strings.HasPrefix(trimmed, "CN") {
			return "CN" + dotted
		}
		return dotted
	}
	return trimmed
}

// ClassifyNumber 判号码形态：app（申请号）/ pub（公开公告号）/ other。
func ClassifyNumber(raw string) string {
	normalized := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if pubNumberPattern.MatchString(normalized) {
		return "pub"
	}
	if appNumberPattern.MatchString(NormalizeAppNumber(normalized)) {
		return "app"
	}
	return "other"
}

// NormalizePDFURLs 把页面抓到的直链整成可 GET 的 http 直链并去重。
func NormalizePDFURLs(matches []string) []string {
	seen := make(map[string]bool, len(matches))
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(match, "https://"), "http://")
		url := "http://" + trimmed
		if seen[url] {
			continue
		}
		seen[url] = true
		urls = append(urls, url)
	}
	return urls
}

// ParseLegalRecords 解析法律状态结果页文本：total 取「共 N 条记录」，
// 零命中页不输出该标记，此时为 nil（不编造 0）。
func ParseLegalRecords(text string) (*int, []LegalRecord) {
	folded := strings.Join(strings.Fields(text), " ")
	var total *int
	if match := legalTotalPattern.FindStringSubmatch(folded); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			total = &value
		}
	}
	var records []LegalRecord
	for _, segment := range splitLegalSegments(folded) {
		match := legalRecordHeadPattern.FindStringSubmatch(segment)
		if match == nil {
			continue
		}
		records = append(records, LegalRecord{
			AppNumber: match[1], AnnounceDate: match[2],
			Status: match[3], StatusInfo: strings.TrimSpace(match[4]),
		})
	}
	return total, records
}

// splitLegalSegments 按「申请号：」切分记录段：RE2 没有前瞻，切分后再逐段解析，
// 段尾的「共 N 条记录 / 知识产权出版社」在解析前截掉。
func splitLegalSegments(folded string) []string {
	normalized := strings.ReplaceAll(folded, "申请号:", "申请号：")
	parts := strings.Split(normalized, "申请号：")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		for _, marker := range []string{"共 ", "知识产权出版社"} {
			if at := strings.Index(trimmed, marker); at >= 0 {
				trimmed = strings.TrimSpace(trimmed[:at])
			}
		}
		if trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	return segments
}

// PlanPDFRequests 规划 PDF 直链请求：只处理公开(公告)号，其余按条目报错。
func PlanPDFRequests(numbers []string) ([]string, []PdfLinkItem) {
	var requested []string
	var unsupported []PdfLinkItem
	for _, number := range numbers {
		if ClassifyNumber(number) == "pub" {
			requested = append(requested, number)
			continue
		}
		unsupported = append(unsupported, PdfLinkItem{
			Number: number, URLs: []string{}, Error: "CNIPR 通道只处理公开(公告)号",
		})
	}
	return requested, unsupported
}

// PlanLegalQueries 规划法律状态查询：申请号直接查，公开号先解析申请号。
func PlanLegalQueries(numbers []string) ([]LegalQuery, []LegalStatusItem) {
	var queries []LegalQuery
	var unsupported []LegalStatusItem
	for _, number := range numbers {
		switch ClassifyNumber(number) {
		case "app":
			queries = append(queries, LegalQuery{Number: number, AppNumber: NormalizeAppNumber(number)})
		case "pub":
			queries = append(queries, LegalQuery{Number: number, PubNumber: number})
		default:
			unsupported = append(unsupported, LegalStatusItem{
				Number: number, Records: []LegalRecord{},
				Error: "CNIPR 通道只处理中国申请号与公开(公告)号",
			})
		}
	}
	return queries, unsupported
}

// PlanSearchRequest 把期望条数收敛到通道能服务的范围并换算页数。
func PlanSearchRequest(limit int) SearchRequest {
	wanted := limit
	if wanted <= 0 {
		wanted = SearchPageSize
	}
	normalized := min(wanted, SearchMaxHits)
	return SearchRequest{Limit: normalized, Pages: (normalized + SearchPageSize - 1) / SearchPageSize}
}

// NormalizeDate 把页面公开日归一为 YYYY-MM-DD；非该形态原样返回。
func NormalizeDate(raw string) string {
	trimmed := strings.TrimSpace(raw)
	match := regexp.MustCompile(`^(\d{4})[./-](\d{2})[./-](\d{2})$`).FindStringSubmatch(trimmed)
	if match == nil {
		return trimmed
	}
	return match[1] + "-" + match[2] + "-" + match[3]
}

// ParseSearchTotal 解析结果页「全部 N 件专利」的总数。
func ParseSearchTotal(header string) (int, bool) {
	folded := strings.Join(strings.Fields(header), " ")
	match := searchTotalPattern.FindStringSubmatch(folded)
	if match == nil {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	return value, err == nil
}

// parseSearchRow 从结果行 innerText 取带标签的字段；缺哪个就留空，不拿行内其它文本顶替。
func parseSearchRow(text string) (number, date, assignee, abstract string) {
	if match := searchPubNoPattern.FindStringSubmatch(text); match != nil {
		number = strings.TrimSpace(match[1])
	}
	if match := searchDatePattern.FindStringSubmatch(text); match != nil {
		date = strings.TrimSpace(match[1])
	}
	assignee = fieldBeforeNextLabel(text, "申请(专利权)人", []string{"分类号", "优先权", "摘要", "发明(设计)人"})
	abstract = fieldToEnd(text, "摘要")
	return number, date, assignee, abstract
}

// fieldBeforeNextLabel 取「标签：」到下一个标签之间的文本（RE2 无前瞻，用 Index 切）。
func fieldBeforeNextLabel(text, label string, nextLabels []string) string {
	for _, separator := range []string{"：", ":"} {
		at := strings.Index(text, label+separator)
		if at < 0 {
			continue
		}
		segment := text[at+len(label)+len(separator):]
		end := len(segment)
		for _, next := range nextLabels {
			for _, nextSeparator := range []string{"：", ":"} {
				if index := strings.Index(segment, next+nextSeparator); index >= 0 && index < end {
					end = index
				}
			}
		}
		return strings.TrimSpace(segment[:end])
	}
	return ""
}

// fieldToEnd 取「标签：」到文本末尾的内容，并剥掉尾部的「机器翻译」标记。
func fieldToEnd(text, label string) string {
	for _, separator := range []string{"：", ":"} {
		at := strings.Index(text, label+separator)
		if at < 0 {
			continue
		}
		return strings.TrimSpace(strings.TrimSuffix(
			strings.TrimSpace(text[at+len(label)+len(separator):]), "机器翻译"))
	}
	return ""
}

// excerpt 截取诊断用的页面原文片段。
func excerpt(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 60 {
		runes = runes[:60]
	}
	return string(runes)
}

// readSearchPayload 从脚本收据读出总数与命中；通道级失败以 error 返回。
type scriptFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func readSearchPayload(payload map[string]any) (int, []SearchHit, *scriptFailure) {
	if failure := readFailure(payload); failure != nil {
		return 0, nil, failure
	}
	header, _ := payload["header"].(string)
	total, ok := ParseSearchTotal(header)
	if !ok {
		// 保留页面原文供诊断：可能是站点改版，也可能是页面没渲染出总数标记。
		return 0, nil, &scriptFailure{Code: FailurePageState,
			Message: "CNIPR 检索结果页未给出「全部 N 件专利」总数：页面结构可能已变化（header=" + excerpt(header) + "）"}
	}
	items, _ := payload["items"].([]any)
	hits := make([]SearchHit, 0, len(items))
	for _, raw := range items {
		record, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		text, _ := record["text"].(string)
		number, date, assignee, abstract := parseSearchRow(text)
		if pageNumber, _ := record["number"].(string); number == "" {
			number = pageNumber
		}
		cursor, _ := record["cursor"].(float64)
		title, _ := record["title"].(string)
		hits = append(hits, SearchHit{
			Cursor: int(cursor), Number: number, Title: title,
			PublicationDate: NormalizeDate(date), Assignee: assignee, Abstract: abstract,
		})
	}
	return total, hits, nil
}

func readPdfPayload(payload map[string]any) ([]PdfLinkItem, *scriptFailure) {
	if failure := readFailure(payload); failure != nil {
		return nil, failure
	}
	items, _ := payload["items"].([]any)
	out := make([]PdfLinkItem, 0, len(items))
	for _, raw := range items {
		record, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		number, _ := record["number"].(string)
		item := PdfLinkItem{Number: number, URLs: []string{}}
		if message, _ := record["error"].(string); message != "" {
			item.Error = message
			out = append(out, item)
			continue
		}
		if matches, ok := record["matches"].([]any); ok {
			rawMatches := make([]string, 0, len(matches))
			for _, match := range matches {
				if text, ok := match.(string); ok {
					rawMatches = append(rawMatches, text)
				}
			}
			item.URLs = NormalizePDFURLs(rawMatches)
		}
		item.Unexamined, _ = record["isUnexamined"].(bool)
		item.Authorized, _ = record["isAuthorized"].(bool)
		out = append(out, item)
	}
	return out, nil
}

func readLegalPayload(payload map[string]any) ([]LegalStatusItem, *scriptFailure) {
	if failure := readFailure(payload); failure != nil {
		return nil, failure
	}
	items, _ := payload["items"].([]any)
	out := make([]LegalStatusItem, 0, len(items))
	for _, raw := range items {
		record, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		number, _ := record["number"].(string)
		item := LegalStatusItem{Number: number, Records: []LegalRecord{}}
		if message, _ := record["error"].(string); message != "" {
			item.Error = message
			out = append(out, item)
			continue
		}
		item.AppNumber, _ = record["appNumber"].(string)
		item.ResolvedFrom, _ = record["resolvedFrom"].(string)
		text, _ := record["text"].(string)
		item.Total, item.Records = ParseLegalRecords(text)
		out = append(out, item)
	}
	return out, nil
}

func readFailure(payload map[string]any) *scriptFailure {
	raw, ok := payload["error"].(map[string]any)
	if !ok {
		return nil
	}
	code, _ := raw["code"].(string)
	message, _ := raw["message"].(string)
	if code == "" {
		code = FailurePageState
	}
	return &scriptFailure{Code: code, Message: message}
}

// alignPDF 把请求号与回报条目按号对齐；未回报的号按缺失处理，不静默丢弃。
func alignPDF(numbers []string, reported []PdfLinkItem) []PdfLinkItem {
	byNumber := make(map[string]PdfLinkItem, len(reported))
	for _, item := range reported {
		byNumber[item.Number] = item
	}
	out := make([]PdfLinkItem, 0, len(numbers))
	for _, number := range numbers {
		if item, ok := byNumber[number]; ok {
			out = append(out, item)
			continue
		}
		out = append(out, PdfLinkItem{Number: number, URLs: []string{}, Error: "脚本未回报该号"})
	}
	return out
}

func alignLegal(numbers []string, reported []LegalStatusItem) []LegalStatusItem {
	byNumber := make(map[string]LegalStatusItem, len(reported))
	for _, item := range reported {
		byNumber[item.Number] = item
	}
	out := make([]LegalStatusItem, 0, len(numbers))
	for _, number := range numbers {
		if item, ok := byNumber[number]; ok {
			out = append(out, item)
			continue
		}
		out = append(out, LegalStatusItem{Number: number, Records: []LegalRecord{}, Error: "脚本未回报该号"})
	}
	return out
}

// ExtractTaggedJSON 从 ego 输出中取 `EGO_<tag>:` 后的 JSON 对象。
// 输出可能先有其他 console 内容，故取最后一个带标签的行。
func ExtractTaggedJSON(output, tag string) (map[string]any, bool) {
	marker := "EGO_" + tag + ":"
	index := strings.LastIndex(output, marker)
	if index < 0 {
		return nil, false
	}
	rest := output[index+len(marker):]
	start := strings.Index(rest, "{")
	if start < 0 {
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(rest[start:]))
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, false
	}
	return payload, true
}
