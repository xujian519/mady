package legal

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是法律知识扩展的注册名称。
const ExtensionName = "legal-knowledge"

// ExtensionConfig 配置法律知识扩展。
type ExtensionConfig struct {
	// HomeDir 是 Mady 数据目录（util.MadyHome）；用于定位库与安装产物。
	HomeDir string
	// DBPath 覆盖查询库路径；为空时按 ResolvePaths 解析。
	DBPath string
	// SourcePath 覆盖安装源；为空时按 ResolvePaths 解析。
	SourcePath string
	// Records 是无库降级时使用的内存记录。
	Records []Record
}

// Extension 把法律知识库检索与安装工具注入 Agent。
type Extension struct {
	cfg        ExtensionConfig
	searcher   *Searcher
	sourcePath string
	openErr    error
}

var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

// NewExtension 创建法律知识扩展。
func NewExtension(cfg ExtensionConfig) *Extension {
	return &Extension{cfg: cfg, searcher: newMemorySearcher(cfg.Records)}
}

// Name returns the extension identifier.
func (e *Extension) Name() string { return ExtensionName }

// Init 解析路径并打开知识库。找不到库时降级为内存检索，不阻止 Agent 启动；
// 库存在但打不开时记录错误，检索工具据此报错而不是返回空结果。
func (e *Extension) Init(_ context.Context, _ *agentcore.Agent) error {
	queryPath, sourcePath := ResolvePaths(e.cfg.HomeDir)
	if e.cfg.DBPath != "" {
		queryPath = e.cfg.DBPath
	}
	if e.cfg.SourcePath != "" {
		sourcePath = e.cfg.SourcePath
	}
	e.sourcePath = sourcePath

	if queryPath == "" || !fileExists(queryPath) {
		slog.Warn("legal-knowledge: 未找到法律知识库，降级为内存检索", "home", e.cfg.HomeDir)
		return nil
	}
	searcher, err := Open(Config{DBPath: queryPath, Records: e.cfg.Records})
	if err != nil {
		e.openErr = err
		slog.Error("legal-knowledge: 打开法律知识库失败", "error", err, "path", queryPath)
		return nil
	}
	e.searcher = searcher
	slog.Info("legal-knowledge: 已加载法律知识库",
		"path", queryPath, "fts", searcher.hasLawFTS,
		"guiding_cases", searcher.GuidingCaseDataAvailable())
	return nil
}

// Dispose 关闭数据库连接。
func (e *Extension) Dispose() error {
	if e.searcher == nil {
		return nil
	}
	return e.searcher.Close()
}

// Tools 返回法律知识检索与安装工具。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newSearchTool(e),
		newInstallTool(e),
	}
}

// searchError 返回初始化期打开库的错误（无错误时返回 nil）。
func (e *Extension) searchError() error {
	if e.openErr != nil {
		return fmt.Errorf("法律知识库不可用：%w", e.openErr)
	}
	return nil
}
