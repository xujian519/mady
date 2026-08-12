//go:build darwin

package main

// app.go — Mady 桌面端应用核心。
//
// App 结构体是 Wails Binding 的接收端，将前端调用映射到 server 包。
// 生命周期由 Wails 框架管理：OnStartup → (Chat/Cancel/... 任意序列) → OnShutdown。
//
// 按责任拆分：app_settings.go（AI 设置）、app_files.go（文件操作）、
// app_skills.go（技能管理）、app_mcp.go（MCP 服务器管理）。

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	"github.com/xujian519/mady/tools"

	"github.com/xujian519/mady/bootstrap"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
)

// App 是桌面端顶层应用结构体，持有内嵌的 server 实例。
// 所有公开方法都是 Wails Binding，由前端 TS 通过 wailsjs 调用。
type App struct {
	// ctx 是 Wails 运行时上下文（atomic.Value 存 context.Context）。
	// 原子化原因（G-I8）：startup 在 Wails 主线程写入，而单实例锁的
	// 第二实例回调（focusMainWindow）与托盘菜单可能在独立 goroutine 读取；
	// 直接字段读写属于 -race 语义下的数据竞争。
	ctx    atomic.Value
	server *madyserver.Server
	fc     *bootstrap.Context
	runs   sync.Map // runId -> *runInfo

	// aiMu 保护 aiProvider/aiModel（Q9 全局 AI 设置，读写并发安全）。
	aiMu       sync.RWMutex
	aiProvider string // 当前生效的 Provider 名（含用户覆盖）
	aiModel    string // 当前生效的模型（含用户覆盖）

	// settingsMu 保护 desktop-settings.json 的 load-modify-save 原子性（G-I3）。
	// SetAISettings 与 setCurrentProject 会并发写同一文件；无锁时后写者用
	// 旧快照覆盖先写者的字段更新（静默丢更新）。
	settingsMu sync.Mutex

	// tabs 是 Go 侧会话标签状态机（阶段 2.1），startup 时初始化。
	tabs *tabStore
}

// ctxOrNil 返回 Wails 运行时上下文；未初始化（startup 未执行）时为 nil。
// 所有跨 goroutine 读取 a.ctx 的路径必须走此方法（G-I8）。
func (a *App) ctxOrNil() context.Context {
	v := a.ctx.Load()
	if v == nil {
		return nil
	}
	return v.(context.Context)
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
	if ctx := a.ctxOrNil(); ctx != nil {
		runtime.EventsEmit(ctx, "mady:init-progress", msg)
	}
}

