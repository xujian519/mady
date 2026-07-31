package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/retrieval/domain"
)

// BrowserRetriever 通过 ego-browser CLI 驱动真实浏览器访问在线专利数据库，
// 实现 domain.DomainRetriever。每个实例绑定一个数据源（Google Patents /
// CNIPA / Espacenet），Search 与 GetDocument 各自发起一次独立的
// `ego-browser nodejs` heredoc 调用（进程退出后无状态，task space 隔离）。
//
// ego-browser 不可用时工厂函数返回 nil，调用方应将其视为"该数据源禁用"。
type BrowserRetriever struct {
	cfg BrowserRetrieverConfig
	src dataSource

	// seq 用于生成唯一的 task space 名称，避免并发调用相互干扰。
	seq atomic.Int64
}

// dataSource 描述一个在线专利数据库的访问参数与页面提取脚本。
type dataSource struct {
	// sourceName 是 SourceName() 返回值，如 "Google Patents (via ego-browser)"。
	sourceName string
	// taskSpace 是 task space 名称前缀，如 "mady-gp"。
	taskSpace string
	// searchURL 构造搜索结果页 URL（query 为已编码的完整查询串）。
	searchURL func(query string, maxResults int) string
	// searchPre 是提取前的可选前置步骤（如滚动触发渲染），可为空。
	searchPre string
	// searchJS 是搜索页内的提取代码；占位符 ${max} 在执行前替换为最大条数。
	// 返回 JSON 数组，元素含 title/number/meta/dateLine/abstract/pdfUrl/itemId。
	searchJS string
	// detailURL 构造详情页 URL。
	detailURL func(patentNumber string) string
	// detailPre 是提取前的可选前置步骤（如滚动触发懒加载），可为空。
	detailPre string
	// detailJS 是详情页内的提取代码，返回 JSON 对象（title/number/abstract/claims/description）。
	detailJS string
}

// 编译期接口合规检查。
var _ domain.DomainRetriever = (*BrowserRetriever)(nil)

// NewBrowserRetriever 用给定的数据源构造检索器。ego-browser 不可用时返回
// nil 并记录警告（静默降级，不阻塞启动）。
func NewBrowserRetriever(cfg BrowserRetrieverConfig, src dataSource) *BrowserRetriever {
	if !cfg.IsAvailable() {
		slog.Warn("browser retriever: ego-browser 不可用，禁用数据源", "source", src.sourceName)
		return nil
	}
	// 仅校验路径存在性与可执行性，不探测版本（ego-browser 首次调用即报错，
	// 由调用方降级处理；SKILL.md 亦规定不预检环境）。
	if st, err := os.Stat(cfg.EgoBrowserPath); err != nil || st.IsDir() {
		slog.Warn("browser retriever: ego-browser 不存在，禁用数据源",
			"path", cfg.EgoBrowserPath, "source", src.sourceName)
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	return &BrowserRetriever{cfg: cfg, src: src}
}

// SourceName 返回数据源的易读名称。
func (r *BrowserRetriever) SourceName() string {
	return r.src.sourceName
}

// Search 在在线专利数据库执行关键词检索，返回归一化后的 DomainResults。
// 结果按页面相关度排序，Score 由排名位置映射到 (0,1]。
func (r *BrowserRetriever) Search(ctx context.Context, query domain.DomainQuery) (*domain.DomainResults, error) {
	terms := buildSearchTerms(query)
	if len(terms) == 0 {
		return &domain.DomainResults{Query: query, Source: r.SourceName()}, nil
	}
	topK := query.MaxResults
	if topK <= 0 {
		topK = 10
	}

	// 以自然语言查询为主串，附加关键词拼接（各数据源对布尔语法支持不同，
	// 空格连接在最坏情况下等价于 AND 语义的词组检索）。
	q := strings.Join(terms, " ")
	script := r.buildSearchScript(q, topK)

	out, err := r.callEgoBrowser(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", r.SourceName(), err)
	}

	var raw []map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("%s search: 解析结果失败: %w (输出: %.300s)", r.SourceName(), err, out)
	}

	docs := make([]domain.DomainDocument, 0, len(raw))
	for i, item := range raw {
		doc := r.mapSearchItem(item, i, len(raw))
		if doc.ID == "" {
			continue
		}
		docs = append(docs, doc)
	}
	if len(docs) > topK {
		docs = docs[:topK]
	}
	return &domain.DomainResults{
		Query:      query,
		Documents:  docs,
		TotalCount: len(docs),
		Source:     r.SourceName(),
	}, nil
}

