package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/xujian519/mady/pkg/csync"
)

var (
	defaultBrowserManagerRegistry = csync.NewMap[string, *BrowserManager]()
	reHostname                    = regexp.MustCompile(`https?://([^/]+)`)
)

// RegisterBrowserManager 将 mgr 注册到 id 标识的全局注册表，供 EmergencyCleanupAll 使用。
func RegisterBrowserManager(id string, mgr *BrowserManager) {
	defaultBrowserManagerRegistry.Set(id, mgr)
}

// UnregisterBrowserManager 从全局注册表移除指定 id 的 manager。
func UnregisterBrowserManager(id string) {
	defaultBrowserManagerRegistry.Del(id)
}

// EmergencyCleanupAll 关闭所有已注册 manager 的会话并停止所有 CDP supervisor。
func EmergencyCleanupAll() {
	for _, mgr := range defaultBrowserManagerRegistry.Copy() {
		mgr.CloseAll()
		mgr.Stop()
	}
	GetSupervisorRegistry().StopAll()
}

// SetupSignalHandler 注册 SIGINT/SIGTERM 处理，收到信号时调用 EmergencyCleanupAll。
func SetupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		EmergencyCleanupAll()
	}()
}

// OrphanReaper 扫描临时目录中残留的 agent-browser socket 目录，清理归属进程已退出的孤儿。
type OrphanReaper struct {
	mu          sync.Mutex
	socketDir   string
	reapedCount int
}

// NewOrphanReaper 创建使用 os.TempDir() 作为扫描目录的孤儿清理器。
func NewOrphanReaper() *OrphanReaper {
	socketDir := os.TempDir()
	return &OrphanReaper{
		socketDir: socketDir,
	}
}

// ReapOrphans 扫描并清理孤儿 agent-browser 进程，返回清理数量。
func (r *OrphanReaper) ReapOrphans() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.socketDir)
	if err != nil {
		return 0, err
	}

	reaped := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "agent-browser-") {
			continue
		}

		dirPath := filepath.Join(r.socketDir, entry.Name())
		ownerPIDPath := filepath.Join(dirPath, "owner_pid")

		pidData, err := os.ReadFile(ownerPIDPath) //nolint:gosec // G304: path under managed socket directory, not user-controlled
		if err != nil {
			continue
		}

		var ownerPID int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &ownerPID)

		if ownerPID > 0 && isProcessAlive(ownerPID) {
			continue
		}

		daemonPIDPath := filepath.Join(dirPath, "daemon_pid")
		if daemonPIDData, err := os.ReadFile(daemonPIDPath); err == nil { //nolint:gosec // G304: path under managed socket directory
			var daemonPID int
			_, _ = fmt.Sscanf(strings.TrimSpace(string(daemonPIDData)), "%d", &daemonPID)
			if daemonPID > 0 {
				killProcess(daemonPID)
			}
		}

		_ = os.RemoveAll(dirPath)
		reaped++
	}

	r.reapedCount += reaped
	return reaped, nil
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		// 找不到进程句柄→按「已退出」处理（回收器只需判断存活）。
		return false
	}

	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func killProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		// 找不到进程句柄→无需清理，直接返回。
		return
	}
	_ = proc.Signal(syscall.SIGTERM)

	time.Sleep(2 * time.Second)

	if isProcessAlive(pid) {
		_ = proc.Signal(syscall.SIGKILL)
	}
}

// NeedsLightpandaFallback 判断 lightpanda 后端是否需要回退到 Chrome（命令错误/空快照/截图过小）。
func NeedsLightpandaFallback(engine string, snapshot string, screenshotSize int, commandError error) bool {
	if engine != "lightpanda" && engine != "auto" {
		return false
	}

	if commandError != nil {
		return true
	}

	if len(snapshot) < 20 && strings.TrimSpace(snapshot) == "" {
		return true
	}

	if screenshotSize > 0 && screenshotSize < 20480 {
		return true
	}

	return false
}

// AnnotateLightpandaFallback 在 result 中标记 lightpanda→chrome 回退（传 nil 则仅记录全局标记）。
func AnnotateLightpandaFallback(result map[string]any) {
	result["browser_engine_fallback"] = "chrome"
	result["fallback_warning"] = "Lightpanda returned empty/broken result, automatically retried with Chrome"
}

// RunChromeFallbackCommand 用本地 Chrome 执行 lightpanda 回退命令（navigate / screenshot）。
func RunChromeFallbackCommand(ctx context.Context, command string, args map[string]any, timeout time.Duration) (map[string]any, error) {
	chrome, err := FindChrome()
	if err != nil {
		return nil, fmt.Errorf("chrome not found for fallback: %w", err)
	}
	execPath := chrome.Path

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Headless,
		chromedp.WindowSize(1280, 800),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-sync", true),
	)

	if ua := os.Getenv("BROWSER_USER_AGENT"); ua != "" {
		opts = append(opts, chromedp.Flag("user-agent", ua))
	}
	if proxyURL := os.Getenv("BROWSER_PROXY_URL"); proxyURL != "" {
		opts = append(opts, chromedp.Flag("proxy-server", proxyURL), chromedp.Flag("proxy-bypass-list", "localhost;127.0.0.1;[::1]"))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	timeoutCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	var result map[string]any

	switch command {
	case "navigate":
		url, _ := args["url"].(string)
		if err := chromedp.Do(timeoutCtx,
			chromedp.Navigate(url),
			chromedp.WaitReady(chromedp.CSS("body")),
		); err != nil {
			return nil, err
		}
		title, err := chromedp.Run(timeoutCtx, chromedp.Title())
		if err != nil {
			return nil, err
		}

		snapshot, err := generateSnapshot(timeoutCtx, false, nil)
		result = map[string]any{
			"url":      url,
			"title":    title,
			"snapshot": snapshot,
		}
		if err != nil {
			result["snapshot_error"] = err.Error()
		}

	case "screenshot":
		buf, err := chromedp.Run(timeoutCtx, chromedp.CaptureScreenshot())
		if err != nil {
			return nil, err
		}
		result = map[string]any{
			"screenshot": base64.StdEncoding.EncodeToString(buf),
			"size_bytes": len(buf),
		}

	default:
		return nil, fmt.Errorf("unsupported fallback command: %s", command)
	}

	return result, nil
}

// ValidatePostRedirectURL 校验导航后的最终 URL 是否被重定向到元数据端点或私有地址。
func ValidatePostRedirectURL(finalURL string, originalURL string, allowPrivate bool, autoLocal bool) error {
	if finalURL == "" {
		return nil
	}

	if isMetadataEndpoint(getHostname(finalURL)) {
		return fmt.Errorf("navigation redirected to cloud metadata endpoint (%s), blocked for security", finalURL)
	}

	if !allowPrivate && !autoLocal && IsPrivateURL(finalURL) {
		return fmt.Errorf("navigation redirected to private/internal address (%s), blocked for security", finalURL)
	}

	return nil
}

func getHostname(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		if matches := reHostname.FindStringSubmatch(rawURL); len(matches) > 1 {
			return matches[1]
		}
	}
	return rawURL
}
