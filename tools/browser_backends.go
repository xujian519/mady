package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/chromedp/chromedp"

	"github.com/xujian519/mady/tools/browserproviders"
)

func (bm *BrowserManager) createCloudSession(ctx context.Context, session *BrowserSession) error {
	if bm.cloudProvider == nil || !bm.cloudProvider.IsConfigured() {
		return fmt.Errorf("cloud provider not configured")
	}

	result, err := bm.cloudProvider.CreateSession(session.sessionID)
	if err != nil {
		return fmt.Errorf("cloud session creation failed: %w", err)
	}

	session.backendType = DetectBackend(&bm.config)
	session.cloudProvider = bm.cloudProvider
	session.cloudSessionID = result["session_id"]
	session.cdpURL = result["cdp_url"]
	if session.cdpURL == "" {
		return fmt.Errorf("cloud provider did not return a CDP URL")
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, session.cdpURL)
	browserCtx, cancel := chromedp.NewContext(allocCtx)
	session.ctx = browserCtx
	session.cancel = func() {
		cancel()
		allocCancel()
	}

	if session.cdpURL != "" && bm.config.DialogPolicy != "" {
		supervisor := NewCDPSupervisor(session.cdpURL, session.sessionID, bm.config.DialogPolicy, bm.config.DialogTimeout)
		if err := supervisor.Start(ctx); err == nil {
			session.supervisor = supervisor
		}
	}

	return nil
}

func (bm *BrowserManager) createCDPSession(ctx context.Context, session *BrowserSession) error {
	session.backendType = BackendCDP
	session.cdpURL = bm.config.CDPURL
	if session.cdpURL == "" {
		return fmt.Errorf("cdp url is required")
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, session.cdpURL)
	browserCtx, cancel := chromedp.NewContext(allocCtx)
	session.ctx = browserCtx
	session.cancel = func() {
		cancel()
		allocCancel()
	}

	if bm.config.DialogPolicy != "" {
		supervisor := NewCDPSupervisor(bm.config.CDPURL, session.sessionID, bm.config.DialogPolicy, bm.config.DialogTimeout)
		if err := supervisor.Start(ctx); err != nil {
			return fmt.Errorf("cdp supervisor failed: %w", err)
		}
		session.supervisor = supervisor
	}

	return nil
}

func (bm *BrowserManager) createLightpandaSession(ctx context.Context, session *BrowserSession, targetURL string) error {
	if bm.lightpandaMgr == nil {
		bm.lightpandaMgr = NewLightpandaManager()
	}

	proc, err := bm.lightpandaMgr.StartProcess(ctx, session.sessionID, bm.config.Headless)
	if err != nil {
		return fmt.Errorf("lightpanda start failed: %w", err)
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, proc.CDPURL)
	browserCtx, cancel := chromedp.NewContext(allocCtx)

	session.backendType = BackendLightpanda
	session.lightpandaProc = proc
	session.cdpURL = proc.CDPURL
	session.ctx = browserCtx
	session.cancel = func() {
		cancel()
		allocCancel()
	}

	if bm.config.DialogPolicy != "" {
		supervisor := NewCDPSupervisor(proc.CDPURL, session.sessionID, bm.config.DialogPolicy, bm.config.DialogTimeout)
		if err := supervisor.Start(ctx); err == nil {
			session.supervisor = supervisor
		}
	}

	if targetURL != "" {
		timeoutCtx, cancel := context.WithTimeout(browserCtx, bm.config.CommandTimeout)
		if err := chromedp.Run(timeoutCtx, chromedp.Navigate(targetURL)); err != nil {
			cancel()
			return fmt.Errorf("lightpanda navigation failed: %w", err)
		}
		cancel()
		session.url = targetURL
	}

	return nil
}

func (bm *BrowserManager) createCloudSessionWithProvider(ctx context.Context, session *BrowserSession, provider browserproviders.CloudBrowserProvider) error {
	if provider == nil || !provider.IsConfigured() {
		return fmt.Errorf("cloud provider not configured")
	}

	result, err := provider.CreateSession(session.sessionID)
	if err != nil {
		return fmt.Errorf("cloud session creation failed: %w", err)
	}

	session.cloudProvider = provider
	session.cloudSessionID = result["session_id"]
	session.cdpURL = result["cdp_url"]

	if session.cdpURL == "" {
		return fmt.Errorf("cloud provider did not return a CDP URL")
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, session.cdpURL)
	browserCtx, cancel := chromedp.NewContext(allocCtx)
	session.ctx = browserCtx
	session.cancel = func() {
		cancel()
		allocCancel()
	}

	if bm.config.DialogPolicy != "" {
		supervisor := NewCDPSupervisor(session.cdpURL, session.sessionID, bm.config.DialogPolicy, bm.config.DialogTimeout)
		if err := supervisor.Start(ctx); err == nil {
			session.supervisor = supervisor
		}
	}

	return nil
}

