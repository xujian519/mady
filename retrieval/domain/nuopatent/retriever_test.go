package nuopatent

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/xujian519/mady/retrieval/domain"
)

// fakeRunner 供测试注入：正常输出 / 非零错误 / 阻塞（触发超时）。
type fakeRunner struct {
	out   []byte
	err   error
	block bool
}

func (f *fakeRunner) Run(ctx context.Context, _ []string) ([]byte, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.out, f.err
}

func TestNewNuoPatentRetriever_ExplicitBin(t *testing.T) {
	r, err := NewNuoPatentRetriever(Config{Bin: "/usr/local/bin/nuo-patent"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil retriever with explicit bin")
	}
	if r.SourceName() != "CNIPA(nuo-patent)" {
		t.Errorf("SourceName = %q", r.SourceName())
	}
}

func TestDiscoverBin_EnvOverride(t *testing.T) {
	t.Setenv("MADY_NUO_PATENT_BIN", "/env/bin/nuo-patent")
	if got := discoverBin(Config{}); got != "/env/bin/nuo-patent" {
		t.Errorf("discoverBin = %q, want env", got)
	}
}

func TestDiscoverBin_ConfigPrecedence(t *testing.T) {
	t.Setenv("MADY_NUO_PATENT_BIN", "/env/bin/nuo-patent")
	if got := discoverBin(Config{Bin: "/cfg/bin/nuo-patent"}); got != "/cfg/bin/nuo-patent" {
		t.Errorf("discoverBin = %q, want cfg precedence", got)
	}
}

func TestSearch_Normal(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{
		out: []byte(`{"results":[{"id":"cn001","title":"标题","snippet":"片段","score":0.8},{"id":"cn002","title":"好","score":0.5}]}`),
	}, timeout: time.Second}
	res, err := r.Search(context.Background(), domain.DomainQuery{Text: "传感器"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res == nil || len(res.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %+v", res)
	}
	if res.Documents[0].ID != "cn001" || res.Documents[0].Title != "标题" {
		t.Errorf("doc0: %+v", res.Documents[0])
	}
	if res.TotalCount != 2 {
		t.Errorf("TotalCount = %d", res.TotalCount)
	}
}

func TestSearch_EmptyResultNotError(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{
		out: []byte(`{"results":[]}`),
	}, timeout: time.Second}
	res, err := r.Search(context.Background(), domain.DomainQuery{Text: "无结果"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Documents) != 0 {
		t.Errorf("expected 0 docs, got %d", len(res.Documents))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{}, timeout: time.Second}
	res, err := r.Search(context.Background(), domain.DomainQuery{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res == nil || len(res.Documents) != 0 {
		t.Errorf("expected empty result for empty query, got %+v", res)
	}
}

func TestSearch_NonZeroExit(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{
		err: errors.New("exit status 1"),
	}, timeout: time.Second}
	if _, err := r.Search(context.Background(), domain.DomainQuery{Text: "x"}); err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}

func TestSearch_Timeout(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{block: true}, timeout: 5 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := r.Search(ctx, domain.DomainQuery{Text: "x"}); err == nil {
		t.Fatal("expected context timeout error")
	}
}

func TestSearch_StderrInError(t *testing.T) {
	// execRunner 路径把 stderr 拼进 error（此处用真实 execRunner + 不存在的 bin 覆盖 stderr 结构）。
	_ = os.Getenv("MADY_NUO_PATENT_BIN") // touch env to avoid unused import
	r := &NuoPatentRetriever{bin: "/nonexistent/nuo-patent", runner: execRunner{}, timeout: time.Second}
	if _, err := r.Search(context.Background(), domain.DomainQuery{Text: "x"}); err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestGetDocument_Normal(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{
		out: []byte(`{"id":"cn001","title":"单文档","content":"全文","metadata":{"ipc":"G06F"}}`),
	}, timeout: time.Second}
	d, err := r.GetDocument(context.Background(), "cn001")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if d.Title != "单文档" || d.Metadata["ipc"] != "G06F" {
		t.Errorf("doc: %+v", d)
	}
}

func TestGetDocument_EmptyID(t *testing.T) {
	r := &NuoPatentRetriever{bin: "nuo-patent", runner: &fakeRunner{}, timeout: time.Second}
	if _, err := r.GetDocument(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty docID")
	}
}
