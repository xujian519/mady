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

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/mcp"
	madyserver "github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"

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

// --- 错误值 ---

var errServerNotReady = fmt.Errorf("server not ready: startup may not have completed")
var errTabsNotReady = fmt.Errorf("tabs not ready: startup may not have completed")

func errRunNotFound(runID string) error {
	return fmt.Errorf("run %s not found", runID)
}
