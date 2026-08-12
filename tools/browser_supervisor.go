package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type DialogPolicy string

const (
	DialogMustRespond DialogPolicy = "must_respond"
	DialogAutoDismiss DialogPolicy = "auto_dismiss"
	DialogAutoAccept  DialogPolicy = "auto_accept"
)

type DialogInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Default string `json:"default"`
}

type FrameInfo struct {
	ID       string       `json:"id"`
	URL      string       `json:"url"`
	Name     string       `json:"name"`
	ParentID string       `json:"parent_id"`
	Children []*FrameInfo `json:"children,omitempty"`
}

type CDPSupervisor struct {
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	cdpURL        string
	taskID        string
	dialogPolicy  DialogPolicy
	dialogTimeout time.Duration

	pendingDialogs map[string]*DialogInfo
	frameTree      []*FrameInfo
	maxFrames      int
	maxDepth       int

	reconnectBackoff time.Duration
	maxReconnects    int
	reconnectCount   int

	listeners []CDPEventListener
}

type CDPEvent struct {
	Type string
	Data any
}

type CDPEventListener func(event CDPEvent)

func NewCDPSupervisor(cdpURL string, taskID string, dialogPolicy DialogPolicy, dialogTimeout time.Duration) *CDPSupervisor {
	if dialogTimeout <= 0 {
		dialogTimeout = 300 * time.Second
	}
	return &CDPSupervisor{
		cdpURL:           cdpURL,
		taskID:           taskID,
		dialogPolicy:     dialogPolicy,
		dialogTimeout:    dialogTimeout,
		pendingDialogs:   make(map[string]*DialogInfo),
		maxFrames:        30,
		maxDepth:         2,
		reconnectBackoff: 2 * time.Second,
		maxReconnects:    5,
	}
}

func (s *CDPSupervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ctx != nil {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	go s.runSupervisor()

	return nil
}

func (s *CDPSupervisor) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// cancel() is idempotent. We intentionally do NOT null out s.ctx/s.cancel:
	// runSupervisor reads s.ctx.Done() without holding the lock, and a nil ctx
	// would panic. Leaving s.ctx pointing at the (now-canceled) context makes
	// s.ctx.Done() return a closed channel, letting runSupervisor exit cleanly.
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *CDPSupervisor) runSupervisor() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		if err := s.connectAndListen(); err != nil {
			s.mu.RLock()
			count := s.reconnectCount
			s.mu.RUnlock()

			if count >= s.maxReconnects {
				return
			}

			s.mu.Lock()
			s.reconnectCount++
			s.mu.Unlock()

			timer := time.NewTimer(s.reconnectBackoff)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				s.mu.Lock()
				s.reconnectBackoff *= 2
				if s.reconnectBackoff > 30*time.Second {
					s.reconnectBackoff = 30 * time.Second
				}
				s.mu.Unlock()
			}
		}
	}
}

func (s *CDPSupervisor) connectAndListen() error {
	s.mu.RLock()
	parentCtx := s.ctx
	cdpURL := s.cdpURL
	s.mu.RUnlock()
	if parentCtx == nil {
		return fmt.Errorf("supervisor context is not initialized")
	}

	var allocCtx context.Context
	var allocCancel context.CancelFunc
	if cdpURL != "" {
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(parentCtx, cdpURL)
	} else {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.NoSandbox,
			chromedp.DisableGPU,
		)
		allocCtx, allocCancel = chromedp.NewExecAllocator(parentCtx, opts...)
	}
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	if err := chromedp.Run(browserCtx); err != nil {
		return fmt.Errorf("browser start failed: %w", err)
	}

	s.mu.Lock()
	s.reconnectCount = 0
	s.reconnectBackoff = 2 * time.Second
	s.mu.Unlock()

	go s.listenForDialogs(browserCtx)
	go s.trackFrameTree(browserCtx)

	select {
	case <-parentCtx.Done():
		return parentCtx.Err()
	case <-browserCtx.Done():
		return browserCtx.Err()
	}
}

