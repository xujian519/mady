package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// DialogPolicy 定义浏览器对话框（alert/confirm/prompt）的自动处理策略。
type DialogPolicy string

const (
	DialogMustRespond DialogPolicy = "must_respond"
	DialogAutoDismiss DialogPolicy = "auto_dismiss"
	DialogAutoAccept  DialogPolicy = "auto_accept"
)

// DialogInfo 描述一个待处理的 JavaScript 对话框。
type DialogInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Default string `json:"default"`
}

// FrameInfo 描述页面 frame 树中的一个节点。
type FrameInfo struct {
	ID       string       `json:"id"`
	URL      string       `json:"url"`
	Name     string       `json:"name"`
	ParentID string       `json:"parent_id"`
	Children []*FrameInfo `json:"children,omitempty"`
}

// CDPSupervisor 通过独立 CDP 连接监听页面对话框与 frame 树，并提供
// JS 求值与对话框处理能力，不占用会话主连接。
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

// CDPEvent 是 supervisor 向监听器广播的事件（如 dialog_open / dialog_closed）。
type CDPEvent struct {
	Type string
	Data any
}

// CDPEventListener 是 CDP 事件的监听器回调。
type CDPEventListener func(event CDPEvent)

// NewCDPSupervisor 构造连接到 cdpURL 的 supervisor；dialogTimeout<=0 时默认 300s。
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

// Start 启动 supervisor 的 CDP 监听循环；重复调用幂等。
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

// Stop 停止 supervisor 的 CDP 连接与监听循环。
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
		allocCtx, allocCancel = remote.NewAllocator(parentCtx, cdpURL)
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

	if err := chromedp.Do(browserCtx); err != nil {
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

// targetSession 返回 context 对应的 CDP target 会话；未就绪时返回 nil。
func targetSession(ctx context.Context) *chromedp.Target {
	c := chromedp.FromContext(ctx)
	if c == nil {
		return nil
	}
	return c.Target
}

func (s *CDPSupervisor) listenForDialogs(ctx context.Context) {
	session := targetSession(ctx)
	if session == nil {
		return
	}

	go func() {
		for e, err := range cdp.Events(ctx, session, page.JavascriptDialogOpening) {
			if err != nil {
				return
			}
			dialogID := fmt.Sprintf("dlg_%d", time.Now().UnixNano())
			s.mu.Lock()
			s.pendingDialogs[dialogID] = &DialogInfo{
				Type:    string(e.Type),
				Message: e.Message,
				Default: e.DefaultPrompt,
			}
			s.mu.Unlock()

			s.notifyListeners(CDPEvent{Type: "dialog_open", Data: &e})

			switch s.dialogPolicy {
			case DialogAutoDismiss:
				s.HandleDialog(dialogID, false, "") //nolint:gosec // G104: fire-and-forget dialog auto-dismiss
			case DialogAutoAccept:
				s.HandleDialog(dialogID, true, "") //nolint:gosec // G104: fire-and-forget dialog auto-accept
			}
		}
	}()

	go func() {
		for e, err := range cdp.Events(ctx, session, page.JavascriptDialogClosed) {
			if err != nil {
				return
			}
			s.mu.Lock()
			for id := range s.pendingDialogs {
				delete(s.pendingDialogs, id)
				break
			}
			s.mu.Unlock()

			s.notifyListeners(CDPEvent{Type: "dialog_closed", Data: &e})
		}
	}()
}

func (s *CDPSupervisor) trackFrameTree(ctx context.Context) {
	session := targetSession(ctx)
	if session == nil {
		return
	}

	attached := cdp.Events(ctx, session, target.AttachedToTarget)
	detached := cdp.Events(ctx, session, target.DetachedFromTarget)

	go func() {
		for _, err := range attached {
			if err != nil {
				return
			}
			s.mu.Lock()
			s.updateFrameTree(ctx)
			s.mu.Unlock()
		}
	}()

	go func() {
		for _, err := range detached {
			if err != nil {
				return
			}
			s.mu.Lock()
			s.updateFrameTree(ctx)
			s.mu.Unlock()
		}
	}()
}

func (s *CDPSupervisor) updateFrameTree(ctx context.Context) {
	res, err := chromedp.Call(ctx, page.GetFrameTree, cdp.Empty{})
	if err != nil {
		return
	}
	tree := res.FrameTree

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

// GetPendingDialogs 返回当前待处理对话框的副本。
func (s *CDPSupervisor) GetPendingDialogs() map[string]*DialogInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*DialogInfo)
	for k, v := range s.pendingDialogs {
		result[k] = v
	}
	return result
}

// HandleDialog 通过 CDP 处理指定对话框（accept 决定接受/关闭，promptText 用于 prompt）。
func (s *CDPSupervisor) HandleDialog(dialogID string, accept bool, promptText string) error {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		return fmt.Errorf("supervisor not connected")
	}

	_, err := chromedp.Call(ctx, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{
		Accept:     accept,
		PromptText: promptText,
	})
	return err
}

// EvaluateJS 在页面（或 targetFrameID 指定的 frame）中以 DevTools 权限执行 JS 并返回结果字符串。
func (s *CDPSupervisor) EvaluateJS(expression string, targetFrameID string) (string, error) {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		return "", fmt.Errorf("supervisor not connected")
	}

	result, err := chromedp.Run(ctx, chromedp.EvaluateAsDevTools[string](expression))
	if err != nil {
		return "", fmt.Errorf("JS evaluation failed: %w", err)
	}

	return result, nil
}

// GetFrameTree 返回当前页面 frame 树的副本（受 maxFrames/maxDepth 截断）。
func (s *CDPSupervisor) GetFrameTree() []*FrameInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*FrameInfo, len(s.frameTree))
	copy(result, s.frameTree)
	return result
}

// IsTruncated 报告 frame 树是否因超过 maxFrames 被截断。
func (s *CDPSupervisor) IsTruncated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.frameTree) >= s.maxFrames
}

// AddListener 注册一个 CDP 事件监听器。
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
