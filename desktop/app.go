package main

// app.go — Mady 桌面端应用核心。
//
// App 结构体是 Wails Binding 的接收端，将前端调用映射到 server 包。
// 生命周期由 Wails 框架管理：OnStartup → (Chat/Cancel/... 任意序列) → OnShutdown。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/mcp"
	madyserver "github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"

	"github.com/xujian519/mady/bootstrap"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
)

// App 是桌面端顶层应用结构体，持有内嵌的 server 实例。
// 所有公开方法都是 Wails Binding，由前端 TS 通过 wailsjs 调用。
type App struct {
	ctx    context.Context
	server *madyserver.Server
	fc     *bootstrap.Context
	runs   sync.Map // runId -> *runInfo

	// aiMu 保护 aiProvider/aiModel（Q9 全局 AI 设置，读写并发安全）。
	aiMu       sync.RWMutex
	aiProvider string // 当前生效的 Provider 名（含用户覆盖）
	aiModel    string // 当前生效的模型（含用户覆盖）
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

// emitInitProgress 通过 Wails Events 向前端发射初始化进度消息。
func (a *App) emitInitProgress(msg string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "mady:init-progress", msg)
	}
}

// startup 在 Wails 窗口就绪后调用。
//
// 启动分两阶段：
//
//	阶段 1（同步）— 仅初始化 chat 和文件操作必需的核心部件，完成后 chat 即可用。
//	阶段 2（后台）— 知识库、规则引擎、记忆系统、推理引擎等重型初始化在后台完成。
//
// 前端通过 mady:init-progress 事件接收进度文案，通过 mady:init-done 得知就绪。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("[mady-desktop] startup: framework initializing (phase 1 — core)")
	a.emitInitProgress("正在初始化引擎...")

	// == 阶段 1：核心初始化（同步） ==

	madyHome, err := util.MadyHome()
	if err != nil {
		log.Printf("[mady-desktop] MadyHome unavailable: %v", err)
		madyHome = ""
	}

	// Q9：用户保存的 Provider/Model 覆盖环境默认值（desktop-settings.json）。
	var saved AISettings
	if madyHome != "" {
		saved = loadAISettingsFrom(aiSettingsPath(madyHome))
		if saved.Provider != "" {
			// BuildProvider 依据 PROVIDER 环境变量构建，设置面板的选择优先级最高。
			_ = os.Setenv("PROVIDER", saved.Provider)
		}
	}

	provider, err := agentconfig.BuildProvider()
	if err != nil {
		log.Printf("[mady-desktop] provider setup failed: %v", err)
		a.emitInitProgress("引擎初始化失败: " + err.Error())
		runtime.EventsEmit(ctx, "mady:init-error", err.Error())
		return
	}

	model := agentconfig.DefaultModel()
	if saved.Model != "" {
		model = saved.Model
	}
	a.aiMu.Lock()
	a.aiProvider = util.EnvOrDefault("PROVIDER", "deepseek")
	a.aiModel = model
	a.aiMu.Unlock()

	fc := &bootstrap.Context{
		Provider: provider,
		MadyHome: madyHome,
	}
	fc.BaseConfig = agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "mady-router",
			Model:     model,
			Provider:  provider,
			Streaming: true,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          25,
			ExecutionMode:     agentcore.ModeSerial,
			ValidateArguments: true,
		},
		CompactionConfig: agentcore.CompactionConfig{
			ContextWindow:    agentconfig.ResolveContextWindow(model),
			ReserveTokens:    32000,
			KeepRecentTokens: 4000,
		},
		RetryConfig: &agentcore.RetryConfig{
			MaxRetries:  3,
			BaseDelayMs: 1000,
			MaxDelayMs:  15000,
		},
	}

	// 模型级联回退候选链。
	if fbCfg := bootstrap.LoadFallbackConfig(); fbCfg != nil {
		fc.BaseConfig.FallbackConfig = fbCfg
	}

	// 用户自定义风格目录。
	if fc.MadyHome != "" {
		domains.AddStylePath(filepath.Join(fc.MadyHome, "styles"))
	}

	// Manifest 加载。
	bootstrap.LoadManifests(fc)

	// 工作区初始化（文件操作的前提）。
	a.emitInitProgress("正在初始化工作区...")
	bootstrap.InitWorkspace(fc)

	// 基础工具扩展（文件读写/删除/项目树等桌面端 Wails Binding
	// 所依赖的 sandbox 工具链）。
	bootstrap.BuildBaseTools(fc)

	// 保存 fc 引用供后续阶段使用。
	a.fc = fc
	a.applyLastProject(saved)
	log.Printf("[mady-desktop] core ready: madyHome=%s, workspace=%s, project=%s", fc.MadyHome, fc.WorkspaceDir, fc.BaseConfig.ProjectDir)

	// == 阶段 2：重型初始化（后台） ==
	//
	// 知识库（SQLite + 向量）、规则引擎（YAML）、记忆系统（SQLite + BM25）、
	// 推理引擎等工作区级初始化在后台完成后，完整功能才可用。
	// 不阻塞窗口交互。

	go a.initDeferred(ctx, fc)
}

