package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/retrieval/literature"
)

// paperDoer 按 URL 路由固定的文献源响应与 PDF 响应。
type paperDoer struct {
	openAlexJSON string
	pdfBody      []byte
	calls        []string
}

func (d *paperDoer) Do(req *http.Request) (*http.Response, error) {
	d.calls = append(d.calls, req.URL.String())
	var body []byte
	contentType := "application/json"
	switch {
	case strings.Contains(req.URL.Host, "openalex.org") && strings.Contains(req.URL.Path, "/works"):
		body = []byte(d.openAlexJSON)
	case strings.HasSuffix(req.URL.Path, ".pdf"):
		body = d.pdfBody
		contentType = "application/pdf"
	default:
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("not found")),
			Request:    req,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}, nil
}

const openAlexSearchJSON = `{"results":[{
	"id":"https://openalex.org/W123",
	"doi":"https://doi.org/10.1234/abc",
	"display_name":"Deep Learning for Patents",
	"publication_year":2015,
	"cited_by_count":42,
	"authorships":[{"author":{"display_name":"Ada Lovelace"}}],
	"primary_location":{"source":{"display_name":"Nature"},"landing_page_url":"https://example.org/article","pdf_url":""},
	"abstract_inverted_index":{"prior":[0],"art":[1]}
}]}`

const openAlexWorkJSON = `{
	"id":"https://openalex.org/W123",
	"doi":"https://doi.org/10.1234/abc",
	"display_name":"Deep Learning for Patents",
	"publication_year":2015,
	"cited_by_count":42,
	"authorships":[{"author":{"display_name":"Ada Lovelace"}}],
	"primary_location":{"source":{"display_name":"Nature"},"landing_page_url":"https://example.org/article","pdf_url":"https://example.org/paper.pdf"},
	"best_oa_location":{"pdf_url":"https://example.org/paper.pdf"}
}`

func fakePDF() []byte {
	body := make([]byte, 2048)
	copy(body, "%PDF-1.7\n")
	for i := len("%PDF-1.7\n"); i < len(body); i++ {
		body[i] = 'x'
	}
	return body
}

func newTestPaperTools(t *testing.T, openAlexJSON string) (*paperTools, *paperDoer, string) {
	t.Helper()
	dir := t.TempDir()
	doer := &paperDoer{openAlexJSON: openAlexJSON, pdfBody: fakePDF()}
	client := literature.NewClient(literature.ClientOptions{Doer: doer})
	registry := literature.NewRegistry(client, "", "")
	downloader := literature.NewDownloader(literature.DownloadOptions{Doer: doer})
	return newPaperTools(registry, downloader, dir), doer, dir
}

func TestPaperSearchRendersHits(t *testing.T) {
	tools, _, _ := newTestPaperTools(t, openAlexSearchJSON)
	search := tools.searchTool()
	if !search.ReadOnly {
		t.Fatal("paper_search 应为只读工具")
	}
	out, err := search.Func(context.Background(), json.RawMessage(`{"query":"patents","sources":["openalex"]}`))
	if err != nil {
		t.Fatalf("paper_search 失败：%v", err)
	}
	result, ok := out.(PaperSearchResult)
	if !ok {
		t.Fatalf("返回类型错误：%T", out)
	}
	if len(result.Papers) != 1 {
		t.Fatalf("期望 1 篇，得到 %d", len(result.Papers))
	}
	if result.Papers[0].DOI != "10.1234/abc" || result.Papers[0].Source != "openalex" {
		t.Fatalf("命中字段错误：%+v", result.Papers[0])
	}
	for _, want := range []string{"Deep Learning for Patents", "Ada Lovelace", "引用：42"} {
		if !strings.Contains(result.Rendered, want) {
			t.Errorf("渲染缺少 %q：\n%s", want, result.Rendered)
		}
	}
}

func TestPaperSearchRejectsUnknownSource(t *testing.T) {
	tools, _, _ := newTestPaperTools(t, openAlexSearchJSON)
	_, err := tools.searchTool().Func(context.Background(),
		json.RawMessage(`{"query":"patents","sources":["no-such-source"]}`))
	if err == nil || !strings.Contains(err.Error(), "未知文献源") {
		t.Fatalf("期望未知源错误，得到 %v", err)
	}
}

