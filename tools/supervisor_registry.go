package tools

import (
	"context"
	"sync"
	"time"

	"github.com/xujian519/mady/pkg/csync"
)

type SupervisorRegistry struct {
	supervisors csync.Map[string, *CDPSupervisor]
}

var (
	defaultSupervisorRegistry   = &SupervisorRegistry{}
	defaultSupervisorRegistryMu sync.RWMutex
)

func SetSupervisorRegistry(r *SupervisorRegistry) {
	defaultSupervisorRegistryMu.Lock()
	defer defaultSupervisorRegistryMu.Unlock()
	defaultSupervisorRegistry = r
}

func GetSupervisorRegistry() *SupervisorRegistry {
	defaultSupervisorRegistryMu.RLock()
	defer defaultSupervisorRegistryMu.RUnlock()
	return defaultSupervisorRegistry
}

func (r *SupervisorRegistry) GetOrStart(taskID string, cdpURL string, dialogPolicy DialogPolicy, dialogTimeout time.Duration) (*CDPSupervisor, error) {
	// Fast path: already exists
	if existing, ok := r.supervisors.Get(taskID); ok {
		return existing, nil
	}

	supervisor := NewCDPSupervisor(cdpURL, taskID, dialogPolicy, dialogTimeout)
	ctx := context.Background()
	if err := supervisor.Start(ctx); err != nil {
		return nil, err
	}

	// GetOrSet returns the existing value if present, or stores and returns the new one.
	actual := r.supervisors.GetOrSet(taskID, func() *CDPSupervisor { return supervisor })
	if actual != supervisor {
		// Another goroutine stored first — clean up our extra.
		supervisor.Stop()
		return actual, nil
	}
	return supervisor, nil
}

func (r *SupervisorRegistry) Get(taskID string) (*CDPSupervisor, bool) {
	return r.supervisors.Get(taskID)
}

func (r *SupervisorRegistry) Stop(taskID string) {
	if s, ok := r.supervisors.Get(taskID); ok {
		s.Stop()
		r.supervisors.Del(taskID)
	}
}

func (r *SupervisorRegistry) StopAll() {
	for id, s := range r.supervisors.Copy() {
		s.Stop()
		r.supervisors.Del(id)
	}
}

type BrowserBackendType string

const (
	BackendLocal        BrowserBackendType = "local"
	BackendCDP          BrowserBackendType = "cdp"
	BackendCamofox      BrowserBackendType = "camofox"
	BackendBrowserbase  BrowserBackendType = "browserbase"
	BackendBrowserUse   BrowserBackendType = "browser_use"
	BackendFirecrawl    BrowserBackendType = "firecrawl"
	BackendLightpanda   BrowserBackendType = "lightpanda"
	BackendAgentBrowser BrowserBackendType = "agent_browser"
	BackendEgoLite      BrowserBackendType = "egolite"
)

type BrowserConfig struct {
	Headless            bool
	AllowPrivate        bool
	CommandTimeout      time.Duration
	CDPURL              string
	CamofoxURL          string
	CloudProvider       string
	Engine              string
	DialogPolicy        DialogPolicy
	DialogTimeout       time.Duration
	AutoLocalForPrivate bool
	RecordSessions      bool
	RecordingDir        string
	InactivityTimeout   time.Duration
	UserAgent           string
	AcceptLanguage      string
	ProxyURL            string
	ViewportWidth       int
	ViewportHeight      int
	AgentBrowserEnabled bool
	// EgoLite 配置
	EgoLiteEnabled  bool
	EgoLiteTaskName string
}

func (c *BrowserConfig) defaults() {
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = 30 * time.Second
	}
	if c.DialogTimeout <= 0 {
		c.DialogTimeout = 300 * time.Second
	}
	if c.InactivityTimeout <= 0 {
		c.InactivityTimeout = 5 * time.Minute
	}
	if c.DialogPolicy == "" {
		c.DialogPolicy = DialogMustRespond
	}
}

func DetectBackend(cfg *BrowserConfig) BrowserBackendType {
	if cfg.EgoLiteEnabled {
		return BackendEgoLite
	}
	if cfg.CDPURL != "" {
		return BackendCDP
	}
	if cfg.CamofoxURL != "" {
		return BackendCamofox
	}
	if cfg.AgentBrowserEnabled {
		return BackendAgentBrowser
	}
	if cfg.Engine == "lightpanda" {
		return BackendLightpanda
	}
	switch cfg.CloudProvider {
	case "browserbase":
		return BackendBrowserbase
	case "browser_use":
		return BackendBrowserUse
	case "firecrawl":
		return BackendFirecrawl
	case "local":
		return BackendLocal
	default:
		return BackendLocal
	}
}
