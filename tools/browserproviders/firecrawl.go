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

// FirecrawlProvider 实现 Firecrawl 云浏览器提供商。
type FirecrawlProvider struct {
	apiKey     string
	apiURL     string
	ttl        int
	httpClient *http.Client
}

// NewFirecrawlProvider 从环境变量构造 FirecrawlProvider。
func NewFirecrawlProvider() *FirecrawlProvider {
	ttl := 300
	if envTTL := os.Getenv("FIRECRAWL_BROWSER_TTL"); envTTL != "" {
		if t, err := strconv.Atoi(envTTL); err == nil {
			ttl = t
		}
	}

	apiURL := "https://api.firecrawl.dev"
	if envURL := os.Getenv("FIRECRAWL_API_URL"); envURL != "" {
		apiURL = envURL
	}

	return &FirecrawlProvider{
		apiKey:     os.Getenv("FIRECRAWL_API_KEY"),
		apiURL:     apiURL,
		ttl:        ttl,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ProviderName 返回提供商标识 "firecrawl"。
func (p *FirecrawlProvider) ProviderName() string {
	return "firecrawl"
}

// IsConfigured 报告是否已配置 FIRECRAWL_API_KEY。
func (p *FirecrawlProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// CreateSession 在 Firecrawl 上创建浏览器会话，返回 session_id 与 cdp_url。
func (p *FirecrawlProvider) CreateSession(taskID string) (map[string]string, error) {
	reqBody := map[string]any{
		"ttl": p.ttl,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", p.apiURL+"/v2/browser", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max error body
		return nil, fmt.Errorf("firecrawl API error %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	sessionID, _ := result["id"].(string)
	cdpURL, _ := result["cdpUrl"].(string)

	return map[string]string{
		"session_id": sessionID,
		"cdp_url":    cdpURL,
	}, nil
}

// CloseSession 删除指定 Firecrawl 浏览器会话。
func (p *FirecrawlProvider) CloseSession(sessionID string) error {
	req, err := http.NewRequestWithContext(context.Background(), "DELETE", fmt.Sprintf("%s/v2/browser/%s", p.apiURL, sessionID), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max error body
		return fmt.Errorf("firecrawl close error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// EmergencyCleanup 尽力删除会话，失败时静默忽略（用于紧急清理路径）。
func (p *FirecrawlProvider) EmergencyCleanup(sessionID string) {
	_ = p.CloseSession(sessionID)
}
