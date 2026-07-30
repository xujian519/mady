package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/xujian519/mady/tools/browserproviders"
)

var (
	privateIPBlocks   []*net.IPNet
	privateIPInitOnce sync.Once
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret[_-]?key|access[_-]?token|auth[_-]?token|password|passwd)=`),
	regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`),
	regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36})`),
}

type BrowserSession struct {
	mu             sync.RWMutex
	sessionID      string
	backendType    BrowserBackendType
	cdpURL         string
	cloudProvider  browserproviders.CloudBrowserProvider
	cloudSessionID string
	camofoxClient  *CamofoxClient
	lightpandaProc *LightpandaProcess
	ctx            context.Context
	cancel         context.CancelFunc
	url            string
	title          string
	createdAt      time.Time
	lastActivity   time.Time
	supervisor     *CDPSupervisor
	recorder       *CDPRecorder
	refMapper      *RefMapper
	egoLiteManager *EgoLiteManager
}

type BrowserManager struct {
	mu                     sync.RWMutex
	sessions               map[string]*BrowserSession
	config                 BrowserConfig
	cloudProvider          browserproviders.CloudBrowserProvider
	camofoxClient          *CamofoxClient
	lightpandaMgr          *LightpandaManager
	agentBrowserMgr        *AgentBrowserManager
	egoLiteMgr             *EgoLiteManager
	fallbackCloudProviders []browserproviders.CloudBrowserProvider
	activeSession          string

	ctx    context.Context
	cancel context.CancelFunc
}

func NewBrowserManager(cfg *BrowserConfig) *BrowserManager {
	cfg.defaults()

	ctx, cancel := context.WithCancel(context.Background())
	mgr := &BrowserManager{
		sessions: make(map[string]*BrowserSession),
		config:   *cfg,
		ctx:      ctx,
		cancel:   cancel,
	}

	backend := DetectBackend(&mgr.config)
	switch backend {
	case BackendBrowserbase:
		mgr.cloudProvider = browserproviders.NewBrowserbaseProvider()
	case BackendBrowserUse:
		mgr.cloudProvider = browserproviders.NewBrowserUseProvider()
	case BackendFirecrawl:
		mgr.cloudProvider = browserproviders.NewFirecrawlProvider()
	case BackendCamofox:
		mgr.camofoxClient = NewCamofoxClient(CamofoxConfig{
			BaseURL:    cfg.CamofoxURL,
			UserID:     os.Getenv("CAMOFOX_USER_ID"),
			SessionKey: os.Getenv("CAMOFOX_SESSION_KEY"),
		})
	case BackendLightpanda:
		mgr.lightpandaMgr = NewLightpandaManager()
	case BackendAgentBrowser:
		mgr.agentBrowserMgr = NewAgentBrowserManager()
	case BackendEgoLite:
		var err error
		mgr.egoLiteMgr, err = NewEgoLiteManager(cfg.EgoLiteTaskName)
		if err != nil {
			slog.Warn("egolite: create manager failed, falling back to local", "err", err)
			backend = BackendLocal
			mgr.config.EgoLiteEnabled = false // 持久化回退
		}
	}

	if backend == BackendAgentBrowser {
		return mgr
	}

	if backend == BackendLocal || backend == BackendLightpanda {
		for _, try := range []struct {
			name    string
			factory func() browserproviders.CloudBrowserProvider
		}{
			{"browserbase", func() browserproviders.CloudBrowserProvider { return browserproviders.NewBrowserbaseProvider() }},
			{"browser_use", func() browserproviders.CloudBrowserProvider { return browserproviders.NewBrowserUseProvider() }},
			{"firecrawl", func() browserproviders.CloudBrowserProvider { return browserproviders.NewFirecrawlProvider() }},
		} {
			if try.name == cfg.CloudProvider {
				continue // already the primary provider
			}
			p := try.factory()
			if p != nil && p.IsConfigured() {
				mgr.fallbackCloudProviders = append(mgr.fallbackCloudProviders, p)
			}
		}
	}

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-mgr.ctx.Done():
				return
			case <-ticker.C:
				mgr.CleanupInactiveSessions(cfg.InactivityTimeout)
			}
		}
	}()

	return mgr
}

func (bm *BrowserManager) GetSession(sessionID string) (*BrowserSession, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	s, ok := bm.sessions[sessionID]
	return s, ok
}

func (bm *BrowserManager) GetActiveSession(fallbackSessionID string) (*BrowserSession, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	if bm.activeSession != "" {
		if s, ok := bm.sessions[bm.activeSession]; ok {
			return s, true
		}
	}
	s, ok := bm.sessions[fallbackSessionID]
	return s, ok
}

// ErrNoActiveBrowserSession is returned when no browser session is active.
var ErrNoActiveBrowserSession = fmt.Errorf("no active browser session: call browser_navigate first")

// RequireActiveSession returns the active browser session or an error if none exists.
// It replaces the repeated DefaultBrowserManager().GetActiveSession("default") + error pattern.
func RequireActiveSession() (*BrowserSession, error) {
	bm := DefaultBrowserManager()
	if bm == nil {
		return nil, ErrNoActiveBrowserSession
	}
	session, ok := bm.GetActiveSession("default")
	if !ok {
		return nil, ErrNoActiveBrowserSession
	}
	return session, nil
}

func (bm *BrowserManager) SetActiveSession(sessionID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if s, ok := bm.sessions[sessionID]; ok {
		bm.activeSession = sessionID
		// lastActivity is owned by session.mu (handlers update it there too);
		// never write it under bm.mu to avoid split-lock races.
		s.mu.Lock()
		s.lastActivity = time.Now()
		s.mu.Unlock()
	}
}

func (bm *BrowserManager) CreateSession(ctx context.Context, sessionID string, targetURL string) (*BrowserSession, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if existing, ok := bm.sessions[sessionID]; ok {
		bm.activeSession = sessionID
		existing.mu.Lock()
		existing.lastActivity = time.Now()
		existing.mu.Unlock()
		return existing, nil
	}

	backend := DetectBackend(&bm.config)

	if bm.config.AutoLocalForPrivate && bm.cloudProvider != nil && targetURL != "" {
		if IsPrivateURL(targetURL) {
			backend = BackendLocal
			sessionID += "::local"
		}
	}
	if existing, ok := bm.sessions[sessionID]; ok {
		bm.activeSession = sessionID
		existing.mu.Lock()
		existing.lastActivity = time.Now()
		existing.mu.Unlock()
		return existing, nil
	}

	session := &BrowserSession{
		sessionID:    sessionID,
		backendType:  backend,
		createdAt:    time.Now(),
		lastActivity: time.Now(),
		refMapper:    NewRefMapper(),
	}

	var err error
	switch backend {
	case BackendCDP:
		err = bm.createCDPSession(ctx, session)
	case BackendCamofox:
		err = bm.createCamofoxSession(ctx, session, targetURL)
	case BackendLightpanda:
		err = bm.createLightpandaSession(ctx, session, targetURL)
	case BackendBrowserbase, BackendBrowserUse, BackendFirecrawl:
		err = bm.createCloudSession(ctx, session)
	case BackendAgentBrowser:
		err = bm.createAgentBrowserSession(ctx, session)
	case BackendEgoLite:
		if bm.egoLiteMgr != nil {
			session.egoLiteManager = bm.egoLiteMgr
		}
	case BackendLocal:
		err = bm.createLocalSession(ctx, session)
	default:
		err = bm.createLocalSession(ctx, session)
	}

	if err != nil && len(bm.fallbackCloudProviders) > 0 {
		// Primary backend failed; try cloud providers as fallback.
		for _, fp := range bm.fallbackCloudProviders {
			providerName := fp.ProviderName()
			session.backendType = BackendBrowserbase // generic cloud type
			fallbackErr := bm.createCloudSessionWithProvider(ctx, session, fp)
			if fallbackErr == nil {
				fmt.Fprintf(os.Stderr, "[browser] fallback: %s took over after %s failed (%v)\n", providerName, backend, err)
				err = nil
				break
			}
			fmt.Fprintf(os.Stderr, "[browser] fallback %s also failed (%v)\n", providerName, fallbackErr)
		}
	}

	if err != nil {
		return nil, err
	}

	if bm.config.RecordSessions && bm.config.RecordingDir != "" {
		session.recorder = NewCDPRecorder(bm.config.RecordingDir)
	}

	bm.sessions[sessionID] = session
	bm.activeSession = sessionID
	return session, nil
}

// closeSessionResources tears down a single session's external resources
// (supervisor, recorder, cloud/camofox/lightpanda/agent-browser handles,
// chromedp cancel). It performs slow I/O and MUST be called WITHOUT holding
// bm.mu — callers collect+remove the session under bm.mu, then invoke this.
// emergency=true uses EmergencyCleanup (best-effort) instead of CloseSession
// (graceful), matching the cleanup/CloseAll paths.
func (bm *BrowserManager) closeSessionResources(session *BrowserSession, emergency bool) {
	if session.supervisor != nil {
		session.supervisor.Stop()
	}

	if session.recorder != nil && session.recorder.IsRecording() {
		session.recorder.StopRecording() //nolint:gosec // G104: recorder stop is best-effort cleanup
	}

	if session.cloudProvider != nil && session.cloudSessionID != "" {
		if emergency {
			session.cloudProvider.EmergencyCleanup(session.cloudSessionID)
		} else {
			session.cloudProvider.CloseSession(session.cloudSessionID) //nolint:gosec // G104: close session is best-effort cleanup
		}
	}

	if session.camofoxClient != nil {
		session.camofoxClient.CloseTab(context.Background(), session.sessionID) //nolint:gosec // G104: close tab is best-effort cleanup
	}

	if session.lightpandaProc != nil {
		bm.lightpandaMgr.StopProcess(session.sessionID)
	}

	if session.backendType == BackendAgentBrowser && bm.agentBrowserMgr != nil {
		bm.agentBrowserMgr.StopSession(session.sessionID)
	}

	if session.cancel != nil {
		session.cancel()
	}
}

func (bm *BrowserManager) CloseSession(sessionID string) {
	// Collect + remove under bm.mu; tear down outside the lock to avoid
	// blocking other callers on slow external I/O.
	bm.mu.Lock()
	session, ok := bm.sessions[sessionID]
	if ok {
		delete(bm.sessions, sessionID)
		if bm.activeSession == sessionID {
			bm.activeSession = ""
		}
	}
	bm.mu.Unlock()

	if ok {
		bm.closeSessionResources(session, false)
	}
}

func (bm *BrowserManager) CleanupInactiveSessions(timeout time.Duration) {
	now := time.Now()

	// Collect expired sessions + remove from map under bm.mu.
	// lastActivity is owned by session.mu, so read it under session.mu.RLock
	// (not bm.mu).
	bm.mu.Lock()
	var expired []*BrowserSession
	for id, session := range bm.sessions {
		session.mu.RLock()
		last := session.lastActivity
		session.mu.RUnlock()
		if now.Sub(last) > timeout {
			expired = append(expired, session)
			delete(bm.sessions, id)
			if bm.activeSession == id {
				bm.activeSession = ""
			}
		}
	}
	bm.mu.Unlock()

	// Tear down outside the lock — EmergencyCleanup/Stop may block for seconds.
	for _, session := range expired {
		bm.closeSessionResources(session, true)
	}
}

// Stop cancels the background cleanup goroutine. After Stop returns, the
// BrowserManager is safe to discard. CloseAll is preferred for graceful
// shutdown; Stop ensures the ticker goroutine exits even if CloseAll is
// not called.
func (bm *BrowserManager) Stop() {
	bm.cancel()
}

func (bm *BrowserManager) CloseAll() {
	// Snapshot + clear under bm.mu; tear down outside the lock so slow
	// EmergencyCleanup/Stop calls don't block the manager mutex.
	bm.mu.Lock()
	sessions := bm.sessions
	bm.sessions = make(map[string]*BrowserSession)
	bm.activeSession = ""
	bm.mu.Unlock()

	for _, session := range sessions {
		bm.closeSessionResources(session, true)
	}

	if bm.egoLiteMgr != nil {
		bm.egoLiteMgr.Close() //nolint:gosec // G104: close manager is best-effort cleanup
	}

	if bm.agentBrowserMgr != nil {
		bm.agentBrowserMgr.StopAll()
	}

	// Stop the background cleanup ticker goroutine.
	bm.Stop()
}
