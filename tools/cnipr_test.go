package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/retrieval/domain/cnipr"
)

type fakeCNIPRSession struct {
	available bool
	reason    string
	output    string
	timedOut  bool
}

func (s *fakeCNIPRSession) CheckAvailability() (bool, string) {
	return s.available, s.reason
}

func (s *fakeCNIPRSession) RunScript(string, time.Duration) (cnipr.RunResult, error) {
	return cnipr.RunResult{Output: s.output, TimedOut: s.timedOut}, nil
}

func runCNIPRTool(t *testing.T, session cnipr.Session, args string) (CNIPRToolResult, error) {
	t.Helper()
	tool := newCNIPRToolWithChannel(cnipr.NewChannel(session))
	raw, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return CNIPRToolResult{}, err
	}
	result, ok := raw.(CNIPRToolResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return result, nil
}

func TestCNIPRToolSearch(t *testing.T) {
	session := &fakeCNIPRSession{
		available: true,
		output:    `EGO_CNIPR_SEARCH:{"header":"全部 1 件专利","items":[{"cursor":0,"number":"CN123456789A","title":"一种散热壳体","text":"公开(公告)号：CN123456789A 公开(公告)日：2024.01.02 申请(专利权)人：某公司 摘要：一种散热壳体。"}]}`,
	}
	result, err := runCNIPRTool(t, session, `{"query":"散热壳体","limit":5}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.Mode != "search" || result.Search == nil || result.Search.Total != 1 {
		t.Fatalf("result = %+v", result)
	}
	for _, want := range []string{"CNIPR 检索", "全部 1 件", "一种散热壳体", "2024-01-02", "某公司"} {
		if !strings.Contains(result.Rendered, want) {
			t.Errorf("rendered 缺少 %q：%s", want, result.Rendered)
		}
	}
}

func TestCNIPRToolPDFLinks(t *testing.T) {
	session := &fakeCNIPRSession{
		available: true,
		output:    `EGO_CNIPR_PDFLINKS:{"items":[{"number":"CN123456789A","matches":["pic.cnipr.com/pdfcn/1.PDF"],"isUnexamined":false,"isAuthorized":true}]}`,
	}
	result, err := runCNIPRTool(t, session, `{"mode":"pdf_links","numbers":["CN123456789A","202311356097.5"]}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if len(result.PDFLinks) != 2 {
		t.Fatalf("items = %+v", result.PDFLinks)
	}
	for _, want := range []string{"http://pic.cnipr.com/pdfcn/1.PDF", "只处理公开(公告)号"} {
		if !strings.Contains(result.Rendered, want) {
			t.Errorf("rendered 缺少 %q：%s", want, result.Rendered)
		}
	}
}

func TestCNIPRToolLegalStatus(t *testing.T) {
	session := &fakeCNIPRSession{
		available: true,
		output:    `EGO_CNIPR_LEGAL:{"items":[{"number":"202311356097.5","appNumber":"202311356097.5","text":"申请号：202311356097.5 法律状态公告日：2024.05.01 法律状态：授权 法律状态信息：授权公告 共 1 条记录"}]}`,
	}
	result, err := runCNIPRTool(t, session, `{"mode":"legal_status","numbers":["202311356097.5"]}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if len(result.Legal) != 1 || result.Legal[0].Total == nil || *result.Legal[0].Total != 1 {
		t.Fatalf("legal = %+v", result.Legal)
	}
	if !strings.Contains(result.Rendered, "共 1 条记录") || !strings.Contains(result.Rendered, "授权") {
		t.Errorf("rendered = %s", result.Rendered)
	}
}

func TestCNIPRToolFailureGivesFallback(t *testing.T) {
	_, err := runCNIPRTool(t, &fakeCNIPRSession{available: false, reason: "未安装 ego lite"}, `{"query":"x"}`)
	if err == nil {
		t.Fatal("ego 不可用时应报错")
	}
	for _, want := range []string{"setup_required", "patent_web_search"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err 缺少 %q：%v", want, err)
		}
	}
}

func TestCNIPRToolRejectsBadArgs(t *testing.T) {
	session := &fakeCNIPRSession{available: true}
	if _, err := runCNIPRTool(t, session, `{"mode":"unknown"}`); err == nil {
		t.Error("未知 mode 应报错")
	}
	if _, err := runCNIPRTool(t, session, `{"mode":"search"}`); err == nil {
		t.Error("search 缺 query 应报错")
	}
	if _, err := runCNIPRTool(t, session, `{"mode":"pdf_links"}`); err == nil {
		t.Error("pdf_links 缺 numbers 应报错")
	}
}
