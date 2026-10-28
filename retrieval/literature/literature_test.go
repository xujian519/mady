package literature

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDoer 按 URL 片段返回预置响应并统计调用次数。
type fakeDoer struct {
	mu       sync.Mutex
	routes   []fakeRoute
	calls    map[string]int
	requests []string
}

type fakeRoute struct {
	contains string
	status   int
	body     string
	delay    time.Duration
}

func (d *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	if d.calls == nil {
		d.calls = make(map[string]int)
	}
	d.requests = append(d.requests, req.URL.String())
	var matched *fakeRoute
	for i := range d.routes {
		if strings.Contains(req.URL.String(), d.routes[i].contains) {
			d.calls[d.routes[i].contains]++
			matched = &d.routes[i]
			break
		}
	}
	d.mu.Unlock()
	if matched == nil {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
	}
	if matched.delay > 0 {
		select {
		case <-time.After(matched.delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	status := matched.status
	if status == 0 {
		status = 200
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(matched.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func (d *fakeDoer) callCount(contains string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[contains]
}

func newTestClient(doer Doer) *Client {
	return NewClient(ClientOptions{Doer: doer})
}

const openAlexPage = `{"results":[
 {"id":"https://openalex.org/W1","doi":"https://doi.org/10.1000/abc","display_name":"散热壳体研究","publication_year":2023,"cited_by_count":12,
  "abstract_inverted_index":{"一种":[0],"散热":[1],"壳体":[2]},
  "authorships":[{"author":{"display_name":"张三"}},{"author":{"display_name":"李四"}}],
  "primary_location":{"source":{"display_name":"机械工程学报"},"landing_page_url":"https://example.org/w1","pdf_url":""},
  "best_oa_location":{"pdf_url":"https://example.org/w1.pdf"},"open_access":{"oa_url":"https://example.org/oa1"}},
 {"id":"https://openalex.org/W2","display_name":"仅开放链接","publication_year":2021,"open_access":{"oa_url":"https://example.org/oa2"}}
],"meta":{"count":2}}`

func TestOpenAlexSearchParsesWorks(t *testing.T) {
	doer := &fakeDoer{routes: []fakeRoute{{contains: "api.openalex.org/works?search=", body: openAlexPage}}}
	connector := NewOpenAlexConnector(newTestClient(doer), "")
	papers, err := connector.Search(context.Background(), "散热壳体", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("papers = %+v", papers)
	}
	first := papers[0]
	if first.ID != "W1" || first.Title != "散热壳体研究" || first.Year != 2023 || first.CitedBy != 12 {
		t.Errorf("first = %+v", first)
	}
	if first.Abstract != "一种 散热 壳体" {
		t.Errorf("abstract = %q", first.Abstract)
	}
	if first.PDFURL != "https://example.org/w1.pdf" {
		t.Errorf("pdf = %q", first.PDFURL)
	}
	if len(first.Authors) != 2 || first.Authors[0] != "张三" {
		t.Errorf("authors = %v", first.Authors)
	}
	if second := papers[1]; second.PDFURL != "https://example.org/oa2" {
		t.Errorf("开放获取链接回退失败：%+v", second)
	}
}

func TestOpenAlexYearFilterAndLimitClamp(t *testing.T) {
	doer := &fakeDoer{routes: []fakeRoute{{contains: "api.openalex.org/works?search=", body: `{"results":[]}`}}}
	connector := NewOpenAlexConnector(newTestClient(doer), "me@example.org")
	if _, err := connector.Search(context.Background(), "x", SearchOptions{Limit: 100, YearFrom: 2020}); err != nil {
		t.Fatal(err)
	}
	request := doer.requests[0]
	for _, want := range []string{"per-page=25", "from_publication_date%3A2020-01-01", "mailto=me%40example.org"} {
		if !strings.Contains(request, want) {
			t.Errorf("请求缺少 %q：%s", want, request)
		}
	}
}

func TestOpenAlexFetchByDOI(t *testing.T) {
	doer := &fakeDoer{routes: []fakeRoute{{contains: "doi:10.1000/abc", body: `{"id":"https://openalex.org/W9","display_name":"DOI 论文","publication_year":2022}`}}}
	connector := NewOpenAlexConnector(newTestClient(doer), "")
	paper, err := connector.Fetch(context.Background(), "10.1000/abc")
	if err != nil || paper == nil {
		t.Fatalf("fetch: %v %v", paper, err)
	}
	if paper.Title != "DOI 论文" || paper.Source != "openalex" {
		t.Errorf("paper = %+v", paper)
	}
}

func TestOpenAlexHTTPError(t *testing.T) {
	doer := &fakeDoer{routes: []fakeRoute{{contains: "api.openalex.org", status: 503, body: "busy"}}}
	connector := NewOpenAlexConnector(newTestClient(doer), "")
	_, err := connector.Search(context.Background(), "x", SearchOptions{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != 503 {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenAlexSearchCached(t *testing.T) {
	doer := &fakeDoer{routes: []fakeRoute{{contains: "api.openalex.org/works?search=", body: openAlexPage}}}
	connector := NewOpenAlexConnector(newTestClient(doer), "")
	for range 2 {
		if _, err := connector.Search(context.Background(), "散热", SearchOptions{Limit: 3}); err != nil {
			t.Fatal(err)
		}
	}
	if calls := doer.callCount("api.openalex.org/works?search="); calls != 1 {
		t.Errorf("相同检索应命中缓存，实际请求 %d 次", calls)
	}
}

func TestSemanticScholarSearch(t *testing.T) {
	body := `{"data":[{"paperId":"S1","title":"Attention","year":2017,"citationCount":100,"url":"https://s2.org/1",
	 "authors":[{"name":"Vaswani"}],"abstract":"transformer","openAccessPDF":{"url":"https://s2.org/1.pdf"}}]}`
	doer := &fakeDoer{routes: []fakeRoute{{contains: "api.semanticscholar.org", body: body}}}
	connector := NewSemanticScholarConnector(newTestClient(doer), "")
	papers, err := connector.Search(context.Background(), "attention", SearchOptions{Limit: 5, YearFrom: 2015})
	if err != nil || len(papers) != 1 {
		t.Fatalf("papers=%v err=%v", papers, err)
	}
	paper := papers[0]
	if paper.PDFURL != "https://s2.org/1.pdf" || paper.CitedBy != 100 || paper.Year != 2017 {
		t.Errorf("paper = %+v", paper)
	}
	if !strings.Contains(doer.requests[0], "year=2015-") {
		t.Errorf("年份过滤缺失：%s", doer.requests[0])
	}
}

func TestArxivSearch(t *testing.T) {
	body := `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
	  <entry><id>http://arxiv.org/abs/2401.00001v1</id><title>Deep Learning</title>
	  <summary>A paper.</summary><published>2024-01-02T00:00:00Z</published>
	  <author><name>Alice</name></author>
	  <link rel="alternate" href="http://arxiv.org/abs/2401.00001v1"/>
	  <link title="pdf" href="http://arxiv.org/pdf/2401.00001v1"/></entry>
	</feed>`
	doer := &fakeDoer{routes: []fakeRoute{{contains: "export.arxiv.org", body: body}}}
	connector := NewArxivConnector(newTestClient(doer))
	papers, err := connector.Search(context.Background(), "deep learning", SearchOptions{Limit: 3})
	if err != nil || len(papers) != 1 {
		t.Fatalf("papers=%v err=%v", papers, err)
	}
	paper := papers[0]
	if paper.Title != "Deep Learning" || paper.Year != 2024 || paper.PDFURL != "http://arxiv.org/pdf/2401.00001v1" {
		t.Errorf("paper = %+v", paper)
	}
}

func TestCrossrefSearch(t *testing.T) {
	body := `{"message":{"items":[{"DOI":"10.5555/x","title":["A Study"],"abstract":"<jats:p>Hello</jats:p>",
	 "container-title":["Journal"],"author":[{"given":"Bob","family":"Smith"}],"issued":{"date-parts":[[2020,5]]},
	 "URL":"https://doi.org/10.5555/x","link":[{"URL":"https://example.org/x.pdf","content-type":"application/pdf"}]}]}}`
	doer := &fakeDoer{routes: []fakeRoute{{contains: "api.crossref.org", body: body}}}
	connector := NewCrossrefConnector(newTestClient(doer))
	papers, err := connector.Search(context.Background(), "study", SearchOptions{Limit: 5})
	if err != nil || len(papers) != 1 {
		t.Fatalf("papers=%v err=%v", papers, err)
	}
	paper := papers[0]
	if paper.Abstract != "Hello" || paper.Year != 2020 || paper.PDFURL != "https://example.org/x.pdf" {
		t.Errorf("paper = %+v", paper)
	}
	if paper.Authors[0] != "Bob Smith" {
		t.Errorf("authors = %v", paper.Authors)
	}
}

func TestRegistrySearchAllPartialFailureAndConcurrency(t *testing.T) {
	openAlexBody := `{"results":[{"id":"https://openalex.org/W1","doi":"https://doi.org/10.1/a","display_name":"共同论文一","publication_year":2023}]}`
	crossrefBody := `{"message":{"items":[{"DOI":"10.1/a","title":["共同论文一"]},{"DOI":"10.1/b","title":["另一篇论文"]}]}}`
	doer := &fakeDoer{routes: []fakeRoute{
		{contains: "api.openalex.org", body: openAlexBody, delay: 40 * time.Millisecond},
		{contains: "api.crossref.org", body: crossrefBody, delay: 40 * time.Millisecond},
		{contains: "api.semanticscholar.org", status: 500, body: "boom"},
	}}
	registry := NewRegistry(newTestClient(doer), "", "")
	start := time.Now()
	result := registry.SearchAll(context.Background(), "论文", SearchOptions{Limit: 3}, []string{"openalex", "crossref", "semantic-scholar", "unknown"})
	elapsed := time.Since(start)
	if elapsed > 75*time.Millisecond {
		t.Errorf("不同主机应并发查询，实际耗时 %v", elapsed)
	}
	if len(result.Papers) != 2 {
		t.Errorf("跨源 DOI 去重后应为 2 篇：%+v", result.Papers)
	}
	if result.Errors["semantic-scholar"] == "" || result.Errors["unknown"] == "" {
		t.Errorf("errors = %v", result.Errors)
	}
	if len(registry.ValidateSources([]string{"openalex", "bogus"})) != 1 {
		t.Error("未知源应被识别")
	}
	if len(registry.Sources()) != 4 {
		t.Errorf("sources = %+v", registry.Sources())
	}
}

func TestDownloaderValidation(t *testing.T) {
	validPDF := "%PDF-1.4\n" + strings.Repeat("x", 1200)
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     string
	}{
		{"valid", 200, "application/pdf", validPDF, ""},
		{"html", 200, "text/html; charset=utf-8", validPDF, "HTML"},
		{"small", 200, "application/pdf", "%PDF-1.4\n", "过小"},
		{"no magic", 200, "application/pdf", strings.Repeat("x", 2000), "magic"},
		{"http error", 403, "application/pdf", validPDF, "HTTP 403"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doer := &fakeDoer{routes: []fakeRoute{{contains: "example.org", status: tc.status, body: tc.body}}}
			downloader := NewDownloader(DownloadOptions{Doer: &contentTypeDoer{inner: doer, contentType: tc.contentType}})
			_, err := downloader.Fetch(context.Background(), "https://example.org/paper.pdf")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
	if _, err := NewDownloader(DownloadOptions{}).Fetch(context.Background(), "ftp://example.org/x.pdf"); err == nil {
		t.Error("非 http(s) 链接应拒绝")
	}
}

// contentTypeDoer 覆盖响应的 Content-Type。
type contentTypeDoer struct {
	inner       Doer
	contentType string
}

func (d *contentTypeDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.inner.Do(req)
	if err != nil || resp == nil {
		return resp, err
	}
	resp.Header = http.Header{"Content-Type": []string{d.contentType}}
	return resp, nil
}

func TestSavePDFSanitizesName(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	path, err := SavePDF(root, "papers", "一种/散热:壳体 研究", "https://example.org/x.pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("papers", "一种_散热_壳体_研究.pdf") {
		t.Errorf("path = %q", path)
	}
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("saved = %q err=%v", data, err)
	}
}

func TestSavePDFRefusesSymlinkEscape(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(work, "link")); err != nil {
		t.Skipf("当前环境不支持符号链接：%v", err)
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if _, err := SavePDF(root, "link", "escape", "https://example.org/x.pdf", []byte("%PDF-1.4")); err == nil {
		t.Fatal("经符号链接写出应被 os.Root 拒绝")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("越界目录被写入：%v", entries)
	}
}
