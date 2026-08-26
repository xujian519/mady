package browser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// 各数据源的 heredoc 内嵌提取代码（经真实页面验证）。
//
// 约束：代码内不得包含单引号或反引号（heredoc 模板以单引号字符串传给
// js() 求值），选择器一律使用双引号。
//
// 注意：本文件的 JS 常量与 skills/ego-browser/learnings/*/tools/*.js 内的
// 同名脚本互为镜像（逻辑重复但各自独立演进）。改动任一处的页面选择器或
// 提取逻辑时，必须同步另一处，否则 Go 检索管道与 SKILL.md 提示词驱动的手工
// 操作会得出不一致结果。

// googleSearchJS 提取 Google Patents 搜索结果（search-result-item 按行解析）。
// ${max} 在执行前替换为最大条数。
const googleSearchJS = `(() => {
  const items = [...document.querySelectorAll('search-result-item')]
  return items.slice(0, ${max}).map(item => {
    const lines = item.innerText.split('\n').map(l => l.trim()).filter(Boolean)
    const title = lines[0] || ''
    const meta = lines[1] || ''
    const dateLine = lines[2] || ''
    const abstract = lines.slice(3).join('\n').slice(0, 500)
    const pdf = item.querySelector('a.pdfLink')
    const link = item.querySelector('a#link')
    return {
      title, meta, dateLine, abstract,
      number: pdf ? pdf.innerText.trim() : '',
      pdfUrl: pdf ? pdf.href : '',
      url: link ? link.href : '',
      itemId: item.id,
    }
  }).filter(r => r.title)
})()`

// googleDetailPre 滚动详情页触发 claims/description 懒加载。
const googleDetailPre = `await scrollToBottomUntil(
  async () => {
    const n = await js("document.querySelectorAll('section#claims, section#description').length")
    return n >= 2
  },
  { step: 800, wait: 1, maxSteps: 15 }
)`

// googleDetailJS 提取 Google Patents 详情页全文。
const googleDetailJS = `(() => {
  const title = document.querySelector('meta[name="DC.title"]')?.content?.trim() || ''
  const number = document.querySelector('meta[name="citation_patent_number"]')?.content?.trim() || ''
  const abstract = document.querySelector('section#abstract')?.innerText?.trim() || ''
  const claims = document.querySelector('section#claims')?.innerText?.trim() || ''
  const description = document.querySelector('section#description')?.innerText?.trim() || ''
  return { title, number, abstract, claims, description }
})()`

// espacenetSearchJS 提取 Espacenet 搜索结果（data-qa 稳定选择器）。
const espacenetSearchJS = `(() => {
  const items = [...document.querySelectorAll('article[data-qa="result_resultList"]')]
  return items.slice(0, ${max}).map(item => {
    const title = item.querySelector('span[lang="en"]')?.innerText?.trim() || ''
    const subtitle = item.querySelector('[class*="subtitle"]')?.innerText?.trim() || ''
    const applicant = item.querySelector('[title="Applicant"] span')?.innerText?.trim() || ''
    const abstract = item.querySelector('[class*="abstract"]')?.innerText?.trim() || ''
    return { title, subtitle, applicant, abstract }
  }).filter(r => r.title)
})()`

// espacenetSearchPre 等待并滚动 Espacenet 搜索结果触发 XHR 渲染。
const espacenetSearchPre = `await wait(3)
await scrollToBottomUntil(
  async () => {
    const n = await js("document.querySelectorAll('article[data-qa=\"result_resultList\"]').length")
    return n >= 1
  },
  { step: 600, wait: 1, maxSteps: 10 }
)
await wait(2)`

// espacenetDetailJS 提取 Espacenet biblio 详情页。
const espacenetDetailJS = `(() => {
  const title = document.querySelector('[data-qa="biblio_title"]')?.innerText?.trim()
    || document.querySelector('h1')?.innerText?.trim() || ''
  const number = document.querySelector('[data-qa="publicationNumber"]')?.innerText?.trim() || ''
  const applicant = document.querySelector('[data-qa="applicant"]')?.innerText?.trim() || ''
  const inventor = document.querySelector('[data-qa="inventor"]')?.innerText?.trim() || ''
  const abstract = document.querySelector('[data-qa="abstract"]')?.innerText?.trim() || ''
  return { title, number, applicant, inventor, abstract }
})()`

// 包级正则：避免每次调用重复编译（热路径）。
var (
	cnCountryRe = regexp.MustCompile(`(?i)country\s*:\s*CN`)
	notAlnumRe  = regexp.MustCompile(`[^A-Za-z0-9]`)
)

// googleSearchURL 构造 Google Patents 搜索 URL（query 为纯文本，此处编码）。
func googleSearchURL(query string, maxResults int) string {
	return fmt.Sprintf("https://patents.google.com/?q=%s&num=%d",
		url.QueryEscape(query), maxResults)
}

