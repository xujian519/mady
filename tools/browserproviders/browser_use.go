package browserproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// BrowserUseProvider 实现 Browser Use（browser-use.com）云浏览器提供商。
type BrowserUseProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewBrowserUseProvider 从 BROWSER_USE_API_KEY 环境变量构造 BrowserUseProvider。
func NewBrowserUseProvider() *BrowserUseProvider {
	return &BrowserUseProvider{
		apiKey:     os.Getenv("BROWSER_USE_API_KEY"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ProviderName 返回提供商标识 "browser_use"。
func (p *BrowserUseProvider) ProviderName() string {
	return "browser_use"
}

// IsConfigured 报告是否已配置 BROWSER_USE_API_KEY。
func (p *BrowserUseProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// CreateSession 在 Browser Use 上创建浏览器会话，返回 session_id 与 cdp_url。
func (p *BrowserUseProvider) CreateSession(taskID string) (map[string]string, error) {
	reqBody := map[string]any{
		"idempotency_key": "agent-" + taskID,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.browser-use.com/v3/browsers", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max error body
		return nil, fmt.Errorf("browser_use API error %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	sessionID, _ := result["id"].(string)
	cdpURL, _ := result["cdpUrl"].(string)
	if cdpURL == "" {
		cdpURL, _ = result["connectUrl"].(string)
	}

	return map[string]string{
		"session_id": sessionID,
		"cdp_url":    cdpURL,
	}, nil
}

// CloseSession 优雅关闭指定会话（PATCH stop）。
func (p *BrowserUseProvider) CloseSession(sessionID string) error {
	return closeSessionRequest(p.httpClient,
		"PATCH",
		fmt.Sprintf("https://api.browser-use.com/v3/browsers/%s", sessionID),
		p.apiKey, "x-api-key",
		map[string]any{"action": "stop"},
		"browser_use close",
	)
}

// EmergencyCleanup 尽力关闭会话，失败时静默忽略（用于紧急清理路径）。
func (p *BrowserUseProvider) EmergencyCleanup(sessionID string) {
	_ = p.CloseSession(sessionID)
}