// timedPhase 执行带计时和进度通知的初始化步骤。
// progressMsg 发送到前端（中文），timingLabel 用于 [timing] 日志。
func (a *App) timedPhase(progressMsg, timingLabel string, fn func()) {
	a.emitInitProgress(progressMsg)
	t := time.Now()
	fn()
	log.Printf("[timing] %s: %v", timingLabel, time.Since(t))
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
	a.ctx.Store(ctx)
	log.Println("[mady-desktop] startup: framework initializing (phase 1 — core)")
	a.emitInitProgress("正在初始化引擎...")

	// S-8：恢复上次保存的窗口位置（无记录时保持 Wails 默认居中）
	a.applySavedWindowPosition(ctx)

	// 阶段 2.1：恢复会话标签（ListTabs 驱动前端 TabBar；MadyHome 为空时仅内存）
	a.tabs = newTabStore(a.resolveMadyHome())
	log.Printf("[mady-desktop] tabs restored: %d tab(s), active=%s", len(a.tabs.List()), a.tabs.ActiveID())

	// 系统托盘（W4-T11）：独立 goroutine，不阻塞初始化
	a.startTray()

	// == 阶段 1：核心初始化（同步） ==

	madyHome, err := util.MadyHome()
	if err != nil {
		log.Printf("[mady-desktop] MadyHome unavailable: %v", err)
		madyHome = ""
	}

	// Q9：用户保存的 Provider/Model 覆盖环境默认值（desktop-settings.json）。
	var saved AISettings
	if madyHome != "" {
		saved = loadJSONFile[AISettings](aiSettingsPath(madyHome))
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
	// 使用统一的 BaseConfig 构造，确保 max_tokens 默认值、用户配置覆盖
	// 与 fallback 候选链在 desktop 与 cli 入口行为一致。
	fc.BaseConfig = bootstrap.NewBaseConfig(model, provider, agentconfig.LoadOrDefault())

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
	// 优化策略（2026-07）：
	//   1. Server 在 Phase 2 开始时立即创建，emit init-done，前端立刻可用。
	//   2. 知识库/规则引擎/技能/MCP/记忆系统/推理引擎等重型初始化在后台静默完成。
	//   3. 完成后通过 SyncConfig 将新扩展注入 server，后续新建会话获得完整能力。
	//
	// 这消除了此前"必须等全部初始化完成才能交互"的两阶段门闩缺陷。
	//
	// 注入 MCP 发现超时（desktop 路径此前未命中 bootstrap 的 timeout 分支）。
	discoveryCtx := mcp.WithDiscoveryTimeout(ctx, 1500*time.Millisecond)

	// === Server 创建（同步段，G-I1） ===
	// 使用 UnifiedAgentConfig 包装基础配置，确保专利/法律 handoff、
	// doomloop、gateway、guardrails 等专业能力从第一阶段就可用。
	// 同步创建避免后台 goroutine 写入 a.server 与 Binding 读取的数据竞争；
	// 后续重型初始化通过 SyncConfig 将知识库/MCP/记忆等扩展注入 server。
	a.server = madyserver.New(buildDesktopAgentConfig(fc))
	a.emitInitProgress("就绪")
	runtime.EventsEmit(ctx, "mady:init-done", map[string]any{"ready": true, "degraded": true})
	log.Printf("[mady-desktop] server created — frontend now interactive")

	go a.initDeferred(discoveryCtx, fc)
}

// initDeferred 在后台 goroutine 中完成重型初始化，通过 Wails Events
// 向前端发射进度文案。
//
// 优化（2026-07）：Server 由 startup 同步段创建并 emit init-done（G-I1），
// 使前端在主窗口显示后即可交互。重型初始化在后台静默完成，
// 完成后通过 SyncConfig 将完整的扩展列表注入 server。
func (a *App) initDeferred(ctx context.Context, fc *bootstrap.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[mady-desktop] PANIC in deferred init: %v", r)
			a.emitInitProgress("初始化异常: 部分功能不可用")
			// init-done 已由 startup 同步发射，此处不再重复（G-I1）。
			// M-9：panic 后仍尽力注入阶段 2 已完成的扩展（知识库/MCP/技能/记忆等），
			// 避免 server config 停留在缺失扩展的同步段状态直至下次项目切换。
			if a.server != nil {
				a.server.SyncConfig(buildDesktopAgentConfig(fc))
			}
		}
	}()

	// 检查上下文是否已被取消（窗口提前关闭）。
	select {
	case <-ctx.Done():
		log.Printf("[mady-desktop] deferred init canceled: %v", ctx.Err())
		return
	default:
	}

	// === 阶段 2：重型初始化（后台，不阻塞前端） ===
	t0 := time.Now()
	log.Println("[mady-desktop] startup: phase 2 — deferred init starting")

	// 知识库嵌入/Rerank 模型设置注入：将保存的配置写入 OMLX_* 环境变量，
	// 使 LoadWikiStore → BuildEmbedder/BuildReranker 装配时读到新值（重启生效）。
	a.applyKnowledgeModelEnv()

	a.timedPhase("正在加载知识库...", "LoadWikiStore", func() {
		fc.WikiStore, fc.WikiHook, fc.KnowledgeExt, fc.KnowledgeBackend = bootstrap.LoadWikiStore(fc.MadyHome)
		if fc.KnowledgeBackend != nil {
			log.Printf("[mady-desktop] wiki store loaded (backend: %v)", fc.KnowledgeBackend)
		}
		fc.WikiRoot = bootstrap.ResolveWikiRoot(fc.MadyHome)
	})

	a.timedPhase("正在加载规则引擎...", "LoadRuleEngine", func() {
		if engine, err := rules.LoadEngineFromMadyHome(); err != nil {
			log.Printf("[mady-desktop] rule engine load failed: %v", err)
		} else {
			fc.RuleEngine = engine
		}
	})

	a.timedPhase("正在发现技能和 MCP 服务...", "DiscoverSkills+MCP", func() {
		bootstrap.DiscoverSkills(fc)
		bootstrap.DiscoverMCP(ctx, fc)
	})

	a.timedPhase("正在初始化记忆系统...", "InitMemorySystem", func() {
		bootstrap.InitMemorySystem(fc)
	})

	a.timedPhase("正在加载推理引擎...", "InitReasoningAndTemplates", func() {
		bootstrap.InitReasoningAndTemplates(fc)
	})

	// 将 Phase 2 新增的扩展（知识库/MCP/技能/记忆编译器/推理引擎等）
	// 同步到 server，使后续新建的会话获得完整能力。
	tEnd := time.Now()
	a.server.SyncConfig(buildDesktopAgentConfig(fc))
	log.Printf("[timing] SyncConfig: %v | total phase 2: %v", time.Since(tEnd), time.Since(t0))

	log.Printf("[mady-desktop] deferred init complete: phase 2 took %v", time.Since(t0))
	log.Println("[mady-desktop] startup: deferred init complete — all capabilities available")
}

