package browserproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type CloudBrowserProvider interface {
	ProviderName() string
	IsConfigured() bool
	CreateSession(taskID string) (map[string]string, error)
	CloseSession(sessionID string) error
	EmergencyCleanup(sessionID string)
}

type CloudSessionInfo struct {
	SessionName string
	SessionID   string
	CDPURL      string
	Features    map[string]bool
}

func GetEnv(key string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func GetEnvBool(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	return val == "true" || val == "1" || val == "yes"
}

// closeSessionRequest sends a provider-specific session close request with the
// shared marshal → request → error-body pattern used by Browserbase and
// Browser Use.
func closeSessionRequest(httpClient *http.Client, method, url, apiKey, apiKeyHeader string, reqBody any, errPrefix string) error {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(apiKeyHeader, apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max error body
		return fmt.Errorf("%s error %d: %s", errPrefix, resp.StatusCode, string(body))
	}

	return nil
}