func (s *CDPSupervisor) listenForDialogs(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *page.EventJavascriptDialogOpening:
			dialogID := fmt.Sprintf("dlg_%d", time.Now().UnixNano())
			s.mu.Lock()
			s.pendingDialogs[dialogID] = &DialogInfo{
				Type:    string(e.Type),
				Message: e.Message,
				Default: e.DefaultPrompt,
			}
			s.mu.Unlock()

			s.notifyListeners(CDPEvent{Type: "dialog_open", Data: e})

			switch s.dialogPolicy {
			case DialogAutoDismiss:
				s.HandleDialog(dialogID, false, "") //nolint:gosec // G104: fire-and-forget dialog auto-dismiss
			case DialogAutoAccept:
				s.HandleDialog(dialogID, true, "") //nolint:gosec // G104: fire-and-forget dialog auto-accept
			}

		case *page.EventJavascriptDialogClosed:
			s.mu.Lock()
			for id := range s.pendingDialogs {
				delete(s.pendingDialogs, id)
				break
			}
			s.mu.Unlock()

			s.notifyListeners(CDPEvent{Type: "dialog_closed", Data: e})
		}
	})
}

func (s *CDPSupervisor) trackFrameTree(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev.(type) {
		case *target.EventAttachedToTarget:
			s.mu.Lock()
			s.updateFrameTree(ctx)
			s.mu.Unlock()
		case *target.EventDetachedFromTarget:
			s.mu.Lock()
			s.updateFrameTree(ctx)
			s.mu.Unlock()
		}
	})
}

func (s *CDPSupervisor) updateFrameTree(ctx context.Context) {
	var tree *page.FrameTree
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = page.GetFrameTree().Do(ctx)
		return err
	})); err != nil {
		return
	}

	s.frameTree = convertFrameTree(tree, 0, s.maxDepth)
	if len(s.frameTree) > s.maxFrames {
		s.frameTree = s.frameTree[:s.maxFrames]
	}
}

func convertFrameTree(tree *page.FrameTree, depth int, maxDepth int) []*FrameInfo {
	if tree == nil || depth > maxDepth {
		return nil
	}

	frame := &FrameInfo{
		ID:   tree.Frame.ID.String(),
		URL:  tree.Frame.URL,
		Name: tree.Frame.Name,
	}

	var children []*FrameInfo
	for _, child := range tree.ChildFrames {
		children = append(children, convertFrameTree(child, depth+1, maxDepth)...)
	}
	if len(children) > 0 {
		frame.Children = children
	}

	return []*FrameInfo{frame}
}

func (s *CDPSupervisor) GetPendingDialogs() map[string]*DialogInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*DialogInfo)
	for k, v := range s.pendingDialogs {
		result[k] = v
	}
	return result
}

func (s *CDPSupervisor) HandleDialog(dialogID string, accept bool, promptText string) error {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		return fmt.Errorf("supervisor not connected")
	}

	return chromedp.Run(ctx,
		page.HandleJavaScriptDialog(accept).WithPromptText(promptText),
	)
}

func (s *CDPSupervisor) EvaluateJS(expression string, targetFrameID string) (string, error) {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		return "", fmt.Errorf("supervisor not connected")
	}

	var result string
	err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(expression, &result),
	)

	if err != nil {
		return "", fmt.Errorf("JS evaluation failed: %w", err)
	}

	return result, nil
}

func (s *CDPSupervisor) GetFrameTree() []*FrameInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*FrameInfo, len(s.frameTree))
	copy(result, s.frameTree)
	return result
}

func (s *CDPSupervisor) IsTruncated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.frameTree) >= s.maxFrames
}

func (s *CDPSupervisor) AddListener(listener CDPEventListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, listener)
}

func (s *CDPSupervisor) notifyListeners(event CDPEvent) {
	s.mu.RLock()
	listeners := make([]CDPEventListener, len(s.listeners))
	copy(listeners, s.listeners)
	s.mu.RUnlock()

	for _, l := range listeners {
		l(event)
	}
}
