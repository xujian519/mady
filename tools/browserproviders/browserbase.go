package browserproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// BrowserbaseProvider 实现 Browserbase 云浏览器提供商。
type BrowserbaseProvider struct {
	apiKey     string
	projectID  string
	proxies    bool
	stealth    bool
	keepAlive  bool
	timeout    int
	httpClient *http.Client
}

// NewBrowserbaseProvider 从环境变量构造 BrowserbaseProvider。
func NewBrowserbaseProvider() *BrowserbaseProvider {
	return &BrowserbaseProvider{
		apiKey:     os.Getenv("BROWSERBASE_API_KEY"),
		projectID:  os.Getenv("BROWSERBASE_PROJECT_ID"),
		proxies:    GetEnvBool("BROWSERBASE_PROXIES", true),
		stealth:    GetEnvBool("BROWSERBASE_ADVANCED_STEALTH", false),
		keepAlive:  GetEnvBool("BROWSERBASE_KEEP_ALIVE", true),
		timeout:    300000,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ProviderName 返回提供商标识 "browserbase"。
func (p *BrowserbaseProvider) ProviderName() string {
	return "browserbase"
}

// IsConfigured 报告是否已配置 BROWSERBASE_API_KEY 与 PROJECT_ID。
func (p *BrowserbaseProvider) IsConfigured() bool {
	return p.apiKey != "" && p.projectID != ""
}

// CreateSession 在 Browserbase 上创建浏览器会话，返回 session_id 与 cdp_url。
// 402（付费墙）时自动降级关闭 proxies / keepAlive 重试。
func (p *BrowserbaseProvider) CreateSession(taskID string) (map[string]string, error) {
	if customTimeout := os.Getenv("BROWSERBASE_SESSION_TIMEOUT"); customTimeout != "" {
		if t, err := strconv.Atoi(customTimeout); err == nil {
			p.timeout = t
		}
	}

	reqBody := map[string]any{
		"projectId": p.projectID,
		"keepAlive": p.keepAlive,
		"timeout":   p.timeout,
	}

	if p.proxies {
		reqBody["proxies"] = true
	}
	if p.stealth {
		reqBody["advancedStealth"] = true
	}

	return p.createSessionWithBody(reqBody)
}

func (p *BrowserbaseProvider) createSessionWithBody(reqBody map[string]any) (map[string]string, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.browserbase.com/v1/sessions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-bb-api-key", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 402 {
		if p.proxies {
			reqBody["proxies"] = false
			return p.createSessionWithBody(reqBody)
		}
		if p.keepAlive {
			reqBody["keepAlive"] = false
			return p.createSessionWithBody(reqBody)
		}
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max error body
		return nil, fmt.Errorf("browserbase API error %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	sessionID, _ := result["id"].(string)
	connectURL, _ := result["connectUrl"].(string)
	debugURL, _ := result["debuggerFullscreenUrl"].(string)

	cdpURL := connectURL
	if cdpURL == "" {
		cdpURL = debugURL
	}

	return map[string]string{
		"session_id": sessionID,
		"cdp_url":    cdpURL,
	}, nil
}

// CloseSession 优雅释放指定会话（REQUEST_RELEASE）。
func (p *BrowserbaseProvider) CloseSession(sessionID string) error {
	return closeSessionRequest(p.httpClient,
		"POST",
		fmt.Sprintf("https://api.browserbase.com/v1/sessions/%s", sessionID),
		p.apiKey, "x-bb-api-key",
		map[string]any{"status": "REQUEST_RELEASE"},
		"browserbase close",
	)
}

// EmergencyCleanup 尽力释放会话，失败时静默忽略（用于紧急清理路径）。
func (p *BrowserbaseProvider) EmergencyCleanup(sessionID string) {
	_ = p.CloseSession(sessionID)
}