// GetDocument 打开专利详情页并提取标题/摘要/权利要求/说明书全文。
// docID 为专利号（如 "CN110515732B"）或完整 URL。未找到时返回 nil（
// 与 DomainRetriever 约定一致：缺失文档是正常"未命中"而非失败）。
func (r *BrowserRetriever) GetDocument(ctx context.Context, docID string) (*domain.DomainDocument, error) {
	if docID == "" {
		return nil, nil
	}
	script := r.buildGetDocScript(docID)

	out, err := r.callEgoBrowser(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("%s get document %q: %w", r.SourceName(), docID, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("%s get document %q: 解析结果失败: %w (输出: %.300s)", r.SourceName(), docID, err, out)
	}

	number, _ := raw["number"].(string)
	title, _ := raw["title"].(string)
	abstract, _ := raw["abstract"].(string)
	claims, _ := raw["claims"].(string)
	description, _ := raw["description"].(string)
	if title == "" && abstract == "" {
		return nil, nil
	}

	content := strings.TrimSpace(strings.Join([]string{abstract, claims, description}, "\n"))
	doc := &domain.DomainDocument{
		ID:       normalizePatentNumber(number, docID),
		Title:    strings.TrimSpace(title),
		Snippet:  truncate(abstract, 300),
		Content:  content,
		URL:      r.src.detailURL(docID),
		Metadata: map[string]string{"source": r.SourceName()},
	}
	return doc, nil
}

// callEgoBrowser 执行一次 heredoc 调用：将脚本写入 stdin，读取 stdout。
// stdout 应为 cliLog 输出的 JSON；非 JSON 内容（如 CLI 辅助输出）会被剥离。
func (r *BrowserRetriever) callEgoBrowser(ctx context.Context, script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.cfg.EgoBrowserPath, "nodejs") //nolint:gosec // G204: ego-browser 路径来自配置
	cmd.Stdin = strings.NewReader(script)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ego-browser 执行失败: %w (stderr: %.300s)", err, stderr.String())
	}
	// cliLog 输出写入 stderr；stdout 可能为空或仅含辅助输出，两侧都尝试提取。
	out := trimToJSON(stdout.Bytes())
	if len(out) == 0 {
		out = trimToJSON(stderr.Bytes())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ego-browser 无输出 (stderr: %.300s)", stderr.String())
	}
	return out, nil
}

// buildSearchScript 生成一次搜索调用的 heredoc 脚本。
func (r *BrowserRetriever) buildSearchScript(query string, maxResults int) string {
	js := strings.ReplaceAll(r.src.searchJS, "${max}", strconv.Itoa(maxResults))
	space := fmt.Sprintf("%s-%d", r.src.taskSpace, r.seq.Add(1))
	url := r.src.searchURL(query, maxResults)
	return fmt.Sprintf(heredocTemplate, space, url, r.src.searchPre, escapeJSTemplate(js))
}

// buildGetDocScript 生成一次详情页提取的 heredoc 脚本。
func (r *BrowserRetriever) buildGetDocScript(docID string) string {
	space := fmt.Sprintf("%s-%d", r.src.taskSpace, r.seq.Add(1))
	url := r.src.detailURL(docID)
	return fmt.Sprintf(heredocTemplate, space, url, r.src.detailPre, escapeJSTemplate(r.src.detailJS))
}

