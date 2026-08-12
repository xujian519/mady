// Package bootstrap 提供所有 mady 入口（tui/serve/acp/desktop）共享的装配逻辑。
// 注意：bootstrap 是全局装配器，已知会跨层引用 domains/mcp/guardrails 等上层包。
// 这是设计上接受的"必要之恶"，不应被其他基础设施层包导入。
package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore/filecheckpoint"
	"github.com/xujian519/mady/agentcore/planmode"
	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/doctmpl"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/guardrails/guardian"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/fileindex"
	kgwgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/memory/compiler"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/framework"
	"github.com/xujian519/mady/pkg/util"
	"github.com/xujian519/mady/prompt"
	"github.com/xujian519/mady/provider/sanitizer"
	"github.com/xujian519/mady/tracing"
)

// Mode 控制初始化是同步完成还是分两阶段（首帧必需 + 后台延迟）。
type Mode int

const (
	// ModeSync 全同步初始化，serve/acp/desktop 入口使用。
	ModeSync Mode = iota
	// ModeDeferred 首帧必需部分同步，其余后台延迟，TUI 入口使用。
	ModeDeferred
)

// Options 控制 Setup 的行为。
type Options struct {
	Mode    Mode
	CmdName string // "tui", "serve", "acp", "desktop"
}

// Context 封装入口之间共享的初始化资源。
type Context struct {
	BaseConfig      agentcore.Config
	ProjectRegistry *domains.ProjectRegistry
	// CaseIndex 是基于 SQLite 的案件索引库，替代 ProjectRegistry 的核心功能。
	CaseIndex    *domains.CaseIndex
	WikiHook     agentcore.LifecycleHook //nolint:staticcheck // legacy hook type retained for backward compat
	WikiStore    *knowledge.Store
	KnowledgeExt agentcore.Extension
	Manifests    []agentcore.AgentManifest
	Provider     agentcore.Provider
	MadyHome     string
	PromptStore  *prompt.PromptStore
	WorkspaceDir string
	ManifestDir  string
	// KnowledgeGraph 是实体-关系知识图谱，用于多跳推理遍历。
	KnowledgeGraph *kgwgraph.GraphStore
	// KnowledgeBackend 是已打开的 SQLite 知识库（FTS + 向量）。
	KnowledgeBackend knowledge.KnowledgeBackend
	// RuleEngine 是已加载的确定性规则引擎（domains/rules YAML）。
	RuleEngine *rules.Engine
	// WikiRoot 是 Obsidian wiki 根目录（~/.mady/knowledge/wiki 或 $WIKI_PATH）。
	WikiRoot       string
	MemoryManager  *memory.Manager
	MemoryCompiler *compiler.Compiler
	CompilerDBPath string
	// SessionSummarizer 是会话关闭时的异步汇总器。为 nil 时跳过汇总。
	SessionSummarizer *memory.SessionSummarizer
	PlanModeExt       *planmode.PlanModeExtension
	TracerFlush       func(context.Context) error
	GuardianExt       *guardian.GuardianExtension
	EvidenceExt       *evidence.EvidenceExtension
	FileCheckpointExt *filecheckpoint.FileCheckpointExtension
	PlantaskExt       *plantask.Extension
	PlantaskBridge    *PlantaskBridge
	Deferred          *framework.DeferredInit
}

// CaseFileReader implements domains.FileContentReader by wrapping fileindex.FileReader
// with an os.ReadFile fallback.
type CaseFileReader struct{}

