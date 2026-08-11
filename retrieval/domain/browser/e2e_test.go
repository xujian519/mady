package browser

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/xujian519/mady/retrieval/domain"
)

// skipUnlessE2E 跳过未显式开启的 e2e 测试。
//
// e2e 测试真实调用 ego-browser + 外部专利数据库（Google Patents/CNIPA/Espacenet），
// 依赖网络与浏览器运行时，不满足"提交门禁离线可跑"的前提，故默认跳过。
// 设置 MADY_E2E=1 显式开启；IsAvailable 只保证二进制存在（不保证运行时可用），
// 作为第二道闸保留。
func skipUnlessE2E(t *testing.T, cfg *BrowserRetrieverConfig) {
	t.Helper()
	if os.Getenv("MADY_E2E") != "1" {
		t.Skip("e2e tests disabled: set MADY_E2E=1 to run real ego-browser tests")
	}
	if !cfg.IsAvailable() {
		t.Skip("ego-browser not available")
	}
}

// TestE2EGoogleSearch 真实调用 ego-browser 验证搜索闭环（需要 ego lite 环境）。
func TestE2EGoogleSearch(t *testing.T) {
	cfg := DefaultConfig()
	skipUnlessE2E(t, cfg)
	r := NewGooglePatentsRetriever(*cfg)
	if r == nil {
		t.Skip("retriever nil")
	}
	results, err := r.Search(context.Background(), domain.DomainQuery{
		Text:       "深度学习图像识别",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	fmt.Printf("source=%s total=%d\n", results.Source, len(results.Documents))
	for i, d := range results.Documents {
		fmt.Printf("  [%d] %s | %s | %s | score=%.2f\n", i+1, d.ID, d.Title, d.Metadata["assignee"], d.Score)
	}
	if len(results.Documents) == 0 {
		t.Fatal("no documents returned")
	}
}

// TestE2ECNIPASearch 真实调用验证 CNIPA（country:CN）检索。
func TestE2ECNIPASearch(t *testing.T) {
	cfg := DefaultConfig()
	skipUnlessE2E(t, cfg)
	r := NewCNIPARetriever(*cfg)
	if r == nil {
		t.Skip("retriever nil")
	}
	results, err := r.Search(context.Background(), domain.DomainQuery{
		Text:       "图像识别",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	fmt.Printf("source=%s total=%d\n", results.Source, len(results.Documents))
	for i, d := range results.Documents {
		fmt.Printf("  [%d] %s | %s | %s\n", i+1, d.ID, d.Title, d.Metadata["country"])
	}
}

// TestE2EEspacenetSearch 真实调用验证 Espacenet 检索。
func TestE2EEspacenetSearch(t *testing.T) {
	cfg := DefaultConfig()
	skipUnlessE2E(t, cfg)
	r := NewEspacenetRetriever(*cfg)
	if r == nil {
		t.Skip("retriever nil")
	}
	results, err := r.Search(context.Background(), domain.DomainQuery{
		Text:       "deep learning image recognition",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	fmt.Printf("source=%s total=%d\n", results.Source, len(results.Documents))
	for i, d := range results.Documents {
		fmt.Printf("  [%d] %s | %s | %s\n", i+1, d.ID, d.Title, d.Metadata["assignee"])
	}
}

// TestE2EGetDocument 真实调用验证详情页全文提取。
func TestE2EGetDocument(t *testing.T) {
	cfg := DefaultConfig()
	skipUnlessE2E(t, cfg)
	r := NewGooglePatentsRetriever(*cfg)
	if r == nil {
		t.Skip("retriever nil")
	}
	doc, err := r.GetDocument(context.Background(), "CN106599773B")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc == nil {
		t.Fatal("doc is nil")
	}
	fmt.Printf("id=%s title=%s\nabstract: %.80s...\ncontentLen=%d\n",
		doc.ID, doc.Title, doc.Snippet, len(doc.Content))
}