// cnipaSearchURL 在查询上附加 country:CN 过滤（中国专利）。
func cnipaSearchURL(query string, maxResults int) string {
	q := query
	if !cnCountryRe.MatchString(q) {
		q += " country:CN"
	}
	return fmt.Sprintf("https://patents.google.com/?q=%s&num=%d",
		url.QueryEscape(q), maxResults)
}

// googleDetailURL 构造 Google Patents 详情页 URL（中文界面）。
func googleDetailURL(patentNumber string) string {
	clean := notAlnumRe.ReplaceAllString(patentNumber, "")
	return fmt.Sprintf("https://patents.google.com/patent/%s/zh", clean)
}

var espacenetPubRe = regexp.MustCompile(`^([A-Z]{2})(\d+)([A-Z]\d?)?$`)

// espacenetDetailURL 将公开号解析为 biblio 详情页 URL。
// "CN107891199A" → CC=CN&NR=107891199&KC=A。
// 完整 URL 透传时校验不含单引号（heredoc 模板以单引号字符串嵌入 URL，
// 含引号会破坏 JS 脚本）。
func espacenetDetailURL(patentNumber string) string {
	if strings.HasPrefix(patentNumber, "http") {
		if strings.ContainsAny(patentNumber, "'`") {
			return "https://worldwide.espacenet.com/patent/search"
		}
		return patentNumber
	}
	m := espacenetPubRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(patentNumber)))
	if m == nil {
		return "https://worldwide.espacenet.com/patent/search"
	}
	kc := m[3]
	if kc == "" {
		kc = "A"
	}
	return fmt.Sprintf("https://worldwide.espacenet.com/publicationDetails/biblio?CC=%s&NR=%s&KC=%s",
		m[1], m[2], kc)
}

// SourceName 常量（测试与装配层可引用）。
const (
	SourceNameGoogle    = "Google Patents (via ego-browser)"
	SourceNameCNIPA     = "CNIPA 中国专利 (via ego-browser)"
	SourceNameEspacenet = "Espacenet (via ego-browser)"
)

// NewGooglePatentsRetriever 构造 Google Patents 在线检索器。
// ego-browser 不可用时返回 nil（静默降级）。
func NewGooglePatentsRetriever(cfg BrowserRetrieverConfig) *BrowserRetriever {
	return NewBrowserRetriever(cfg, dataSource{
		sourceName: SourceNameGoogle,
		taskSpace:  "mady-gp",
		searchURL:  googleSearchURL,
		searchJS:   googleSearchJS,
		detailURL:  googleDetailURL,
		detailPre:  googleDetailPre,
		detailJS:   googleDetailJS,
	})
}

// NewCNIPARetriever 构造中国专利在线检索器（Google Patents country:CN 过滤，
// 见 skills/ego-browser/learnings/patent-cnipa/notes/overview.md 的数据源说明）。
func NewCNIPARetriever(cfg BrowserRetrieverConfig) *BrowserRetriever {
	return NewBrowserRetriever(cfg, dataSource{
		sourceName: SourceNameCNIPA,
		taskSpace:  "mady-cn",
		searchURL:  cnipaSearchURL,
		searchJS:   googleSearchJS,
		detailURL:  googleDetailURL,
		detailPre:  googleDetailPre,
		detailJS:   googleDetailJS,
	})
}

// NewEspacenetRetriever 构造 Espacenet 在线检索器。
func NewEspacenetRetriever(cfg BrowserRetrieverConfig) *BrowserRetriever {
	return NewBrowserRetriever(cfg, dataSource{
		sourceName: SourceNameEspacenet,
		taskSpace:  "mady-ep",
		searchURL: func(query string, _ int) string {
			return "https://worldwide.espacenet.com/patent/search?q=" + url.QueryEscape(query)
		},
		searchPre: espacenetSearchPre,
		searchJS:  espacenetSearchJS,
		detailURL: func(n string) string {
			return espacenetDetailURL(n)
		},
		detailJS: espacenetDetailJS,
	})
}

// NewDefaultPatentRetrievers 一次性构造 Google Patents / CNIPA / Espacenet
// 三源在线专利数据库检索器（共用同一配置）。供装配层（tools/patent_web_search.go、
// cmd/mady、bootstrap/init_reasoning.go）复用，避免各处在三源工厂与 taskSpace
// 约定上重复定义导致漂移。ego-browser 不可用时各元素为 nil，由调用方过滤组合。
func NewDefaultPatentRetrievers(cfg BrowserRetrieverConfig) (google, cnipa, espacenet *BrowserRetriever) {
	return NewGooglePatentsRetriever(cfg), NewCNIPARetriever(cfg), NewEspacenetRetriever(cfg)
}
