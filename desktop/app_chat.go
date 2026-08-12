//go:build darwin

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
	madyserver "github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
)

// ChatRequest 直接复用 server.ChatRequest，避免字段漂移。
// 前端发送：{ message, thread_id?, model?, response_format?, thinking?, skills? }
//
// 注意：Stream 字段由桌面端固定设为 true，前端无需传递。
type ChatRequest = madyserver.ChatRequest

// Chat 发起一轮对话，返回 runId。
//
// 内部在 goroutine 中调用 server.Chat，通过 onEvent 回调实时将
// agentcore.Event 经 agui.Convert 转换为 AGUI 事件并通过 Wails Events
// 推送到前端。chat 结束时自动 emit agui:done。
func (a *App) Chat(req madyserver.ChatRequest) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.startChatRun(req)
}

// ChatInTab 向指定标签发起对话（阶段 2.1b：会话绑定按 tab 分派）。
//
// 与 Chat 的差异：
//   - 若标签尚未关联会话（tab.ThreadID 为空），先经 server.EnsureThreadID
//     创建会话并写回标签（持久化），保证「新标签 → 新会话」闭环；
//   - 消息始终作用于该标签的会话，前端切换标签即切换会话上下文。
func (a *App) ChatInTab(tabID string, req madyserver.ChatRequest) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if a.tabs == nil {
		return "", errTabsNotReady
	}
	tab, err := a.tabs.Get(tabID)
	if err != nil {
		return "", err
	}
	threadID, err := a.server.EnsureThreadID(a.ctxOrNil(), tab.ThreadID)
	if err != nil {
		return "", err
	}
	if tab.ThreadID == "" {
		if err := a.tabs.SetThreadID(tabID, threadID); err != nil {
			return "", err
		}
		log.Printf("[mady-desktop] tab %s bound to thread %s", tabID, threadID)
	}
	req.ThreadID = threadID
	return a.startChatRun(req)
}

// startChatRun 执行一轮对话（Chat 与 ChatInTab 共用的运行逻辑）。
// 要求调用方已通过 a.ready() 校验且 req.ThreadID 已就绪。
func (a *App) startChatRun(req madyserver.ChatRequest) (string, error) {
	runID := generateRunID()
	ctx, cancel := context.WithCancel(a.ctxOrNil())
	a.runs.Store(runID, &runInfo{
		cancel:   cancel,
		threadID: req.ThreadID,
		runID:    runID,
	})

	// 每条 chat 创建一个 AGUI converter，携带线程和 run 标识
	converter := agui.NewConverter(req.ThreadID, runID)

	go func() {
		defer a.runs.Delete(runID)
		start := time.Now()

		output, err := a.server.Chat(ctx, req, func(e agentcore.Event) {
			a.emitAguiEvent(ctx, converter, e)
		})

		// 无论成功失败，始终 emit agui:done（释放前端输入框）
		done := map[string]any{
			"runId":    runID,
			"output":   output,
			"threadId": req.ThreadID,
		}
		if err != nil {
			done["error"] = err.Error()
			log.Printf("[mady-desktop] chat %s failed: %v", runID, err)
		}
		runtime.EventsEmit(ctx, "agui:done", done)

		// 长任务完成系统通知（W4-T11，G-I4）：仅「长任务」（超过阈值）成功
		// 时弹通知，避免交互式短对话的通知轰炸；失败通知始终保留。
		if err != nil {
			a.notifyChatDone(err.Error())
		} else if time.Since(start) >= chatNotifyThreshold {
			a.notifyChatDone("")
		}
	}()

	return runID, nil
}

// Cancel 取消指定 runId 的 chat 流。
// 通过 context cancel 终止 server.Chat 内部循环，
// 同时通过 server 层向 agent 发射 interrupt 信号实现优雅中断。
func (a *App) Cancel(runID string) error {
	val, ok := a.runs.Load(runID)
	if !ok {
		return errRunNotFound(runID)
	}
	info, ok := val.(*runInfo)
	if !ok {
		return fmt.Errorf("cancel: invalid run info for %s", runID)
	}
	if info.cancel != nil {
		info.cancel()
	}
	// 使用正确的 threadID 通知 server 层发射 agent interrupt
	a.server.Cancel(info.threadID)
	a.runs.Delete(runID)
	log.Printf("[mady-desktop] canceled run %s (thread %s)", runID, info.threadID)
	return nil
}

