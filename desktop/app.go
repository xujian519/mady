package main

// app.go — Mady 桌面端应用核心。
//
// App 结构体是 Wails Binding 的接收端，将前端调用映射到 server 包。
// 生命周期由 Wails 框架管理：OnStartup → (Chat/Cancel/... 任意序列) → OnShutdown。

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
	madyserver "github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"

	"github.com/xujian519/mady/pkg/framework"
)

// App 是桌面端顶层应用结构体，持有内嵌的 server 实例。
// 所有公开方法都是 Wails Binding，由前端 TS 通过 wailsjs 调用。
type App struct {
	ctx    context.Context
	server *madyserver.Server
	fc     *framework.Context
	runs   sync.Map // runId -> *runInfo
}

// runInfo 记录一次 Chat 会话的运行状态。
type runInfo struct {
	cancel   context.CancelFunc
	threadID string
	runID    string
}

// NewApp 创建一个未初始化的 App 实例。
// 真正的初始化在 startup() 中完成（Wails 调用 OnStartup 时）。
func NewApp() *App {
	return &App{}
}

// startup 在 Wails 窗口就绪后调用。在此完成所有重型初始化：
// 1. 运行 framework.Setup（Provider / 知识库 / 扩展等）
// 2. 构造 server.Server 实例
// 3. 注入必要的扩展（approval store 等）
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("[mady-desktop] startup: framework initializing")

	fc, err := framework.Setup(ctx, framework.Options{
		Mode:    framework.ModeSync,
		CmdName: "desktop",
	})
	if err != nil {
		log.Fatalf("[mady-desktop] framework setup failed: %v", err)
	}
	a.fc = fc
	log.Printf("[mady-desktop] framework ready: madyHome=%s", fc.MadyHome)

	// 构造内嵌 server
	a.server = madyserver.New(fc.BaseConfig)
	log.Println("[mady-desktop] server initialized")
}

// shutdown 在 Wails 窗口关闭前调用。优雅关闭 server，取消所有运行中的 chat。
func (a *App) shutdown(_ context.Context) {
	log.Println("[mady-desktop] shutdown: canceling all runs")

	// 取消所有正在运行的 chat
	a.runs.Range(func(key, value any) bool {
		if info, ok := value.(*runInfo); ok && info.cancel != nil {
			info.cancel()
		}
		a.runs.Delete(key)
		return true
	})

	if a.server != nil {
		a.server.Close()
		log.Println("[mady-desktop] server closed")
	}
	log.Println("[mady-desktop] shutdown complete")
}

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

	runID := generateRunID()
	ctx, cancel := context.WithCancel(a.ctx)
	a.runs.Store(runID, &runInfo{
		cancel:   cancel,
		threadID: req.ThreadID,
		runID:    runID,
	})

	// 每条 chat 创建一个 AGUI converter，携带线程和 run 标识
	converter := agui.NewConverter(req.ThreadID, runID)

	go func() {
		defer a.runs.Delete(runID)

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
		return fmt.Errorf("Cancel: invalid run info for %s", runID)
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
		return fmt.Errorf("SendAction: action is required")
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

// ListThreads 返回所有持久化会话的概要列表。
func (a *App) ListThreads() ([]ThreadSummary, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	infos, err := a.server.ListThreads(a.ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]ThreadSummary, len(infos))
	for i, info := range infos {
		summaries[i] = ThreadSummary{
			Key:       info.ID,
			Title:     info.Name,
			UpdatedAt: info.UpdatedAt,
			MessageN:  int(info.MessageCount),
		}
	}
	return summaries, nil
}

// GetThread 返回指定会话的完整消息列表。
func (a *App) GetThread(key string) (*session.ThreadSnapshot, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.server.GetThread(a.ctx, key)
}

// DeleteThread 删除指定会话。
func (a *App) DeleteThread(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.server.DeleteThread(a.ctx, key)
}

// HealthInfo 返回运行时健康检查信息。
type HealthInfo = madyserver.HealthInfo

// Health 返回桌面端运行时健康信息。
func (a *App) Health() (HealthInfo, error) {
	if err := a.ready(); err != nil {
		return HealthInfo{}, err
	}
	return a.server.Health(), nil
}

// --- 项目树操作（T3.2b） ---

// resolveProjectDir 返回当前可用的项目根目录。
// 优先使用 ProjectDir（由 CWD 解析），回退到 WorkspaceDir。
func (a *App) resolveProjectDir() (string, error) {
	cwd := a.fc.BaseConfig.ProjectDir
	if cwd == "" {
		cwd = a.fc.BaseConfig.WorkspaceDir
	}
	if cwd == "" {
		return "", fmt.Errorf("no working directory available")
	}
	return cwd, nil
}

// isPathWithinSandbox 检查 target 是否位于 sandboxRoot 之下。
// 防止路径穿越攻击（path traversal）。
func isPathWithinSandbox(target, sandboxRoot string) bool {
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	cleanRoot, err := filepath.Abs(sandboxRoot)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return false
	}
	// rel 以 ".." 开头说明 target 在 sandboxRoot 之外
	return len(rel) < 2 || rel[:2] != ".."
}