func TestPaperSearchEmptyQuery(t *testing.T) {
	tools, _, _ := newTestPaperTools(t, openAlexSearchJSON)
	if _, err := tools.searchTool().Func(context.Background(), json.RawMessage(`{"query":"  "}`)); err == nil {
		t.Fatal("空 query 应报错")
	}
}

func TestPaperListSources(t *testing.T) {
	tools, _, _ := newTestPaperTools(t, openAlexSearchJSON)
	out, err := tools.sourcesTool().Func(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("paper_list_sources 失败：%v", err)
	}
	payload, _ := out.(map[string]any)
	sources, _ := payload["sources"].([]literature.SourceInfo)
	if len(sources) != 4 {
		t.Fatalf("期望 4 个源，得到 %d", len(sources))
	}
	rendered, _ := payload["rendered"].(string)
	if !strings.Contains(rendered, "openalex") || !strings.Contains(rendered, "semantic-scholar") {
		t.Fatalf("源清单渲染不完整：\n%s", rendered)
	}
}

func TestPaperDownloadDirectURL(t *testing.T) {
	tools, _, workDir := newTestPaperTools(t, openAlexSearchJSON)
	out, err := tools.downloadTool().Func(context.Background(), json.RawMessage(
		`{"pdf_url":"https://example.org/paper.pdf","title":"专利 论文","output_dir":"papers"}`))
	if err != nil {
		t.Fatalf("下载失败：%v", err)
	}
	payload, _ := out.(map[string]any)
	rel, _ := payload["file"].(string)
	if rel != filepath.Join("papers", "专利_论文.pdf") {
		t.Fatalf("落盘路径不符合预期：%q", rel)
	}
	data, err := os.ReadFile(filepath.Join(workDir, rel))
	if err != nil {
		t.Fatalf("读取落盘文件失败：%v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("落盘内容不是 PDF")
	}
	if bytes, _ := payload["bytes"].(int); bytes != len(fakePDF()) {
		t.Fatalf("字节数不符：%v", payload["bytes"])
	}
}

func TestPaperDownloadResolvesSourceID(t *testing.T) {
	tools, doer, workDir := newTestPaperTools(t, openAlexWorkJSON)
	out, err := tools.downloadTool().Func(context.Background(),
		json.RawMessage(`{"source":"openalex","id":"W123"}`))
	if err != nil {
		t.Fatalf("按 id 下载失败：%v", err)
	}
	payload, _ := out.(map[string]any)
	rel, _ := payload["file"].(string)
	if !strings.HasPrefix(rel, filepath.Join("论文原文")) || !strings.HasSuffix(rel, "Deep_Learning_for_Patents.pdf") {
		t.Fatalf("默认目录或文件名不符合预期：%q", rel)
	}
	if _, err := os.Stat(filepath.Join(workDir, rel)); err != nil {
		t.Fatalf("文件未落盘：%v", err)
	}
	var fetchedPDF bool
	for _, called := range doer.calls {
		if strings.HasSuffix(called, "paper.pdf") {
			fetchedPDF = true
		}
	}
	if !fetchedPDF {
		t.Fatal("没有按记录的 PDF 链接发起下载")
	}
}

func TestPaperDownloadRejectsEscape(t *testing.T) {
	tools, _, _ := newTestPaperTools(t, openAlexSearchJSON)
	if _, err := tools.downloadTool().Func(context.Background(), json.RawMessage(
		`{"pdf_url":"https://example.org/paper.pdf","output_dir":"../escape"}`)); err == nil {
		t.Fatal("越界目录应被拒绝")
	}
}

func TestPaperDownloadRejectsSymlinkEscape(t *testing.T) {
	tools, _, workDir := newTestPaperTools(t, openAlexSearchJSON)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workDir, "link")); err != nil {
		t.Skipf("当前环境不支持符号链接：%v", err)
	}
	if _, err := tools.downloadTool().Func(context.Background(), json.RawMessage(
		`{"pdf_url":"https://example.org/paper.pdf","output_dir":"link"}`)); err == nil {
		t.Fatal("符号链接越界应被拒绝")
	}
}

func TestPaperDownloadRequiresTarget(t *testing.T) {
	tools, _, _ := newTestPaperTools(t, openAlexSearchJSON)
	if _, err := tools.downloadTool().Func(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("缺少 pdf_url 与 source/id 应报错")
	}
}