// initDeferred 在后台 goroutine 中完成重型初始化，通过 Wails Events
// 向前端发射进度文案。
func (a *App) initDeferred(ctx context.Context, fc *bootstrap.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[mady-desktop] PANIC in deferred init: %v", r)
			// 使用 x/slog 记录结构化日志后，仍确保 init-done 发出以避免前端挂死。
			a.emitInitProgress("初始化异常: 部分功能不可用")
			runtime.EventsEmit(ctx, "mady:init-done", map[string]bool{"ready": false})
		}
	}()

	// 检查上下文是否已被取消（窗口提前关闭）。
	select {
	case <-ctx.Done():
		log.Printf("[mady-desktop] deferred init canceled: %v", ctx.Err())
		return
	default:
	}

	log.Println("[mady-desktop] startup: phase 2 — deferred init starting")

	a.emitInitProgress("正在加载知识库...")
	fc.WikiStore, fc.WikiHook, fc.KnowledgeExt, fc.KnowledgeBackend = bootstrap.LoadWikiStore(fc.MadyHome)
	if fc.KnowledgeBackend != nil {
		log.Printf("[mady-desktop] wiki store loaded (backend: %v)", fc.KnowledgeBackend)
	}
	fc.WikiRoot = bootstrap.ResolveWikiRoot(fc.MadyHome)

	a.emitInitProgress("正在加载规则引擎...")
	if engine, err := rules.LoadEngineFromMadyHome(); err != nil {
		log.Printf("[mady-desktop] rule engine load failed: %v", err)
	} else {
		fc.RuleEngine = engine
	}

	a.emitInitProgress("正在发现技能和 MCP 服务...")
	bootstrap.DiscoverSkills(fc)
	bootstrap.DiscoverMCP(ctx, fc)

	a.emitInitProgress("正在初始化记忆系统...")
	bootstrap.InitMemorySystem(fc)

	a.emitInitProgress("正在加载推理引擎...")
	bootstrap.InitReasoningAndTemplates(fc)

	// 所有延迟初始化完成后创建内嵌 server，确保 Config 包含完整 Extensions/AvailableSkills。
	a.server = madyserver.New(fc.BaseConfig)
	log.Println("[mady-desktop] deferred init complete")
	a.emitInitProgress("就绪")
	runtime.EventsEmit(ctx, "mady:init-done", map[string]bool{"ready": true})
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
	models, err := a.server.ListModels(a.ctx)
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

// SaveWindowState 持久化窗口几何信息，供前端 beforeunload 时调用。
func (a *App) SaveWindowState(width, height int) {
	if width < 400 || height < 300 {
		return
	}
	saveWindowState(windowState{Width: width, Height: height})
	log.Printf("[mady-desktop] saved window state: %dx%d", width, height)
}

// Health 返回桌面端运行时健康信息。
func (a *App) Health() (HealthInfo, error) {
	if err := a.ready(); err != nil {
		return HealthInfo{}, err
	}
	return a.server.Health(), nil
}

// --- AI 服务设置（Q9：全局切换 + 新会话生效） ---

// AISettings 是设置面板读写的 AI 服务配置。
// 持久化到 ~/.mady/desktop-settings.json；运行时切换仅对后续新建会话
// 生效，已有会话保持原有模型（Q9 语义）。
type AISettings struct {
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	LastProjectID string `json:"last_project_id,omitempty"`
}

// aiSettingsPath 返回桌面端设置文件路径。
func aiSettingsPath(madyHome string) string {
	return filepath.Join(madyHome, "desktop-settings.json")
}