// buildDesktopAgentConfig 为桌面端构造统一 Agent 配置。
// 与 server 入口保持一致：启用 handoff/doomloop/gateway/guardrails/专业工具链，
// 并对无交互场景默认拒绝危险工具（bash/process/execute_code/browser/computer_use）。
//
// 关键：装配会话持久化 Store（与 cmd/mady/server.go 相同模式）。
// 无 Store 时 server.Chat 的 ensureThreadID 返回空 → agent 不入池即被
// Close，SendAction 的池查找永远失败，A2UI 审批闭环在生产中失效。
func buildDesktopAgentConfig(fc *bootstrap.Context) agentcore.Config {
	cfg := domains.UnifiedAgentConfig(fc.BaseConfig, buildDesktopUnifiedToolExt(fc), buildDesktopPatentToolExt(fc), buildDesktopLegalToolExt(fc))
	cfg.Extensions = append(cfg.Extensions,
		bootstrap.DenyDangerousToolsExtension(),
	)
	if fc.KnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
	}
	if fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
	}
	if cfg.Store == nil {
		sessionDir, err := util.ResolveDataDir("sessions")
		if err != nil {
			log.Printf("[mady-desktop] resolve sessions dir: %v (continuing without persistence)", err)
		} else {
			fileStore, ferr := session.NewFileStore(sessionDir)
			if ferr != nil {
				log.Printf("[mady-desktop] session store: %v (continuing without persistence)", ferr)
			} else {
				cfg.Store = session.NewAgentStore(fileStore, fc.WorkspaceDir)
			}
		}
	}
	return cfg
}

// applySavedWindowPosition 恢复上次保存的窗口位置（S-8）。
// X/Y 记录在 window_state.json（beforeClose 时写入）；无记录则保持 Wails 默认居中。
func (a *App) applySavedWindowPosition(ctx context.Context) {
	ws := loadWindowState()
	if ws == nil || ws.X == nil || ws.Y == nil {
		return
	}
	runtime.WindowSetPosition(ctx, *ws.X, *ws.Y)
	log.Printf("[mady-desktop] restored window position: %d,%d", *ws.X, *ws.Y)
}

// beforeClose 在窗口关闭前持久化完整窗口几何（S-8）。
// 宽高与位置均由 Go 侧 runtime 自取，不依赖前端 beforeunload（异常退出也覆盖）；
// 返回 false 表示不阻止关闭。
func (a *App) beforeClose(ctx context.Context) bool {
	w, h := runtime.WindowGetSize(ctx)
	x, y := runtime.WindowGetPosition(ctx)
	saveWindowState(windowState{Width: w, Height: h, X: &x, Y: &y})
	log.Printf("[mady-desktop] saved window geometry: %dx%d @ %d,%d", w, h, x, y)
	return false
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

// --- 内部辅助 ---

// focusMainWindow 将已运行实例的窗口带到前台（单实例锁回调）。
// 回调可能在第二进程（未执行 OnStartup，ctx 为 nil）中触发，故做 nil 防御；
// 若第二进程已初始化完成，则聚焦并还原最小化窗口。
func (a *App) focusMainWindow() {
	ctx := a.ctxOrNil()
	if ctx == nil {
		return
	}
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
}

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
var errTabsNotReady = fmt.Errorf("tabs not ready: startup may not have completed")

func errRunNotFound(runID string) error {
	return fmt.Errorf("run %s not found", runID)
}

// buildDesktopUnifiedToolExt 为桌面端统一 Agent 构建工具扩展。
func buildDesktopUnifiedToolExt(fc *bootstrap.Context) agentcore.Extension {
	workingDir := fc.BaseConfig.ProjectDir
	if workingDir == "" {
		workingDir = fc.BaseConfig.WorkspaceDir
	}
	allowRead, allowWrite := domains.BuildSandboxAllowLists()
	return tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: fc.BaseConfig.Provider,
			Model:    fc.BaseConfig.Model,
		},
		WebSearch:   &tools.WebSearchToolConfig{},
		WebFetch:    &tools.WebFetchToolConfig{},
		ComputerUse: true,
		MaxBytes:    100 * 1024,
		MaxLines:    5000,
	})
}

// buildDesktopPatentToolExt 为桌面端专利子 Agent 构建工具扩展。
func buildDesktopPatentToolExt(fc *bootstrap.Context) agentcore.Extension {
	workingDir := fc.BaseConfig.ProjectDir
	if workingDir == "" {
		workingDir = fc.BaseConfig.WorkspaceDir
	}
	allowRead, allowWrite := domains.BuildSandboxAllowLists()
	return tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: fc.BaseConfig.Provider,
			Model:    fc.BaseConfig.Model,
		},
		WebSearch:  &tools.WebSearchToolConfig{},
		WebFetch:   &tools.WebFetchToolConfig{},
		PatentTool: tools.PatentToolConfigDefaults(),
		Pandoc:     tools.PandocToolConfigDefaults(),
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode,
		},
		MaxBytes: 100 * 1024,
	})
}

// buildDesktopLegalToolExt 为桌面端法律子 Agent 构建工具扩展。
func buildDesktopLegalToolExt(fc *bootstrap.Context) agentcore.Extension {
	workingDir := fc.BaseConfig.ProjectDir
	if workingDir == "" {
		workingDir = fc.BaseConfig.WorkspaceDir
	}
	allowRead, allowWrite := domains.BuildSandboxAllowLists()
	return tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: fc.BaseConfig.Provider,
			Model:    fc.BaseConfig.Model,
		},
		WebSearch: &tools.WebSearchToolConfig{},
		WebFetch:  &tools.WebFetchToolConfig{},
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode, tools.ToolComputerUse,
			tools.ToolProcess,
		},
		MaxBytes: 100 * 1024,
	})
}