// SendAction 将用户在 A2UI surface 上触发的 action 回传给 agent。
// surfaceID 标识来源 surface，action 包含具体的交互信息（按钮点击、表单提交等）。
func (a *App) SendAction(surfaceID string, action *a2ui.ClientAction) error {
	if err := a.ready(); err != nil {
		return err
	}
	if action == nil {
		return fmt.Errorf("sendAction: action is required")
	}
	// 确保 timestamp 不为空
	if action.Timestamp == "" {
		action.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return a.server.SendAction(surfaceID, action)
}

// ThreadSummary 是侧栏渲染的会话概要信息。
type ThreadSummary struct {
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	MessageN  int       `json:"messageN"`
}

// threadSummaries 将会话元信息列表转换为前端 ThreadSummary 列表。
func threadSummaries(infos []session.Info) []ThreadSummary {
	summaries := make([]ThreadSummary, len(infos))
	for i, info := range infos {
		summaries[i] = ThreadSummary{
			Key:       info.ID,
			Title:     info.Name,
			UpdatedAt: info.UpdatedAt,
			MessageN:  int(info.MessageCount),
		}
	}
	return summaries
}

// ListThreads 返回所有持久化会话的概要列表。
func (a *App) ListThreads() ([]ThreadSummary, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	infos, err := a.server.ListThreads(a.ctxOrNil())
	if err != nil {
		return nil, err
	}
	return threadSummaries(infos), nil
}

// GetThread 返回指定会话的完整消息列表。
func (a *App) GetThread(key string) (*session.ThreadSnapshot, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.server.GetThread(a.ctxOrNil(), key)
}

// DeleteThread 删除指定会话。
func (a *App) DeleteThread(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.server.DeleteThread(a.ctxOrNil(), key)
}

// RenameThread 重命名会话（自定义标题，阶段 1.4）。
// 写入 session_info 元数据；ListThreads 返回的 Info.Name 会携带新标题。
func (a *App) RenameThread(key, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.server.RenameThread(a.ctxOrNil(), key, name)
}

// TrashThread 将会话移入回收站（软删除，阶段 1.4）。
func (a *App) TrashThread(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.server.TrashThread(a.ctxOrNil(), key)
}

// RestoreThread 将回收站中的会话恢复回主目录。
func (a *App) RestoreThread(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.server.RestoreThread(a.ctxOrNil(), key)
}

// ListTrashedThreads 列出回收站中的会话（按更新时间倒序）。
func (a *App) ListTrashedThreads() ([]ThreadSummary, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	infos, err := a.server.ListTrashedThreads(a.ctxOrNil())
	if err != nil {
		return nil, err
	}
	return threadSummaries(infos), nil
}

// PurgeThread 从回收站彻底删除会话（不可恢复）。
func (a *App) PurgeThread(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.server.PurgeThread(a.ctxOrNil(), key)
}

// ModelEntry 是可用模型的 Wails Binding 返回类型。
type ModelEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	ContextWindow int64  `json:"contextWindow"`
}

// ListModels 返回当前可用的模型列表。
// 从 server 配置中读取，前端模型选择器使用此数据。
func (a *App) ListModels() ([]ModelEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	models, err := a.server.ListModels(a.ctxOrNil())
	if err != nil {
		return nil, err
	}
	entries := make([]ModelEntry, len(models))
	for i, m := range models {
		entries[i] = ModelEntry{
			ID:            m.ID,
			Name:          m.Name,
			Provider:      m.Provider,
			ContextWindow: m.ContextWindow,
		}
	}
	return entries, nil
}

// HealthInfo 返回运行时健康检查信息。
type HealthInfo = madyserver.HealthInfo

// Health 返回桌面端运行时健康信息。
func (a *App) Health() (HealthInfo, error) {
	if err := a.ready(); err != nil {
		return HealthInfo{}, err
	}
	info := a.server.Health()
	// S-1：桌面端版本以 desktopVersion 为准（server.Health().Version 为 "unknown"，
	// 面向根模块；桌面端绑定统一暴露自身版本，与设置面板/CheckUpdate 一致）。
	info.Version = desktopVersion
	return info, nil
}