// loadAISettingsFrom 从指定路径读取 AI 设置。文件不存在或解析失败时
// 返回零值（视为无用户覆盖），不视为错误。
func loadAISettingsFrom(path string) AISettings {
	data, err := os.ReadFile(path) //nolint:gosec // path 由 MadyHome 派生
	if err != nil {
		return AISettings{}
	}
	var s AISettings
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("[mady-desktop] invalid AI settings file %s: %v", path, err)
		return AISettings{}
	}
	return s
}

// saveAISettingsTo 将 AI 设置原子写入指定路径（tmp + rename）。
func saveAISettingsTo(path string, s AISettings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// resolveMadyHome 返回 MadyHome；优先取 framework 上下文（便于测试注入），
// 回退到 util.MadyHome()。
func (a *App) resolveMadyHome() string {
	if a.fc != nil && a.fc.MadyHome != "" {
		return a.fc.MadyHome
	}
	home, err := util.MadyHome()
	if err != nil {
		return ""
	}
	return home
}

// applyLastProject 在启动时恢复上次使用的项目。
// 如果 LastProjectID 存在且对应案件目录仍可用，则将其设为当前 ProjectDir。
func (a *App) applyLastProject(saved AISettings) {
	if saved.LastProjectID == "" || a.fc == nil || a.fc.ProjectRegistry == nil {
		return
	}
	rec, ok := a.fc.ProjectRegistry.Lookup(saved.LastProjectID)
	if !ok {
		log.Printf("[mady-desktop] last project %q not found in registry", saved.LastProjectID)
		return
	}
	if err := domains.ValidateProjectPath(rec.RootPath); err != nil {
		log.Printf("[mady-desktop] last project %q path %s unreachable: %v", rec.ProjectID, rec.RootPath, err)
		return
	}
	a.fc.BaseConfig.ProjectDir = rec.RootPath
	a.fc.ProjectRegistry.Touch(rec.ProjectID)
	log.Printf("[mady-desktop] restored last project: %s (%s)", rec.Alias, rec.RootPath)
}

// GetAISettings 返回当前生效的 Provider/Model，供设置面板展示。
func (a *App) GetAISettings() (AISettings, error) {
	a.aiMu.RLock()
	defer a.aiMu.RUnlock()
	if a.aiProvider == "" && a.aiModel == "" {
		return AISettings{}, fmt.Errorf("GetAISettings: AI settings not initialized")
	}
	return AISettings{Provider: a.aiProvider, Model: a.aiModel}, nil
}

// SetAISettings 切换全局 Provider/Model（Q9：全局切换 + 新会话生效）。
//
// 行为契约：
//  1. 持久化到 ~/.mady/desktop-settings.json（重启后仍生效）；
//  2. 运行时立即对后续新建会话生效；已有会话保持原有模型；
//  3. Provider 切换会依据目标 Provider 的 API Key 重建 Provider 实例，
//     重建失败时返回错误且不变更任何状态（环境变量一并回滚）。
func (a *App) SetAISettings(s AISettings) error {
	if s.Provider == "" && s.Model == "" {
		return fmt.Errorf("SetAISettings: provider 或 model 至少一项必填")
	}

	a.aiMu.Lock()
	defer a.aiMu.Unlock()

	newProvider := a.aiProvider
	if s.Provider != "" {
		newProvider = s.Provider
	}
	newModel := a.aiModel
	if s.Model != "" {
		newModel = s.Model
	}

	// Provider 变化时重建 Provider 实例（依据目标 Provider 的 API Key）。
	var providerIface agentcore.Provider
	if newProvider != a.aiProvider {
		prev := os.Getenv("PROVIDER")
		_ = os.Setenv("PROVIDER", newProvider)
		p, err := agentconfig.BuildProvider()
		if err != nil {
			_ = os.Setenv("PROVIDER", prev) // 回滚环境变量
			return fmt.Errorf("SetAISettings: 重建 Provider 失败（请确认 %s 的 API Key 已配置）: %w", newProvider, err)
		}
		providerIface = p
	}

	// 持久化（原子写）；失败时不变更运行时状态。
	// 保留已有的 last_project_id，避免 AI 设置保存覆盖项目状态。
	if home := a.resolveMadyHome(); home != "" {
		saved := loadAISettingsFrom(aiSettingsPath(home))
		saved.Provider = newProvider
		saved.Model = newModel
		if err := saveAISettingsTo(aiSettingsPath(home), saved); err != nil {
			return fmt.Errorf("SetAISettings: 保存配置失败: %w", err)
		}
	}

	// 运行时生效：更新 framework 上下文与 server 全局配置。
	// server.SwitchModel 仅影响后续新建 agent；池中已有会话保持不变。
	ctxWindow := agentconfig.ResolveContextWindow(newModel)
	if a.fc != nil {
		if providerIface != nil {
			a.fc.BaseConfig.Provider = providerIface
		}
		a.fc.BaseConfig.Model = newModel
		a.fc.BaseConfig.ContextWindow = ctxWindow
	}
	if a.server != nil {
		a.server.SwitchModel(providerIface, newModel, ctxWindow)
	}

	a.aiProvider = newProvider
	a.aiModel = newModel
	log.Printf("[mady-desktop] AI settings updated: provider=%s model=%s", newProvider, newModel)
	return nil
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

	if err := os.MkdirAll(newDir, 0750); err != nil {
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

// --- 文件内容读取（T5.1，PilotDeck 对齐） ---

// maxReadFileSize 是 ReadFile 允许读取的单文件上限（20MB）。
const maxReadFileSize = 20 << 20

// FileContent 描述一个已读出的文件内容。
// 文本类（text/md）通过 Text 返回 UTF-8 内容；
// 二进制类（image/pdf）通过 Data 返回 base64 编码内容。
type FileContent struct {
	Name string `json:"name"`           // 文件名
	Path string `json:"path"`           // 相对项目根的路径
	Kind string `json:"kind"`           // text | md | image | pdf
	Text string `json:"text,omitempty"` // kind=text/md 时的内容
	Data string `json:"data,omitempty"` // kind=image/pdf 时的 base64 内容
	Mime string `json:"mime,omitempty"` // image/png、application/pdf 等
	Size int64  `json:"size"`
}

// classifyFileKind 按扩展名归类文件类型。
// svg 按图片处理（前端 <img> 渲染）；未知扩展名默认 text，
// 由 ReadFile 在读出后做二进制嗅探兜底。
func classifyFileKind(name string) (kind, mime string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "md", "text/markdown"
	case ".pdf":
		return "pdf", "application/pdf"
	case ".png":
		return "image", "image/png"
	case ".jpg", ".jpeg":
		return "image", "image/jpeg"
	case ".gif":
		return "image", "image/gif"
	case ".webp":
		return "image", "image/webp"
	case ".svg":
		return "image", "image/svg+xml"
	case ".bmp":
		return "image", "image/bmp"
	case ".ico":
		return "image", "image/x-icon"
	default:
		return "text", "text/plain"
	}
}

// isBinaryContent 嗅探内容是否为二进制（前 8KB 内含 NUL 字节即判定）。
func isBinaryContent(data []byte) bool {
	const sniffLen = 8192
	n := len(data)
	if n > sniffLen {
		n = sniffLen
	}
	return bytes.Contains(data[:n], []byte{0})
}

// resolveSandboxedPath 将路径解析为沙箱内的绝对路径。
// relPath 可以是相对路径（相对于 sandboxRoot）或绝对路径。
// 越狱路径返回错误。
func resolveSandboxedPath(relPath, sandboxRoot string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is required")
	}
	abs := relPath
	if !filepath.IsAbs(relPath) {
		abs = filepath.Join(sandboxRoot, relPath)
	}
	if !isPathWithinSandbox(abs, sandboxRoot) {
		return "", fmt.Errorf("path escape detected: %s is outside %s", abs, sandboxRoot)
	}
	return abs, nil
}

// resolveSandboxedPathMulti 尝试将 relPath 解析为沙箱内的绝对路径。
// 先在 sandboxRoots[0]（项目根）中解析，失败后依次尝试后续沙箱根（如 MADY_HOME）。
// 全部失败时返回组合错误。
func resolveSandboxedPathMulti(relPath string, sandboxRoots ...string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is required")
	}
	var errs []string
	for _, root := range sandboxRoots {
		abs, err := resolveSandboxedPath(relPath, root)
		if err == nil {
			return abs, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("path not allowed: %s", strings.Join(errs, "; "))
}

// ReadFile 读取项目沙箱或 MADY_HOME 沙箱内的文件内容。
// relPath 是相对于项目根目录的路径（项目文件）或相对/绝对路径（全局技能文件）。
// 文本/Markdown 返回 Text；图片/PDF 返回 base64 编码的 Data。
// 其他二进制文件返回错误，不向前端暴露原始字节。
func (a *App) ReadFile(relPath string) (*FileContent, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("ReadFile: %w", err)
	}

	roots := []string{cwd}
	if home, err := util.MadyHome(); err == nil && home != cwd {
		roots = append(roots, home)
	}

	abs, err := resolveSandboxedPathMulti(relPath, roots...)
	if err != nil {
		return nil, fmt.Errorf("ReadFile: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("ReadFile: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("ReadFile: %s is a directory", relPath)
	}
	if info.Size() > maxReadFileSize {
		return nil, fmt.Errorf("ReadFile: file too large (%d bytes, limit %d)", info.Size(), maxReadFileSize)
	}

	raw, err := os.ReadFile(abs) //nolint:gosec // 路径已过沙箱校验
	if err != nil {
		return nil, fmt.Errorf("ReadFile: %w", err)
	}

	kind, mime := classifyFileKind(info.Name())
	fc := &FileContent{
		Name: info.Name(),
		Path: relPath,
		Kind: kind,
		Mime: mime,
		Size: info.Size(),
	}

	switch kind {
	case "text", "md":
		if kind == "text" && isBinaryContent(raw) {
			return nil, fmt.Errorf("ReadFile: %s appears to be a binary file", relPath)
		}
		fc.Text = string(raw)
	case "image", "pdf":
		fc.Data = base64.StdEncoding.EncodeToString(raw)
	}
	return fc, nil
}

// maxWriteFileSize 是 WriteFile 允许写入的内容上限（20MB）。
const maxWriteFileSize = 20 << 20

// WriteFile 将文本内容写入项目沙箱或 MADY_HOME 沙箱内的文件。
// 仅允许写入 text/md 类文件（按扩展名判定），图片/PDF 等二进制不可写。
// 采用临时文件 + rename 的原子写策略，避免写一半留下损坏文件。
func (a *App) WriteFile(relPath, content string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if len(content) > maxWriteFileSize {
		return fmt.Errorf("WriteFile: content too large (%d bytes, limit %d)", len(content), maxWriteFileSize)
	}

	kind, _ := classifyFileKind(relPath)
	if kind != "text" && kind != "md" {
		return fmt.Errorf("WriteFile: %s is not a writable text file", relPath)
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return fmt.Errorf("WriteFile: %w", err)
	}

	roots := []string{cwd}
	if home, err := util.MadyHome(); err == nil && home != cwd {
		roots = append(roots, home)
	}

	abs, err := resolveSandboxedPathMulti(relPath, roots...)
	if err != nil {
		return fmt.Errorf("WriteFile: %w", err)
	}
	// 原子写：同目录临时文件 + rename
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".mady-write-*")
	if err != nil {
		return fmt.Errorf("WriteFile: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("WriteFile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("WriteFile: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return fmt.Errorf("WriteFile: %w", err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return fmt.Errorf("WriteFile: %w", err)
	}
	log.Printf("[mady-desktop] wrote file: %s (%d bytes)", abs, len(content))
	return nil
}

// --- 文件删除（T5.8，PilotDeck 对齐） ---

// DeleteEntry 删除项目沙箱内的文件或空目录。
// 目录必须为空才允许删除（递归删除本期不支持）。
func (a *App) DeleteEntry(relPath string) error {
	if err := a.ready(); err != nil {
		return err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return fmt.Errorf("DeleteEntry: %w", err)
	}

	abs, err := resolveSandboxedPath(relPath, cwd)
	if err != nil {
		return fmt.Errorf("DeleteEntry: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("DeleteEntry: %w", err)
	}

	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return fmt.Errorf("DeleteEntry: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("DeleteEntry: directory %s is not empty", relPath)
		}
	}

	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("DeleteEntry: %w", err)
	}
	log.Printf("[mady-desktop] deleted: %s", abs)
	return nil
}

// --- 技能管理（T5.6，PilotDeck 对齐） ---

// SkillEntry 是一个已发现技能的概要信息。
type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Path 是 SKILL.md 相对项目根的路径，可直接用于 ReadFile/WriteFile。
	Path string `json:"path"`
}

// ListSkills 扫描所有技能发现路径，返回发现的技能列表。
// 项目本地技能优先，全局技能按名称去重（同名以项目本地为准）。
// 未找到任何技能时返回空列表（不视为错误）。
//
// 扫描路径与 bootstrap.DiscoverSkills 保持一致：
//   - SKILL_DIR 环境变量
//   - ~/.agent/
//   - $PWD/.agent/
//   - $PWD/skills/
//   - $PWD/plugins/
//   - $MADY_HOME/skills/
//   - ~/.agents/skills/
func (a *App) ListSkills() ([]SkillEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("ListSkills: %w", err)
	}

	homeDir, _ := os.UserHomeDir()
	madyHome, _ := util.MadyHome()

	type scanned struct {
		root string
		dir  string
	}
	dirs := []scanned{}

	// 1. SKILL_DIR 环境变量
	if env := os.Getenv("SKILL_DIR"); env != "" {
		for _, p := range filepath.SplitList(env) {
			if p != "" {
				dirs = append(dirs, scanned{root: p, dir: p})
			}
		}
	}
	// 2. ~/.agent/
	if homeDir != "" {
		dirs = append(dirs, scanned{root: homeDir, dir: filepath.Join(homeDir, ".agent")})
	}
	// 3. $PWD/.agent/
	dirs = append(dirs, scanned{root: cwd, dir: filepath.Join(cwd, ".agent")})
	// 4. $PWD/skills/
	dirs = append(dirs, scanned{root: cwd, dir: filepath.Join(cwd, "skills")})
	// 4b. $PWD/plugins/ (插件 SKILL.md)
	dirs = append(dirs, scanned{root: cwd, dir: filepath.Join(cwd, "plugins")})
	// 5. $MADY_HOME/skills/
	if madyHome != "" && madyHome != cwd {
		dirs = append(dirs, scanned{root: madyHome, dir: filepath.Join(madyHome, "skills")})
	}
	// 6. ~/.agents/skills/
	if homeDir != "" {
		dirs = append(dirs, scanned{root: homeDir, dir: filepath.Join(homeDir, ".agents", "skills")})
	}

	seen := make(map[string]bool)
	var result []SkillEntry

	for _, d := range dirs {
		if _, err := os.Stat(d.dir); os.IsNotExist(err) {
			continue
		}
		skills, _, err := skill.Load(d.dir)
		if err != nil {
			continue
		}
		for _, s := range skills {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			var entryPath string
			if d.root == cwd {
				rel, err := filepath.Rel(cwd, s.FilePath)
				if err != nil {
					log.Printf("ListSkills: filepath.Rel(%q, %q) failed: %v", cwd, s.FilePath, err)
					entryPath = filepath.ToSlash(s.FilePath)
				} else {
					entryPath = filepath.ToSlash(rel)
				}
			} else {
				entryPath = filepath.ToSlash(s.FilePath)
			}
			result = append(result, SkillEntry{
				Name:        s.Name,
				Description: s.Description,
				Path:        entryPath,
			})
		}
	}

	if result == nil {
		return []SkillEntry{}, nil
	}
	return result, nil
}

