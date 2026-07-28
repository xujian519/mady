package server

// desktop.go — 桌面端专用公开 API（内嵌调用，非 HTTP）
//
// 本文件为 desktop/ 模块提供非 HTTP 的 chat 执行入口和辅助方法，
// 所有方法均不破坏现有 HTTP handler 行为。
//
// 设计原则：
//   - 只新增导出方法，不修改私有 handler 的签名或行为
//   - Chat 方法的核心逻辑与 handleStreamChat 保持一致（agent.OnAll
//     回调注册、Extension Snapshots emit、状态保存），但绕过 SSE
//     层直接返回 agentcore.Event
//   - 调用方（desktop/app.go）负责将事件转换为 AGUI 格式并通过
//     Wails Events 推送到前端

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
)

// Chat runs a synchronous agent conversation. It reuses server.ChatRequest
// (Stream field is ignored internally). onEvent is called for every agent
// event on the agent event bus goroutine — callers must not block.
//
// Internal flow:
//  1. ensureThreadID — auto-create thread on empty threadID
//  2. loadAgent — borrow or create agent from pool
//  3. agent.OnAll(onEvent) — register real-time event callback (including Extension Snapshots)
//  4. agent.Run — run the conversation
//  5. saveAgentState — persist session state
//  6. releaseAgent — return agent to pool
//
// Callers are responsible for converting agentcore.Event to AGUI events in
// onEvent (via agui.Convert), and emitting agui:done after Chat returns.
func (s *Server) Chat(ctx context.Context, req ChatRequest, onEvent func(agentcore.Event)) (output string, err error) {
	if req.Message == "" {
		return "", fmt.Errorf("server.Chat: message is required")
	}

	// 确保 threadID 存在（空则自动创建）
	threadID, err := s.ensureThreadID(ctx, req.ThreadID)
	if err != nil {
		return "", fmt.Errorf("server.Chat: ensureThreadID: %w", err)
	}
	req.ThreadID = threadID

	// 从池中借用或新建 agent
	entry, err := s.loadAgent(ctx, threadID, requestCallConfig(req))
	if err != nil {
		return "", fmt.Errorf("server.Chat: loadAgent: %w", err)
	}
	agent := entry.agent

	// 注册 agent 事件总线回调 — 实时转发事件给调用方
	unregister := agent.OnAll(func(e agentcore.Event) {
		if onEvent != nil {
			onEvent(e)
		}
	})
	defer unregister()

	// 发射扩展快照（MCP 工具列表等）
	agent.EmitExtensionSnapshots()

	// 执行对话
	output, runErr := agent.Run(ctx, req.Message)
	if runErr != nil {
		slog.Error("server.Chat: agent.Run failed", "threadID", threadID, "err", runErr)
	}

	// 持久化会话状态
	saveErr := s.saveAgentState(ctx, agent, threadID)
	if saveErr != nil && runErr == nil {
		runErr = saveErr
		slog.Error("server.Chat: saveAgentState failed", "threadID", threadID, "err", saveErr)
	}

	// 注销回调（defer 作为安全网）并归还 agent
	unregister()
	s.releaseAgent(entry, threadID)

	return output, runErr
}

// ListThreads 返回所有持久化会话的概要信息。
// 对应 HTTP GET /api/threads。
func (s *Server) ListThreads(ctx context.Context) ([]session.Info, error) {
	ts, ok := s.threadStore()
	if !ok {
		return nil, fmt.Errorf("server.ListThreads: thread store not available")
	}
	return ts.ListThreads(ctx)
}

// GetThread 返回指定会话的完整快照（消息列表）。
// 对应 HTTP GET /api/threads/{key}。
func (s *Server) GetThread(ctx context.Context, key string) (*session.ThreadSnapshot, error) {
	if key == "" {
		return nil, fmt.Errorf("server.GetThread: key is required")
	}
	ts, ok := s.threadStore()
	if !ok {
		return nil, fmt.Errorf("server.GetThread: thread store not available")
	}
	return ts.GetThread(ctx, key)
}

// DeleteThread 删除指定会话。
// 对应 HTTP DELETE /api/threads/{key}。
func (s *Server) DeleteThread(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("server.DeleteThread: key is required")
	}
	store := s.snapshotConfig().Store
	if store == nil {
		return fmt.Errorf("server.DeleteThread: no store configured")
	}
	return store.Delete(ctx, key)
}

