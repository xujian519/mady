package literature

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/pkg/hostlimit"
)

// 下载约束。
const (
	// MaxDownloadBytes 是单篇 PDF 的大小上限（50 MB）。
	MaxDownloadBytes int64 = 50 << 20
	// MinPDFBytes 是判定「真实 PDF」的最小长度；更短的通常是错误页。
	MinPDFBytes = 1024
	// DefaultDownloadTimeout 是整次下载的超时。
	DefaultDownloadTimeout = 60 * time.Second
	pdfUserAgent           = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Downloader 负责直链 PDF 下载与三检（Content-Type、魔数、大小）。
type Downloader struct {
	doer     Doer
	limiter  *hostlimit.Limiter
	timeout  time.Duration
	maxBytes int64
}

// DownloadOptions 配置下载器。
type DownloadOptions struct {
	Doer     Doer
	Timeout  time.Duration
	MaxBytes int64
}

// NewDownloader 创建下载器。
func NewDownloader(opts DownloadOptions) *Downloader {
	doer := opts.Doer
	if doer == nil {
		doer = &http.Client{Timeout: DefaultDownloadTimeout}
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultDownloadTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxDownloadBytes
	}
	return &Downloader{
		doer: doer, limiter: hostlimit.New(), timeout: timeout, maxBytes: maxBytes,
	}
}

// downloadLimit 是 PDF 下载的默认礼貌限速；arXiv 按上游要求放宽到 3 秒。
var downloadLimit = hostlimit.Limit{MinInterval: time.Second, MaxConcurrent: 2}

func downloadLimitFor(rawURL string) hostlimit.Limit {
	if parsed, err := url.Parse(rawURL); err == nil {
		if host := strings.ToLower(parsed.Hostname()); host == "arxiv.org" || strings.HasSuffix(host, ".arxiv.org") {
			return arxivLimit
		}
	}
	return downloadLimit
}

// Fetch 下载并校验 PDF；返回通过三检的字节。
func (d *Downloader) Fetch(ctx context.Context, pdfURL string) ([]byte, error) {
	parsed, err := url.Parse(pdfURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("PDF 链接必须是 http/https：%s", pdfURL)
	}
	release, err := d.limiter.Acquire(ctx, pdfURL, downloadLimitFor(pdfURL))
	if err != nil {
		return nil, err
	}
	defer release()
	requestCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, pdfURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", pdfUserAgent)
	req.Header.Set("Accept", "application/pdf")
	resp, err := d.doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		return nil, fmt.Errorf("返回的是 HTML 页面（%s），不是 PDF", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, d.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > d.maxBytes {
		return nil, fmt.Errorf("PDF 超过 %d MB 上限", d.maxBytes>>20)
	}
	if len(body) < MinPDFBytes {
		return nil, fmt.Errorf("响应过小（%d 字节），可能是错误页", len(body))
	}
	if !strings.HasPrefix(string(body[:5]), "%PDF-") {
		return nil, fmt.Errorf("响应不是 PDF（magic 不匹配）")
	}
	return body, nil
}

// SavePDF 在 root 内的 dir 下写入 PDF；文件名由 title 或 URL 生成，扩展名强制 .pdf。
// dir 为相对 root 的路径，不存在时创建。返回相对 root 的落盘路径。
//
// 写入经过 os.Root：即使校验后目录被替换为指向外部的符号链接，也不会越出 root。
func SavePDF(root *os.Root, dir, title, pdfURL string, data []byte) (string, error) {
	if err := root.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("创建输出目录失败：%w", err)
	}
	path := filepath.Join(dir, PDFFilename(title, pdfURL)+".pdf")
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("创建 PDF 文件失败：%w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		removeErr := root.Remove(path)
		return "", fmt.Errorf("写入 PDF 失败：%w", errors.Join(writeErr, closeErr, removeErr))
	}
	return path, nil
}

// PDFFilename 从标题或链接生成文件名（不含扩展名），清洗后为空时回退 paper。
func PDFFilename(title, pdfURL string) string {
	name := sanitizeFilename(title)
	if name == "" {
		if parsed, err := url.Parse(pdfURL); err == nil {
			name = sanitizeFilename(strings.TrimSuffix(filepath.Base(parsed.Path), ".pdf"))
		}
	}
	if name == "" {
		return "paper"
	}
	return name
}

// sanitizeFilename 保留中文与字母数字，替换路径分隔符与控制字符。
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "\x00", "")
	cleaned := strings.TrimSpace(replacer.Replace(name))
	cleaned = strings.Join(strings.Fields(cleaned), "_")
	runes := []rune(cleaned)
	if len(runes) > 120 {
		cleaned = string(runes[:120])
	}
	return strings.Trim(cleaned, "._")
}