// --- MCP 服务器管理（T5.7，PilotDeck 对齐，只读） ---

// McpServerEntry 是一个已配置 MCP 服务器的只读概要。
// Env 仅暴露键名，不返回值，防止 API Key 泄露到前端。
type McpServerEntry struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"` // stdio | http
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
	EnvKeys []string `json:"envKeys,omitempty"`
	// Source 是来源配置文件（~/.mady/mcp.json 或项目 .mcp.json）。
	Source string `json:"source"`
}

// ListMcpServers 返回已配置的 MCP 服务器列表（只读）。
// 扫描 ~/.mady/mcp.json 与项目 .mcp.json，不触碰信任存储写路径。
// 项目 .mcp.json 的来源不受信，仅作展示（实际执行仍需信任校验）。
func (a *App) ListMcpServers() ([]McpServerEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	var result []McpServerEntry
	collect := func(path, source string) {
		cfg, err := mcp.LoadMCPConfig(path)
		if err != nil {
			return // 文件不存在或解析失败：跳过（best-effort 展示）
		}
		for name, srv := range cfg.MCPServers {
			typ := srv.Type
			if typ == "" {
				typ = "stdio"
			}
			envKeys := make([]string, 0, len(srv.Env))
			for k := range srv.Env {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			result = append(result, McpServerEntry{
				Name:    name,
				Type:    typ,
				Command: srv.Command,
				Args:    srv.Args,
				URL:     srv.URL,
				EnvKeys: envKeys,
				Source:  source,
			})
		}
	}

	if home, err := util.MadyHome(); err == nil {
		collect(filepath.Join(home, "mcp.json"), "global")
	}

	if cwd, err := a.resolveProjectDir(); err == nil {
		collect(filepath.Join(cwd, ".mcp.json"), "project")
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if result == nil {
		result = []McpServerEntry{}
	}
	return result, nil
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