// Cancel 取消指定 threadID 对应 agent 的正在执行的 chat 流。
// 通过 agent 事件总线发射 AgentInterruptEvent，agent.Run 循环会
// 检测此信号并进行优雅中断（保存状态后返回）。
// threadID 来自 ChatRequest.ThreadID（桌面端在前端 Chat 调用时自动生成）。
func (s *Server) Cancel(threadID string) {
	if threadID == "" {
		return
	}
	reason := &agentcore.InterruptReason{
		Reason: "user_canceled",
		Data:   map[string]any{"source": "desktop"},
	}
	s.poolMu.Lock()
	cached, ok := s.agentPool.Load(threadID)
	s.poolMu.Unlock()
	if ok {
		entry := cached.(*poolEntry)
		slog.Info("server.Cancel: emitting interrupt event", "threadID", threadID)
		entry.agent.Emit(agentcore.NewAgentInterruptEvent("mady-agent", reason))
	}
}

// SendAction 将客户端 A2UI action 投递给当前会话的 agent。
//
// 通过 surfaceID（格式 "surface_<threadID>"）提取 threadID 并在池中查找 agent，
// 将 ClientAction 包装为 A2UIEvent 通过 agent 事件总线投递。
//
// 注意：当前 agent 侧尚未注册 A2UIEvent 入站处理器来消费此事件，
// 因此 SendAction 投递的事件目前仅到达事件总线，不会被 agent 执行循环处理。
// TODO: agent 侧需注册 EventA2UI 监听器解析 ClientAction 并注入 agent 上下文。
func (s *Server) SendAction(surfaceID string, action *a2ui.ClientAction) error {
	if action == nil {
		return fmt.Errorf("server.SendAction: action is required")
	}
	if surfaceID == "" {
		return fmt.Errorf("server.SendAction: surfaceID is required")
	}

	slog.Debug("server.SendAction: delivering action to agent",
		"surfaceID", surfaceID,
		"action", action.Name,
	)

	// 从 surfaceID 格式 "surface_<threadID>" 中提取 threadID
	threadID := extractThreadID(surfaceID)
	if threadID == "" {
		return fmt.Errorf("server.SendAction: cannot extract threadID from surfaceID %s", surfaceID)
	}

	s.poolMu.Lock()
	cached, ok := s.agentPool.Load(threadID)
	s.poolMu.Unlock()
	if !ok {
		return fmt.Errorf("server.SendAction: no active agent for thread %s", threadID)
	}
	entry := cached.(*poolEntry)

	// 将 ClientAction 包装为 A2UIEvent 通过 agent 事件总线投递。
	// agent 侧收到此事件后可解析出 ClientMessage 并处理 action。
	// 当前阶段：仅送达事件总线，agent 侧处理尚未实现。
	cm := a2ui.ClientMessage{Action: action}
	payload := map[string]any{
		"kind":    "client_action",
		"action":  cm,
		"surface": surfaceID,
	}
	entry.agent.Emit(agentcore.NewA2UIEvent(payload))
	slog.Info("server.SendAction: delivered to agent event bus",
		"surfaceID", surfaceID, "threadID", threadID, "action", action.Name,
	)
	return nil
}

// extractThreadID 从 surfaceID 格式 "surface_<threadID>" 中提取 threadID。
func extractThreadID(surfaceID string) string {
	const prefix = "surface_"
	if len(surfaceID) > len(prefix) && surfaceID[:len(prefix)] == prefix {
		return surfaceID[len(prefix):]
	}
	return ""
}

// HealthInfo 是桌面端健康检查的响应结构。
type HealthInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Version  string `json:"version"`
	Uptime   string `json:"uptime"`
}

// Health 返回运行时健康信息。
func (s *Server) Health() HealthInfo {
	cfg := s.snapshotConfig()
	info := HealthInfo{
		Model:   cfg.Model,
		Version: "unknown",
		Uptime:  s.uptime(),
	}
	if cfg.Provider != nil {
		info.Provider = fmt.Sprintf("%T", cfg.Provider)
	}
	return info
}

// uptime 返回从 Server 创建到现在的运行时长。
func (s *Server) uptime() string {
	if s.createdAt.IsZero() {
		return "0s"
	}
	return time.Since(s.createdAt).Truncate(time.Second).String()
}

// SwitchModel 切换全局默认模型，可选同时更换 Provider 与上下文窗口。
// 仅影响后续新建的 agent（新会话）；已池化的会话 agent 保持原有配置
// 不变，符合桌面端 Q9「全局切换 + 新会话生效」语义。
// provider 为 nil 表示保持现有 Provider；model 为空表示保持现有模型；
// contextWindow <= 0 表示保持现有上下文窗口。
func (s *Server) SwitchModel(provider agentcore.Provider, model string, contextWindow int64) {
	cfg := s.config.Get()
	if provider != nil {
		cfg.Provider = provider
	}
	if model != "" {
		cfg.Model = model
	}
	if contextWindow > 0 {
		cfg.ContextWindow = contextWindow
	}
	s.config.Set(cfg)
}
