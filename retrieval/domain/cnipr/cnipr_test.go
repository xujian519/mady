package cnipr

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeAppNumber(t *testing.T) {
	cases := map[string]string{
		"202311356097.5":     "202311356097.5",
		"2023113560975":      "202311356097.5",
		"CN2023113560975":    "CN202311356097.5",
		"cn202311356097.5":   "CN202311356097.5",
		" 2023 1135 6097 5 ": "202311356097.5",
	}
	for input, want := range cases {
		if got := NormalizeAppNumber(input); got != want {
			t.Errorf("NormalizeAppNumber(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClassifyNumber(t *testing.T) {
	cases := map[string]string{
		"CN123456789A":     "pub",
		"ZL202310000001.0": "other",
		"202311356097.5":   "app",
		"2023113560975":    "app",
		"US1234567A":       "other",
	}
	for input, want := range cases {
		if got := ClassifyNumber(input); got != want {
			t.Errorf("ClassifyNumber(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizePDFURLs(t *testing.T) {
	urls := NormalizePDFURLs([]string{
		"https://pic.cnipr.com/pdfcn/1.PDF",
		"pic.cnipr.com/pdfcn/2.PDF",
		"http://pic.cnipr.com/pdfcn/1.PDF",
	})
	want := []string{"http://pic.cnipr.com/pdfcn/1.PDF", "http://pic.cnipr.com/pdfcn/2.PDF"}
	if len(urls) != len(want) || urls[0] != want[0] || urls[1] != want[1] {
		t.Errorf("urls = %v, want %v", urls, want)
	}
}

func TestParseLegalRecords(t *testing.T) {
	text := `申请号：202311356097.5 法律状态公告日：2024.05.01 法律状态：授权 法律状态信息：授予发明专利权通知书
申请号：202311356097.5 法律状态公告日：2023.11.01 法律状态：公开 法律状态信息：公布
共 2 条记录 知识产权出版社`
	total, records := ParseLegalRecords(text)
	if total == nil || *total != 2 {
		t.Fatalf("total = %v", total)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v", records)
	}
	if records[0].AppNumber != "202311356097.5" || records[0].Status != "授权" {
		t.Errorf("record0 = %+v", records[0])
	}
	if !strings.Contains(records[1].StatusInfo, "公布") {
		t.Errorf("record1 = %+v", records[1])
	}
	// 零命中页没有「共 N 条记录」：total 必须是 nil，不编造 0。
	zeroTotal, zeroRecords := ParseLegalRecords("未检索到相关记录")
	if zeroTotal != nil || len(zeroRecords) != 0 {
		t.Errorf("zero = %v/%v", zeroTotal, zeroRecords)
	}
}

func TestPlanSearchRequest(t *testing.T) {
	cases := []struct {
		limit int
		want  SearchRequest
	}{
		{0, SearchRequest{Limit: 10, Pages: 1}},
		{5, SearchRequest{Limit: 5, Pages: 1}},
		{11, SearchRequest{Limit: 11, Pages: 2}},
		{100, SearchRequest{Limit: 30, Pages: 3}},
	}
	for _, tc := range cases {
		if got := PlanSearchRequest(tc.limit); got != tc.want {
			t.Errorf("PlanSearchRequest(%d) = %+v, want %+v", tc.limit, got, tc.want)
		}
	}
}

func TestPlanPDFAndLegalRequests(t *testing.T) {
	requested, unsupported := PlanPDFRequests([]string{"CN123456789A", "202311356097.5"})
	if len(requested) != 1 || requested[0] != "CN123456789A" {
		t.Errorf("requested = %v", requested)
	}
	if len(unsupported) != 1 || unsupported[0].Error == "" {
		t.Errorf("unsupported = %+v", unsupported)
	}

	queries, unsupportedLegal := PlanLegalQueries([]string{"2023113560975", "CN123456789A", "US123"})
	if len(queries) != 2 || queries[0].AppNumber != "202311356097.5" || queries[1].PubNumber != "CN123456789A" {
		t.Errorf("queries = %+v", queries)
	}
	if len(unsupportedLegal) != 1 {
		t.Errorf("unsupportedLegal = %+v", unsupportedLegal)
	}
}

func TestParseSearchTotalAndDate(t *testing.T) {
	if total, ok := ParseSearchTotal("全部655件专利"); !ok || total != 655 {
		t.Errorf("total = %d ok=%v", total, ok)
	}
	if _, ok := ParseSearchTotal("结果页"); ok {
		t.Error("无标记时不应给出总数")
	}
	if got := NormalizeDate("2024.01.02"); got != "2024-01-02" {
		t.Errorf("date = %q", got)
	}
	if got := NormalizeDate("2024年1月2日"); got != "2024年1月2日" {
		t.Errorf("非标准日期应原样返回，got %q", got)
	}
}

func TestExtractTaggedJSON(t *testing.T) {
	output := "启动相关日志\nEGO_CNIPR_SEARCH:{\"header\":\"全部1件专利\",\"items\":[]}\n"
	payload, ok := ExtractTaggedJSON(output, "CNIPR_SEARCH")
	if !ok || payload["header"] != "全部1件专利" {
		t.Fatalf("payload = %v ok=%v", payload, ok)
	}
	if _, ok := ExtractTaggedJSON(output, "OTHER"); ok {
		t.Error("错误标签不应命中")
	}
}

func TestScriptBuilders(t *testing.T) {
	search := BuildSearchScript(`石墨烯 "电池"`, 2, DefaultPageBudgets)
	for _, want := range []string{"taskSpace(", `const query = "石墨烯 \"电池\""`, "const pages = 2", "EGO_CNIPR_SEARCH:", "欢迎您"} {
		if !strings.Contains(search, want) {
			t.Errorf("search 脚本缺少 %q", want)
		}
	}
	if strings.Contains(search, "{{") {
		t.Error("检索脚本存在未替换的占位符")
	}
	pdf := BuildPDFLinksScript([]string{"CN123456789A"}, DefaultPageBudgets)
	for _, want := range []string{"CN123456789A", "doDetailSearch", "EGO_CNIPR_PDFLINKS:"} {
		if !strings.Contains(pdf, want) {
			t.Errorf("pdf 脚本缺少 %q", want)
		}
	}
	legal := BuildLegalScript([]LegalQuery{{Number: "202311356097.5", AppNumber: "202311356097.5"}}, DefaultPageBudgets)
	for _, want := range []string{"202311356097.5", "doLegalSearch", "EGO_CNIPR_LEGAL:"} {
		if !strings.Contains(legal, want) {
			t.Errorf("legal 脚本缺少 %q", want)
		}
	}
}

// fakeSession 是可编排的假 ego 会话。
type fakeSession struct {
	available bool
	reason    string
	output    string
	stderr    string
	timedOut  bool
	err       error
	delay     time.Duration
	runs      atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
}

func (s *fakeSession) CheckAvailability() (bool, string) {
	return s.available, s.reason
}

func (s *fakeSession) RunScript(_ string, _ time.Duration) (RunResult, error) {
	s.runs.Add(1)
	current := s.active.Add(1)
	for {
		previous := s.maxActive.Load()
		if current <= previous || s.maxActive.CompareAndSwap(previous, current) {
			break
		}
	}
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.active.Add(-1)
	return RunResult{Output: s.output, Stderr: s.stderr, TimedOut: s.timedOut}, s.err
}

func newAvailableSession(output string) *fakeSession {
	return &fakeSession{available: true, output: output}
}

func TestChannelSearchSuccess(t *testing.T) {
	payload := `EGO_CNIPR_SEARCH:{"header":"全部 2 件专利","items":[{"cursor":0,"number":"CN123456789A","title":"一种装置","text":"公开(公告)号：CN123456789A 公开(公告)日：2024.01.02 申请(专利权)人：某公司 摘要：一种装置。"}]}`
	channel := NewChannel(newAvailableSession(payload))
	result, err := channel.Search("电池", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 2 || len(result.Hits) != 1 {
		t.Fatalf("result = %+v", result)
	}
	hit := result.Hits[0]
	if hit.Number != "CN123456789A" || hit.PublicationDate != "2024-01-02" || hit.Assignee != "某公司" {
		t.Errorf("hit = %+v", hit)
	}
	if !strings.Contains(hit.Abstract, "一种装置") {
		t.Errorf("abstract = %q", hit.Abstract)
	}
}

func TestChannelClassifiedFailures(t *testing.T) {
	cases := []struct {
		name     string
		session  *fakeSession
		wantCode string
	}{
		{"unavailable", &fakeSession{available: false, reason: "没装 ego"}, FailureSetupRequired},
		{"timeout", &fakeSession{available: true, timedOut: true, output: "x"}, FailureTimeout},
		{"receipt missing", &fakeSession{available: true, output: "no receipt"}, FailureReceiptMissing},
		{"login required", newAvailableSession(`EGO_CNIPR_SEARCH:{"error":{"code":"login_required","message":"请登录"}}`), FailureLoginRequired},
		{"page state", newAvailableSession(`EGO_CNIPR_SEARCH:{"items":[],"error":null}`), FailurePageState},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewChannel(tc.session).Search("x", 1)
			var channelErr *ChannelError
			if !errors.As(err, &channelErr) || channelErr.Code != tc.wantCode {
				t.Fatalf("err = %v, want %s", err, tc.wantCode)
			}
		})
	}
}

func TestChannelPDFAndLegalAlignment(t *testing.T) {
	payload := `EGO_CNIPR_PDFLINKS:{"items":[{"number":"CN123456789A","matches":["pic.cnipr.com/pdfcn/1.PDF"],"isUnexamined":true,"isAuthorized":false}]}`
	channel := NewChannel(newAvailableSession(payload))
	items, err := channel.PDFLinks([]string{"CN123456789A", "202311356097.5"})
	if err != nil {
		t.Fatalf("pdfLinks: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if len(items[0].URLs) != 1 || items[0].URLs[0] != "http://pic.cnipr.com/pdfcn/1.PDF" || !items[0].Unexamined {
		t.Errorf("item0 = %+v", items[0])
	}
	if items[1].Error == "" {
		t.Errorf("非公开号应报条目错误：%+v", items[1])
	}

	legalPayload := `EGO_CNIPR_LEGAL:{"items":[{"number":"202311356097.5","appNumber":"202311356097.5","text":"申请号：202311356097.5 法律状态公告日：2024.05.01 法律状态：授权 法律状态信息：授权公告 共 1 条记录"}]}`
	legal, err := NewChannel(newAvailableSession(legalPayload)).LegalStatus([]string{"202311356097.5"})
	if err != nil {
		t.Fatalf("legal: %v", err)
	}
	if len(legal) != 1 || legal[0].Total == nil || *legal[0].Total != 1 || len(legal[0].Records) != 1 {
		t.Errorf("legal = %+v", legal)
	}
}

func TestChannelSerializesRuns(t *testing.T) {
	session := &fakeSession{available: true, output: `EGO_CNIPR_SEARCH:{"header":"全部1件专利","items":[]}`, delay: 20 * time.Millisecond}
	channel := NewChannel(session)
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = channel.Search("x", 1)
		}()
	}
	wg.Wait()
	if session.maxActive.Load() != 1 {
		t.Errorf("并发运行数 = %d, want 1", session.maxActive.Load())
	}
	if session.runs.Load() != 3 {
		t.Errorf("runs = %d", session.runs.Load())
	}
}
