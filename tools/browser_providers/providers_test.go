package browser_providers

import (
	"os"
	"testing"
)

// withEnv sets env vars for the duration of the test and restores them after.
func withEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	old := make(map[string]string)
	for k, v := range vars {
		old[k] = os.Getenv(k)
		os.Setenv(k, v)
	}
	t.Cleanup(func() {
		for k, v := range old {
			os.Setenv(k, v)
		}
	})
}

// --- BrowserbaseProvider ---

func TestBrowserbase_ProviderName(t *testing.T) {
	p := NewBrowserbaseProvider()
	if p.ProviderName() != "browserbase" {
		t.Fatalf("expected browserbase, got %q", p.ProviderName())
	}
}

func TestBrowserbase_IsConfigured_WhenEnvMissing(t *testing.T) {
	withEnv(t, map[string]string{
		"BROWSERBASE_API_KEY":    "",
		"BROWSERBASE_PROJECT_ID": "",
	})
	p := NewBrowserbaseProvider()
	if p.IsConfigured() {
		t.Fatal("should not be configured when env is missing")
	}
}

func TestBrowserbase_IsConfigured_WhenEnvPresent(t *testing.T) {
	withEnv(t, map[string]string{
		"BROWSERBASE_API_KEY":    "test-key",
		"BROWSERBASE_PROJECT_ID": "test-project",
	})
	p := NewBrowserbaseProvider()
	if !p.IsConfigured() {
		t.Fatal("should be configured when both API_KEY and PROJECT_ID are set")
	}
}

func TestBrowserbase_CreateSession_NotConfigured(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	withEnv(t, map[string]string{
		"BROWSERBASE_API_KEY":    "",
		"BROWSERBASE_PROJECT_ID": "",
	})
	p := NewBrowserbaseProvider()
	_, err := p.CreateSession("task-1")
	// BrowserbaseProvider does not gate CreateSession on IsConfigured — it
	// sends the request and surfaces the upstream HTTP error.
	if err == nil {
		t.Fatal("expected error when API_KEY/PROJECT_ID missing")
	}
}

func TestBrowserbase_EmergencyCleanup_NoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	p := NewBrowserbaseProvider()
	// EmergencyCleanup should not panic even with an invalid session.
	// The call hits the network but we only assert no panic; in CI without
	// network this may fail the HTTP call but the method swallows the error.
	p.EmergencyCleanup("")
}

// --- BrowserUseProvider ---

func TestBrowserUse_ProviderName(t *testing.T) {
	p := NewBrowserUseProvider()
	if p.ProviderName() != "browser_use" {
		t.Fatalf("expected browser_use, got %q", p.ProviderName())
	}
}

func TestBrowserUse_IsConfigured_WhenEnvMissing(t *testing.T) {
	withEnv(t, map[string]string{
		"BROWSER_USE_API_KEY": "",
	})
	p := NewBrowserUseProvider()
	if p.IsConfigured() {
		t.Fatal("should not be configured when API_KEY is missing")
	}
}

func TestBrowserUse_IsConfigured_WhenEnvPresent(t *testing.T) {
	withEnv(t, map[string]string{
		"BROWSER_USE_API_KEY": "test-key",
	})
	p := NewBrowserUseProvider()
	if !p.IsConfigured() {
		t.Fatal("should be configured when API_KEY is set")
	}
}

func TestBrowserUse_EmergencyCleanup_NoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	p := NewBrowserUseProvider()
	p.EmergencyCleanup("")
}

// --- FirecrawlProvider ---

func TestFirecrawl_ProviderName(t *testing.T) {
	p := NewFirecrawlProvider()
	if p.ProviderName() != "firecrawl" {
		t.Fatalf("expected firecrawl, got %q", p.ProviderName())
	}
}

func TestFirecrawl_IsConfigured_WhenEnvMissing(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_API_KEY": "",
	})
	p := NewFirecrawlProvider()
	if p.IsConfigured() {
		t.Fatal("should not be configured when API_KEY is missing")
	}
}

func TestFirecrawl_IsConfigured_WhenEnvPresent(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_API_KEY": "test-key",
	})
	p := NewFirecrawlProvider()
	if !p.IsConfigured() {
		t.Fatal("should be configured when API_KEY is set")
	}
}

func TestFirecrawl_APIURL_Customization(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_API_URL": "https://custom.example.com",
	})
	p := NewFirecrawlProvider()
	if p.apiURL != "https://custom.example.com" {
		t.Fatalf("expected custom API URL, got %q", p.apiURL)
	}
}

func TestFirecrawl_APIURL_Default(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_API_URL": "",
	})
	p := NewFirecrawlProvider()
	if p.apiURL != "https://api.firecrawl.dev" {
		t.Fatalf("expected default API URL, got %q", p.apiURL)
	}
}

func TestFirecrawl_TTL_Customization(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_BROWSER_TTL": "600",
	})
	p := NewFirecrawlProvider()
	if p.ttl != 600 {
		t.Fatalf("expected TTL 600, got %d", p.ttl)
	}
}

func TestFirecrawl_TTL_Default(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_BROWSER_TTL": "",
	})
	p := NewFirecrawlProvider()
	if p.ttl != 300 {
		t.Fatalf("expected default TTL 300, got %d", p.ttl)
	}
}

func TestFirecrawl_TTL_Invalid(t *testing.T) {
	withEnv(t, map[string]string{
		"FIRECRAWL_BROWSER_TTL": "not-a-number",
	})
	p := NewFirecrawlProvider()
	if p.ttl != 300 {
		t.Fatalf("expected default TTL on invalid input, got %d", p.ttl)
	}
}

func TestFirecrawl_EmergencyCleanup_NoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	p := NewFirecrawlProvider()
	p.EmergencyCleanup("")
}

// --- Interface compliance ---

func TestAllProvidersImplementInterface(t *testing.T) {
	var _ CloudBrowserProvider = (*BrowserbaseProvider)(nil)
	var _ CloudBrowserProvider = (*BrowserUseProvider)(nil)
	var _ CloudBrowserProvider = (*FirecrawlProvider)(nil)
}
