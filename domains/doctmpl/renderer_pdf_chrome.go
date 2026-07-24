package doctmpl

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// PDFChromeRenderer 使用 headless Chrome（chromedp）将 Markdown 渲染为高质量 PDF。
// 内部链路：Markdown → goldmark → HTML（复用 HTMLRenderer）→ Chrome PrintToPDF。
// 相比 PDFRenderer（gopdf 手动布局），Chrome 引擎提供 CSS 级精确排版、
// 表格/代码块/引用块完美还原以及系统级中文字体支持。
//
// 需要系统安装 Chrome/Chromium；若不可用，请使用 PDFAutoRenderer 自动降级。
type PDFChromeRenderer struct {
	// ChromeWSURL 可选：远程 Chrome 的 DevTools WebSocket 地址
	// （如 ws://localhost:9222/devtools/browser/<id>）。
	// 为空时使用本地 headless Chrome（chromedp.NewExecAllocator）。
	ChromeWSURL string

	// Timeout 是单次 PDF 生成的超时秒数。默认 30。
	Timeout int

	// html 是内部复用的 HTML 渲染器。
	html HTMLRenderer
}

// Format 返回 FormatPDF。
func (r *PDFChromeRenderer) Format() OutputFormat { return FormatPDF }

// Render 将 Markdown 正文转换为 PDF 字节流。
// 流程：Markdown → HTML（goldmark + 内嵌 CSS）→ Chrome page.PrintToPDF。
func (r *PDFChromeRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("doctmpl: nil Chrome PDF renderer")
	}

	// 1. Markdown → 完整 HTML 文档（含 CSS）
	htmlBytes, err := r.html.Render(md, meta)
	if err != nil {
		return nil, fmt.Errorf("doctmpl: HTML pre-render for Chrome PDF failed: %w", err)
	}
	htmlStr := string(htmlBytes)

	// 2. 创建 chromedp context
	timeout := time.Duration(r.timeoutSec()) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	allocCtx, allocCancel := r.newAllocator(ctx)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(format string, a ...any) {
			slog.Debug("chromedp", "msg", fmt.Sprintf(format, a...))
		}))
	defer taskCancel()

	// 3. 导航到 about:blank → 注入 HTML → 打印 PDF
	var pdfBuf []byte
	err = chromedp.Run(taskCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return fmt.Errorf("get frame tree: %w", err)
			}
			return page.SetDocumentContent(tree.Frame.ID, htmlStr).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithMarginTop(0.4).
				WithMarginBottom(0.4).
				WithMarginLeft(0.4).
				WithMarginRight(0.4).
				Do(ctx)
			if err != nil {
				return fmt.Errorf("print to PDF: %w", err)
			}
			pdfBuf = buf
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("doctmpl: Chrome PDF generation failed: %w", err)
	}

	return pdfBuf, nil
}

// timeoutSec 返回超时秒数，默认 30。
func (r *PDFChromeRenderer) timeoutSec() int {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 30
}

// newAllocator 创建 chromedp allocator：远程 WS 或本地 exec。
func (r *PDFChromeRenderer) newAllocator(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.ChromeWSURL != "" {
		return chromedp.NewRemoteAllocator(ctx, r.ChromeWSURL)
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	return chromedp.NewExecAllocator(ctx, opts...)
}

// PDFAutoRenderer 自动选择最佳 PDF 渲染路径。
// 首次 Render 时惰性探测本地 Chrome 是否可用：
//   - 可用 → 委托 PDFChromeRenderer（高质量 CSS 排版）
//   - 不可用 → 回退 PDFRenderer（gopdf，零外部依赖）
//
// 探测仅执行一次（sync.Once），后续调用直接走选定路径。
type PDFAutoRenderer struct {
	chrome *PDFChromeRenderer
	gopdf  PDFRenderer

	once      sync.Once
	useChrome bool
}

// NewPDFAutoRenderer 创建自动降级 PDF 渲染器。
func NewPDFAutoRenderer() *PDFAutoRenderer {
	return &PDFAutoRenderer{
		chrome: &PDFChromeRenderer{Timeout: 30},
	}
}

// Format 返回 FormatPDF。
func (r *PDFAutoRenderer) Format() OutputFormat { return FormatPDF }

// Render 委托给 Chrome 或 gopdf 渲染器。
func (r *PDFAutoRenderer) Render(md string, meta RenderMeta) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("doctmpl: nil auto PDF renderer")
	}
	r.once.Do(func() {
		r.useChrome = probeChrome(r.chrome)
		if r.useChrome {
			slog.Info("doctmpl: PDF rendering via headless Chrome")
		} else {
			slog.Info("doctmpl: Chrome not available, PDF rendering via gopdf fallback")
		}
	})
	if r.useChrome {
		return r.chrome.Render(md, meta)
	}
	return r.gopdf.Render(md, meta)
}

// probeChrome 尝试启动 Chrome 并导航到 about:blank，成功返回 true。
func probeChrome(r *PDFChromeRenderer) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	allocCtx, allocCancel := r.newAllocator(ctx)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	return chromedp.Run(taskCtx, chromedp.Navigate("about:blank")) == nil
}