func (bm *BrowserManager) createCamofoxSession(ctx context.Context, session *BrowserSession, targetURL string) error {
	if bm.camofoxClient == nil {
		bm.camofoxClient = CamofoxFromEnv()
	}
	if bm.camofoxClient == nil {
		return fmt.Errorf("camofox not configured")
	}

	tab, err := bm.camofoxClient.CreateTab(session.sessionID, targetURL)
	if err != nil {
		return fmt.Errorf("camofox create tab failed: %w", err)
	}

	session.backendType = BackendCamofox
	session.camofoxClient = bm.camofoxClient
	session.url = tab.URL

	return nil
}

func (bm *BrowserManager) createAgentBrowserSession(ctx context.Context, session *BrowserSession) error {
	if bm.agentBrowserMgr == nil {
		bm.agentBrowserMgr = NewAgentBrowserManager()
	}

	abSession, err := bm.agentBrowserMgr.EnsureSession(ctx, session.sessionID)
	if err != nil {
		return fmt.Errorf("agent-browser session failed: %w", err)
	}

	session.backendType = BackendAgentBrowser
	session.cdpURL = abSession.CDPURL

	if abSession.CDPURL != "" {
		allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, abSession.CDPURL)
		browserCtx, cancel := chromedp.NewContext(allocCtx)
		session.ctx = browserCtx
		session.cancel = func() {
			cancel()
			allocCancel()
		}

		if bm.config.DialogPolicy != "" {
			supervisor := NewCDPSupervisor(abSession.CDPURL, session.sessionID, bm.config.DialogPolicy, bm.config.DialogTimeout)
			if err := supervisor.Start(ctx); err == nil {
				session.supervisor = supervisor
			}
		}
	}

	return nil
}

func (bm *BrowserManager) createLocalSession(ctx context.Context, session *BrowserSession) error {
	vpWidth := bm.config.ViewportWidth
	vpHeight := bm.config.ViewportHeight
	if vpWidth <= 0 {
		vpWidth = 1200 + cryptoIntn(201)
	}
	if vpHeight <= 0 {
		vpHeight = 700 + cryptoIntn(151)
	}

	chrome, err := FindChrome()
	if err != nil {
		return fmt.Errorf("chrome not found: %w - set CHROME_PATH env var", err)
	}

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chrome.Path),
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("headless", fmt.Sprintf("%v", bm.config.Headless)),
		chromedp.WindowSize(vpWidth, vpHeight),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "Translate,MediaRouter"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("hide-crash-restore-bubble", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("remote-debugging-port", "0"),
	}

	if ua := bm.config.UserAgent; ua != "" {
		opts = append(opts, chromedp.Flag("user-agent", ua))
	} else if ua := os.Getenv("BROWSER_USER_AGENT"); ua != "" {
		opts = append(opts, chromedp.Flag("user-agent", ua))
	}

	acceptLang := bm.config.AcceptLanguage
	if acceptLang == "" {
		acceptLang = os.Getenv("BROWSER_ACCEPT_LANGUAGE")
	}
	if acceptLang == "" {
		acceptLang = "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7"
	}
	opts = append(opts, chromedp.Flag("accept-lang", acceptLang))

	if proxyURL := bm.config.ProxyURL; proxyURL != "" {
		opts = append(opts, chromedp.Flag("proxy-server", proxyURL), chromedp.Flag("proxy-bypass-list", "localhost;127.0.0.1;[::1]"))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	browserCtx, cancel := chromedp.NewContext(allocCtx)

	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		allocCancel()
		return fmt.Errorf("failed to start local browser (path=%s): %w", chrome.Path, err)
	}

	session.ctx = browserCtx
	session.cancel = func() {
		cancel()
		allocCancel()
	}
	session.backendType = BackendLocal

	return nil
}