// escapeJSTemplate 将 JS 提取代码中的反斜杠加倍，使其嵌入模板字面量
// （js(`...`)）后语义不变：提取代码内的 `\n` 等在模板字面量中需写作 `\\n`。
func escapeJSTemplate(code string) string {
	return strings.ReplaceAll(code, `\`, `\\`)
}

// heredocTemplate 是所有调用共享的脚本骨架：创建 task space → 打开页面 →
// 可选滚动（触发懒加载）→ 提取 → cliLog 输出 JSON → 清理 task space。
// 参数依次为 task space 名、目标 URL、前置步骤（滚动代码，可为空）、
// 页面内提取代码。提取代码经模板字面量传给 js()，故其中不得包含反引号
// 或未转义的 ${（占位符 ${max} 在嵌入前已替换为数字）。
const heredocTemplate = "const task = await useOrCreateTaskSpace('%s')\n" +
	"try {\n" +
	"  await openOrReuseTab('%s', { wait: true, timeout: 30 })\n" +
	"  await waitForLoad()\n" +
	"  await wait(2)\n" +
	"  %s\n" +
	"  const data = await js(`%s`)\n" +
	"  cliLog(JSON.stringify(data))\n" +
	"} finally {\n" +
	"  try { await completeTaskSpace(task.id, { keep: false }) } catch {}\n" +
	"}\n"

// mapSearchItem 将搜索页提取的原始条目映射为 DomainDocument。
// i/total 为结果在页面上的排名，映射到 (0,1] 作为相关度分数。
func (r *BrowserRetriever) mapSearchItem(item map[string]any, i, total int) domain.DomainDocument {
	str := func(k string) string { s, _ := item[k].(string); return strings.TrimSpace(s) }
	number := normalizePatentNumber(str("number"), "")
	if number == "" {
		// Espacenet 结果无独立 number 字段，专利号内嵌在 subtitle 中。
		number = extractPubNumber(str("subtitle"))
	}
	if number == "" {
		return domain.DomainDocument{}
	}
	meta := parseMetaLine(str("meta"))
	dates := str("dateLine")
	abstract := str("abstract")
	if abstract == "" {
		abstract = meta.number
	}
	score := 1.0
	if total > 1 {
		score = 1.0 - float64(i)/float64(total)
	}
	url := str("url")
	if url == "" || strings.HasPrefix(url, "https://patents.google.com/?") {
		url = r.src.detailURL(number)
	}
	return domain.DomainDocument{
		ID:      number,
		Title:   firstNonEmpty(str("title"), number),
		Snippet: truncate(abstract, 300),
		Content: abstract,
		URL:     url,
		Metadata: map[string]string{
			"source":    r.SourceName(),
			"country":   meta.country,
			"inventors": meta.inventors,
			"assignee":  meta.assignee,
			"dates":     dates,
			"pdf_url":   str("pdfUrl"),
			"item_id":   str("itemId"),
			"position":  strconv.Itoa(i + 1),
		},
		Score: score,
	}
}

// extractPubNumber 从副标题行提取公开号（"CN107891199A (B) • 2018-04-10" →
// "CN107891199A"）。扫描所有 token 匹配公开号模式（两字母国家码+数字+可选种类）。
func extractPubNumber(subtitle string) string {
	for t := range strings.FieldsSeq(subtitle) {
		if espacenetPubRe.MatchString(strings.ToUpper(t)) {
			return strings.ToUpper(t)
		}
	}
	return ""
}

// meta 是搜索结果 metadata 行的解析结果（"CN • CN110515732B • 发明人 • 权利人"）。
type meta struct {
	country   string
	number    string
	inventors string
	assignee  string
}

// parseMetaLine 解析搜索结果 metadata 行。页面 innerText 中 bullet 渲染为
// 空格，实际形如 "CN CN106599773B 马惠敏 清华大学" 或同族多国
// "WO CN CN109964446B 李挥 北京大学深圳研究生院"；兼容 "•"/"·" 分隔。
// 规则：公开号（两字母国家码+数字）前的 token 为国别列表，其后第一个
// token 为发明人，剩余为权利人。
func parseMetaLine(line string) meta {
	var parts []string
	for _, sep := range []string{"•", "·"} {
		parts = parts[:0]
		for p := range strings.SplitSeq(line, sep) {
			if t := strings.TrimSpace(p); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) >= 2 {
			break
		}
	}
	if len(parts) == 0 {
		return meta{}
	}

	// 传统分隔格式（含两个以上部分）且第二部分即公开号。
	if len(parts) >= 2 && espacenetPubRe.MatchString(strings.ToUpper(parts[1])) {
		m := meta{country: parts[0], number: strings.ToUpper(parts[1])}
		if len(parts) > 2 {
			m.inventors = parts[2]
		}
		if len(parts) > 3 {
			m.assignee = strings.Join(parts[3:], ", ")
		}
		return m
	}

	// 空格分隔格式（innerText 中 bullet 渲染为空格）：
	// 按空格重新切分，公开号前的 token 均为国别（同族可能多国）。
	fields := strings.Fields(line)
	numberIdx := -1
	for i, t := range fields {
		if espacenetPubRe.MatchString(strings.ToUpper(t)) {
			numberIdx = i
			break
		}
	}
	if numberIdx < 0 {
		m := meta{country: fields[0]}
		if len(fields) > 1 {
			m.number = fields[1]
		}
		return m
	}
	m := meta{
		country: strings.Join(fields[:numberIdx], ","),
		number:  strings.ToUpper(fields[numberIdx]),
	}
	if numberIdx+1 < len(fields) {
		m.inventors = fields[numberIdx+1]
	}
	if numberIdx+2 < len(fields) {
		m.assignee = strings.Join(fields[numberIdx+2:], " ")
	}
	return m
}

// normalizePatentNumber 将 citation 格式（"CN:106599773:B"）与原始专利号
// 归一化为紧凑格式（"CN106599773B"）。归一化失败时返回原始值。
func normalizePatentNumber(number, fallback string) string {
	number = strings.TrimSpace(number)
	if number == "" {
		return fallback
	}
	compact := strings.NewReplacer(":", "", " ", "", "\t", "").Replace(number)
	if compact == "" {
		return fallback
	}
	return compact
}

// trimToJSON 从 stdout 中提取 JSON：先尝试整体解析，失败则逐行向后扫描，
// 优先取以 [ 开头的行（搜索数组），其次取以 { 开头的行（详情对象），
// 剥离 ego-browser 可能的辅助输出。
func trimToJSON(out []byte) []byte {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil
	}
	if out[0] == '[' || out[0] == '{' {
		return out
	}
	var obj []byte
	lines := bytes.Split(out, []byte("\n"))
	for i, line := range lines {
		t := bytes.TrimSpace(line)
		if len(t) == 0 {
			continue
		}
		// 数组可能跨多行（cliLog 输出 JSON.stringify 后换行），
		// 找到起始行后连同其后所有行一并返回。
		if t[0] == '[' {
			return bytes.Join(lines[i:], []byte("\n"))
		}
		if t[0] == '{' && obj == nil {
			obj = t
		}
	}
	return obj
}

// buildSearchTerms 将查询拆为检索串列表：自然语言 Text 优先，关键词补齐。
func buildSearchTerms(q domain.DomainQuery) []string {
	seen := make(map[string]bool)
	var terms []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		terms = append(terms, s)
	}
	add(q.Text)
	for _, k := range q.Keywords {
		add(k)
	}
	return terms
}

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
