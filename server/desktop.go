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

// req：复用 server.ChatRequest，Stream 字段由桌面端内部固定忽略。
// onEvent：每次 agent 产生事件时回调（在 agent 事件总线的 goroutine 中
// 调用，调用方不应阻塞）。
//
// 内部逻辑：
//  1. ensureThreadID — 空 threadID 自动创建新线程
//  2. loadAgent — 从池中借用或新建 agent
//  3. agent.OnAll(onEvent) — 注册实时事件回调（含 Extension Snapshots）
//  4. agent.Run — 执行对话
//  5. saveAgentState — 持久化会话状态
//  6. releaseAgent — 归还 agent 到池
//
// 调用方负责在 onEvent 中将 agentcore.Event 转换为 AGUI 事件
// （使用 agui.Convert），并在 Chat 结束后 emit agui:done。
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

// SendAction 将客户端 A2UI action 投递给当前会话的 agent。
//
// TODO(T2.4): 完整实现 A2UI ClientMessage 投递闭环。
// 当前为占位实现，返回 nil（desktop 端的动作先通过 Chat 内建事件
// 通道传递，由 agent 的 A2UI binding 自行消费）。
func (s *Server) SendAction(surfaceID string, action *a2ui.ClientAction) error {
	if action == nil {
		return fmt.Errorf("server.SendAction: action is required")
	}
	slog.Debug("server.SendAction: received (placeholder)",
		"surfaceID", surfaceID,
		"action", action.Name,
	)
	// TODO(T2.4): 将 action 转发到 agent 的 A2UI ClientMessage 通道。
	// 当前 agent 通过 agent.binding_a2a 上的 HandleClientMessage 接收，
	// 需要先通过 threadID 找到当前 agent entry。
	return nil
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