// ReadText reads the text content of a file, first trying fileindex then falling back to os.ReadFile.
func (CaseFileReader) ReadText(path string) string {
	dir := filepath.Dir(path)
	reader := fileindex.NewFileReader(dir)
	if result, err := reader.ReadProjectFile(context.Background(), filepath.Base(path)); err == nil {
		return result.Content
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is from filepath.Walk or filepath.Join of CWD
	if err != nil {
		return ""
	}
	return string(data)
}

// docxRendererAdapter 适配 doctmpl.Renderer 的 Render(md, meta) 签名
// 到 disclosure.DOCXConverter 的 Render(md) 签名，忽略 meta 参数。
type docxRendererAdapter struct {
	docx doctmpl.Renderer
}

func (a docxRendererAdapter) Render(markdownBody string) ([]byte, error) {
	return a.docx.Render(markdownBody, doctmpl.RenderMeta{})
}

const startupMCPDiscoveryTimeout = 1500 * time.Millisecond

func withStartupDiscoveryTimeout(ctx context.Context, cmdName string) context.Context {
	if os.Getenv("MADY_MCP_DISCOVERY_TIMEOUT_MS") != "" {
		return ctx
	}
	switch cmdName {
	case "tui", "serve":
		return mcp.WithDiscoveryTimeout(ctx, startupMCPDiscoveryTimeout)
	default:
		return ctx
	}
}

// CwdPartitionName returns a short, filesystem-safe identifier for a working
// directory. It uses the first 16 hex chars of SHA-256 so names are stable
// across restarts and avoid special-character issues on any platform.
func CwdPartitionName(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:])[:16]
}

// Setup 执行所有入口共享的初始化逻辑：
//   - Provider 构建
//   - MadyHome 解析（~/.mady，任意 cwd 可用）
//   - Manifest 加载（go:embed 内置 + MADY_HOME/manifests 外部覆盖）
//   - Wiki 知识库加载（可选的 WIKI_PATH 环境变量）
//   - ProjectRegistry 初始化
//
// 当 opts.Mode == ModeDeferred 时，重型初始化（MCP/memory/reasoning/plugins等）
// 注册到 fc.Deferred 延迟队列而非立即执行，由调用方在初始化完成后
// 通过 fc.Deferred.StartAll() 在后台完成。
func Setup(ctx context.Context, opts Options) (*Context, error) {
	ctx = withStartupDiscoveryTimeout(ctx, opts.CmdName)
	fc := &Context{}

	// === 首帧必需：立即执行 ===

	provider, err := agentconfig.BuildProvider()
	if err != nil {
		return nil, fmt.Errorf("构建 Provider 失败: %w", err)
	}
	// PII 脱敏包装：所有发往 LLM 的请求自动脱敏，响应自动还原。
	provider = sanitizer.New(provider)
	slog.Info("PII 脱敏已启用（出站：身份证号/手机号/银行卡号/电子邮箱）")

	model := agentconfig.DefaultModel()

	madyHome, err := util.MadyHome()
	if err != nil {
		slog.Warn("初始化数据目录失败，将使用 cwd 相对路径", "error", err)
		madyHome = ""
	} else {
		slog.Info("数据目录", "path", madyHome)
	}
	fc.MadyHome = madyHome
	fc.Provider = provider

	// 加载用户配置（MADY_CONFIG + 环境变量）并构造入口统一的 BaseConfig。
	// 特别确保 MaxTokens 被正确传递；默认过低会导致长表格/调研输出被
	// provider 在表格中间截断，出现截图中的列错位、缺字现象。
	userCfg := agentconfig.LoadOrDefault()
	fc.BaseConfig = NewBaseConfig(model, provider, userCfg)

	// OpenTelemetry 分布式追踪。
	if tracingMode := os.Getenv("MADY_TRACING"); tracingMode != "" {
		switch tracingMode {
		case "stdout":
			tracer, flushFn, err := tracing.NewStdoutTracer("mady")
			if err != nil {
				slog.Error("初始化 stdout tracer 失败", "error", err)
			} else {
				fc.BaseConfig.Tracer = tracer
				fc.TracerFlush = flushFn
				slog.Info("stdout tracer 已启用")
			}
		default:
			slog.Warn("未知 tracing 模式", "mode", tracingMode)
		}
	}

	// Manifest 加载。
	LoadManifests(fc)

	// 用户自定义风格目录。
	if fc.MadyHome != "" {
		domains.AddStylePath(filepath.Join(fc.MadyHome, "styles"))
	}

	// === 后台延迟阶段 ===
	deferBackground := (opts.Mode == ModeDeferred)

	if deferBackground {
		fc.Deferred = framework.NewDeferredInit()
		registerDeferredTasks(ctx, fc)
	} else {
		executeSyncRemaining(ctx, fc)
	}

	return fc, nil
}
