// Package nuopatent wraps an external `nuo-patent` CLI as a domain.DomainRetriever
// (方案 A：外部 CLI 封装，不移植 TS 实现). The binary is peripheral: when missing,
// NewNuoPatentRetriever returns nil so startup is never blocked. All external
// invocation goes through CommandRunner (test injection point) and argv arrays
// (no shell) to avoid injection.
package nuopatent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/xujian519/mady/retrieval/domain"
)

// Config controls the nuo-patent retriever.
type Config struct {
	// Bin overrides binary discovery. When empty, discovery falls back to the
	// MADY_NUO_PATENT_BIN env var, then exec.LookPath("nuo-patent").
	Bin string
	// Timeout bounds a single CLI invocation (default 60s).
	Timeout time.Duration
}

// CommandRunner runs an argv array and returns stdout. Tests inject a fake.
type CommandRunner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}

// execRunner runs the CLI via exec.CommandContext, capturing stdout/stderr.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, errors.New("nuo-patent: empty argv")
	}
	//nolint:gosec // argv 数组（非 shell 拼接）已防注入；bin 来自受信任发现路径，非用户可控。
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// 非零退出：把 stderr 融入结构化 error，便于上层诊断。
		return nil, fmt.Errorf("nuo-patent: %s 执行失败: %w; stderr=%q", argv[0], err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// NuoPatentRetriever is a pat.retriever backed by the `nuo-patent` CLI.
//
//nolint:revive // 类型名带 NuoPatent 前缀与包名略重叠，但表意清晰且与批次计划文档命名一致
type NuoPatentRetriever struct {
	bin     string
	runner  CommandRunner
	timeout time.Duration
}

// discoverBin 按 cfg.Bin > MADY_NUO_PATENT_BIN > exec.LookPath 顺序发现二进制；
// 全部缺失返回 ""（New 据此返回 nil）。
func discoverBin(cfg Config) string {
	if cfg.Bin != "" {
		return cfg.Bin
	}
	if env := os.Getenv("MADY_NUO_PATENT_BIN"); env != "" {
		return env
	}
	p, _ := exec.LookPath("nuo-patent")
	return p
}

// NewNuoPatentRetriever creates a retriever. Returns nil (no error) when the
// binary is missing, so startup proceeds without the external engine.
func NewNuoPatentRetriever(cfg Config) (*NuoPatentRetriever, error) {
	bin := discoverBin(cfg)
	if bin == "" {
		return nil, nil
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &NuoPatentRetriever{bin: bin, runner: execRunner{}, timeout: timeout}, nil
}

// SourceName identifies the source.
func (r *NuoPatentRetriever) SourceName() string { return "CNIPA(nuo-patent)" }

// Search queries the CLI and normalizes results.
func (r *NuoPatentRetriever) Search(ctx context.Context, query domain.DomainQuery) (*domain.DomainResults, error) {
	q := strings.TrimSpace(query.Text)
	if q == "" && len(query.Keywords) > 0 {
		q = strings.Join(query.Keywords, " ")
	}
	if q == "" {
		// 空查询：空结果而非报错。
		return &domain.DomainResults{Query: query, Source: r.SourceName()}, nil
	}
	if query.MaxResults <= 0 {
		query.MaxResults = 10
	}

	argv := []string{r.bin, "search", q, "--limit", strconv.Itoa(query.MaxResults)}
	out, err := r.run(ctx, argv)
	if err != nil {
		return nil, err
	}
	items, err := parseSearchResults(out)
	if err != nil {
		return nil, err
	}
	docs := make([]domain.DomainDocument, 0, len(items))
	for _, it := range items {
		docs = append(docs, it.toDoc())
	}
	return &domain.DomainResults{
		Query:      query,
		Documents:  docs,
		TotalCount: len(docs),
		Source:     r.SourceName(),
	}, nil
}

// GetDocument fetches one document by ID.
func (r *NuoPatentRetriever) GetDocument(ctx context.Context, docID string) (*domain.DomainDocument, error) {
	if strings.TrimSpace(docID) == "" {
		return nil, errors.New("nuo-patent: docID 不能为空")
	}
	out, err := r.run(ctx, []string{r.bin, "get", docID})
	if err != nil {
		return nil, err
	}
	var d rawDoc
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, fmt.Errorf("nuo-patent: 解析文档 %s 失败: %w", docID, err)
	}
	return d.toDocPtr(), nil
}

// run 施加超时并委托 runner，统一错误包装。
func (r *NuoPatentRetriever) run(ctx context.Context, argv []string) ([]byte, error) {
	if r.runner == nil {
		return nil, errors.New("nuo-patent: runner 未注入")
	}
	cctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out, err := r.runner.Run(cctx, argv)
	if err != nil {
		return nil, fmt.Errorf("nuo-patent: %s: %w", argv[0], err)
	}
	return out, nil
}

// rawDoc is the CLI JSON shape for a single result.
type rawDoc struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Snippet  string            `json:"snippet"`
	Content  string            `json:"content"`
	URL      string            `json:"url"`
	Score    float64           `json:"score"`
	Metadata map[string]string `json:"metadata"`
}

func (d rawDoc) toDoc() domain.DomainDocument {
	return domain.DomainDocument{
		ID:       d.ID,
		Title:    d.Title,
		Snippet:  d.Snippet,
		Content:  d.Content,
		URL:      d.URL,
		Metadata: d.Metadata,
		Score:    d.Score,
	}
}

func (d rawDoc) toDocPtr() *domain.DomainDocument {
	doc := d.toDoc()
	return &doc
}

// parseSearchResults parses the CLI search JSON envelope {results:[...]}.
func parseSearchResults(data []byte) ([]rawDoc, error) {
	var resp struct {
		Results []rawDoc `json:"results"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("nuo-patent: 解析检索结果失败: %w", err)
	}
	return resp.Results, nil
}