// CreateFolder 在指定父目录下创建子文件夹。
// parentPath 是相对于项目根目录的路径，空字符串表示根目录。
// folderName 为要创建的文件夹名称。
// 操作经过沙箱路径校验，越狱路径将被拒绝。
func (a *App) CreateFolder(parentPath, folderName string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if folderName == "" {
		return "", fmt.Errorf("CreateFolder: folderName is required")
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return "", fmt.Errorf("CreateFolder: %w", err)
	}

	targetDir := cwd
	if parentPath != "" {
		targetDir = filepath.Join(cwd, parentPath)
	}

	newDir := filepath.Join(targetDir, folderName)

	// 沙箱边界校验
	if !isPathWithinSandbox(newDir, cwd) {
		return "", fmt.Errorf("CreateFolder: path escape detected: %s is outside %s", newDir, cwd)
	}

	if err := os.MkdirAll(newDir, 0755); err != nil {
		return "", fmt.Errorf("CreateFolder: %w", err)
	}
	log.Printf("[mady-desktop] created folder: %s", newDir)
	return newDir, nil
}

// RenameFolder 重命名指定路径的文件夹。
// oldPath 为当前完整路径（相对于项目根），
// newName 为新文件夹名称。
// 操作经过沙箱路径校验，越狱路径将被拒绝。
func (a *App) RenameFolder(oldPath, newName string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if oldPath == "" || newName == "" {
		return fmt.Errorf("RenameFolder: oldPath and newName are required")
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return fmt.Errorf("RenameFolder: %w", err)
	}

	oldDir := filepath.Join(cwd, oldPath)
	parentDir := filepath.Dir(oldDir)
	newDir := filepath.Join(parentDir, newName)

	// 沙箱边界校验
	if !isPathWithinSandbox(oldDir, cwd) {
		return fmt.Errorf("RenameFolder: path escape detected: %s is outside %s", oldDir, cwd)
	}
	if !isPathWithinSandbox(newDir, cwd) {
		return fmt.Errorf("RenameFolder: path escape detected: %s is outside %s", newDir, cwd)
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("RenameFolder: %w", err)
	}
	log.Printf("[mady-desktop] renamed folder: %s → %s", oldDir, newDir)
	return nil
}

// ListDirectory 返回指定路径下的文件和文件夹列表。
// relPath 是相对于项目根目录的路径，空字符串表示根目录。
func (a *App) ListDirectory(relPath string) ([]FileEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("ListDirectory: %w", err)
	}

	targetDir := cwd
	if relPath != "" {
		targetDir = filepath.Join(cwd, relPath)
	}

	// 沙箱边界校验（ListDirectory 也需校验，防止读越狱路径）
	if !isPathWithinSandbox(targetDir, cwd) {
		return nil, fmt.Errorf("ListDirectory: path escape detected: %s is outside %s", targetDir, cwd)
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, fmt.Errorf("ListDirectory: %w", err)
	}

	var result []FileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, FileEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
	}
	return result, nil
}

// FileEntry 是文件系统条目的概要信息。
type FileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

// --- 内部辅助 ---

// ready 检查 App 是否已完成 startup 初始化。
// 所有公开 Wails Binding 方法应通过 ready() 快速失败。
func (a *App) ready() error {
	if a.server == nil {
		return errServerNotReady
	}
	return nil
}

// emitAguiEvent 将 agentcore.Event 通过 AGUI converter 转换为 AGUI 事件，
// 再映射为 Wails 事件名并通过 Wails Events 推送到前端。
func (a *App) emitAguiEvent(ctx context.Context, converter *agui.Converter, e agentcore.Event) {
	aguiEvents := converter.Convert(e)
	for _, ev := range aguiEvents {
		name := mapAguiEventToWailsName(ev)
		runtime.EventsEmit(ctx, name, ev)
	}
}

// mapAguiEventToWailsName 将 AGUI 事件映射为前端订阅的 Wails 事件名。
// 标准事件：agui: + kebab-case(EventType)
// 自定义事件：agui: + kebab-case(CustomEvent.Name)
func mapAguiEventToWailsName(ev any) string {
	switch e := ev.(type) {
	case agui.RunStartedEvent:
		return "agui:agent-start"
	case agui.TextMessageContentEvent:
		return "agui:message-delta"
	case agui.ThinkingTextMessageContentEvent:
		return "agui:thinking-delta"
	case agui.ToolCallStartEvent:
		return "agui:tool-call-start"
	case agui.ToolCallEndEvent:
		return "agui:tool-call-end"
	case agui.RunErrorEvent:
		return "agui:error"
	case agui.CustomEvent:
		return "agui:" + toKebabCase(e.Name)
	default:
		return "agui:" + toKebabCase(string(eventTypeOf(ev)))
	}
}

// eventTypeOf 从 AGUI 事件中提取 EventType 字符串。
// 所有 AGUI 事件都内嵌 BaseEvent，支持 GetType() 方法。
func eventTypeOf(ev any) agui.EventType {
	if typed, ok := ev.(interface{ GetType() agui.EventType }); ok {
		return typed.GetType()
	}
	return agui.EventRaw
}

// --- 辅助函数 ---

// generateRunID 生成唯一的 run 标识符。
func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

// toKebabCase 将 SCREAMING_SNAKE_CASE 或 camelCase 转换为 kebab-case。
// "RUN_STARTED" → "run-started", "handoff_start" → "handoff-start"
func toKebabCase(s string) string {
	if s == "" {
		return ""
	}
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			if i > 0 && s[i-1] >= 'a' && s[i-1] <= 'z' {
				result = append(result, '-')
			}
			result = append(result, c+('a'-'A'))
		case c == '_':
			result = append(result, '-')
		default:
			result = append(result, c)
		}
	}
	return string(result)
}

// --- 错误值 ---

var errServerNotReady = fmt.Errorf("server not ready: startup may not have completed")

func errRunNotFound(runID string) error {
	return fmt.Errorf("run %s not found", runID)
}
