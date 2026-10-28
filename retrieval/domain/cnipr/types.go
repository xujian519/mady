package cnipr

import "fmt"

// 失败分类。
const (
	FailureLoginRequired  = "login_required"
	FailureSetupRequired  = "setup_required"
	FailureTimeout        = "timeout"
	FailureReceiptMissing = "receipt_missing"
	FailurePageState      = "page_state"
)

// ChannelError 是通道级失败：调用方按 Code 路由（login_required 要用户登录、
// setup_required 要装 ego lite，page_state/receipt_missing 是站点或运行时变化）。
type ChannelError struct {
	Code   string
	Detail string
}

func (e *ChannelError) Error() string {
	return fmt.Sprintf("CNIPR 通道失败（%s）：%s", e.Code, e.Detail)
}

// SearchHit 是一条检索命中。
type SearchHit struct {
	Cursor          int    `json:"cursor"`
	Number          string `json:"number"`
	Title           string `json:"title"`
	PublicationDate string `json:"publication_date,omitempty"`
	Assignee        string `json:"assignee,omitempty"`
	Abstract        string `json:"abstract,omitempty"`
}

// SearchResult 是一次关键词检索的结果。
type SearchResult struct {
	Total int         `json:"total"`
	Hits  []SearchHit `json:"hits"`
}

// PdfLinkItem 是一个公开号的全文 PDF 直链结果。
type PdfLinkItem struct {
	Number     string   `json:"number"`
	URLs       []string `json:"urls"`
	Unexamined bool     `json:"unexamined"`
	Authorized bool     `json:"authorized"`
	Error      string   `json:"error,omitempty"`
}

// LegalRecord 是一条法律状态记录。
type LegalRecord struct {
	AppNumber    string `json:"app_number"`
	AnnounceDate string `json:"announce_date"`
	Status       string `json:"status"`
	StatusInfo   string `json:"status_info"`
}

// LegalStatusItem 是一个号的法律状态结果。
type LegalStatusItem struct {
	Number       string        `json:"number"`
	AppNumber    string        `json:"app_number,omitempty"`
	ResolvedFrom string        `json:"resolved_from,omitempty"`
	Total        *int          `json:"total"`
	Records      []LegalRecord `json:"records"`
	Error        string        `json:"error,omitempty"`
}

// LegalQuery 是一条法律状态查询目标：申请号直接查，公开号先解析申请号。
type LegalQuery struct {
	Number    string
	AppNumber string
	PubNumber string
}

// SearchRequest 是收敛后的检索请求。
type SearchRequest struct {
	Limit int
	Pages int
}

// PageBudgets 是页面等待预算（毫秒）。
type PageBudgets struct {
	LoginPollMs         int
	LoginAttempts       int
	LoginRounds         int
	NavigationTimeoutMs int
	ActionTimeoutMs     int
	ResultTimeoutMs     int
	RenderTimeoutMs     int
	DetailTimeoutMs     int
}

// DefaultPageBudgets 是出厂等待预算（站点实测值）。
var DefaultPageBudgets = PageBudgets{
	LoginPollMs: 1000, LoginAttempts: 10, LoginRounds: 2,
	NavigationTimeoutMs: 45000, ActionTimeoutMs: 15000,
	ResultTimeoutMs: 30000, RenderTimeoutMs: 20000, DetailTimeoutMs: 20000,
}
