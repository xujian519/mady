package main

// 本文件提供工具扩展的构建函数，供 cmd/mady 下各入口（tui/serve/acp）共享。
// 遵循被动注入模式：域层不再导入 tools 包创建扩展，由入口层装配后注入。

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/search"
	"github.com/xujian519/mady/retrieval/domain"
	rbrowser "github.com/xujian519/mady/retrieval/domain/browser"
	"github.com/xujian519/mady/tools"
)

// baseToolConfig 返回所有工具扩展共享的基础配置（沙箱/视觉/大小限制）。
func baseToolConfig(fc *frameworkContext) tools.ExtensionConfig {
	workingDir := fc.BaseConfig.ProjectDir
	if workingDir == "" {
		workingDir = fc.BaseConfig.WorkspaceDir
	}
	allowRead, allowWrite := domains.BuildSandboxAllowLists()
	return tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: fc.BaseConfig.Provider,
			Model:    fc.BaseConfig.Model,
		},
		MaxBytes: 100 * 1024,
	}
}

// buildUnifiedToolExt 为统一 Agent 构建工具扩展（含文件/网络/视觉/桌面控制）。
func buildUnifiedToolExt(fc *frameworkContext) agentcore.Extension {
	cfg := baseToolConfig(fc)
	cfg.WebSearch = &tools.WebSearchToolConfig{}
	cfg.WebFetch = &tools.WebFetchToolConfig{}
	cfg.ComputerUse = true
	cfg.MaxLines = 5000
	return tools.NewExtension(cfg)
}

// buildPatentToolExt 为专利子 Agent 构建工具扩展（含文件/网络/视觉/专利工具）。
// 追加 Search Commander 编排工具（patent_search_commander）：ego-browser 可用时
// 自动注册多轮渐进式检索编排器；不可用时静默降级（工具不注册）。
func buildPatentToolExt(fc *frameworkContext) agentcore.Extension {
	cfg := baseToolConfig(fc)
	cfg.WebSearch = &tools.WebSearchToolConfig{}
	cfg.WebFetch = &tools.WebFetchToolConfig{}
	cfg.PatentTool = tools.PatentToolConfigDefaults()
	cfg.Pandoc = tools.PandocToolConfigDefaults()
	cfg.DisableTools = []string{
		tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
		tools.ToolBrowser, tools.ToolExecuteCode,
	}
	return newCombineExtensions(
		tools.NewExtension(cfg),
		newSearchCommanderExtension(),
	)
}

// newCombineExtensions 构造组合扩展。
func newCombineExtensions(exts ...agentcore.Extension) agentcore.Extension {
	return combineExtensions(exts)
}

// newSearchCommanderExtension 构建 patent_search_commander 编排工具扩展。
// ego-browser 不可用时返回空扩展（无工具），不阻塞装配。
func newSearchCommanderExtension() agentcore.Extension {
	return search.NewCommanderExtension(buildEgoCompositeRetriever())
}

// buildEgoCompositeRetriever 构建 ego-browser 驱动的三源复合检索器
// （Google Patents / CNIPA / Espacenet）。ego-browser 不可用时返回 nil。
func buildEgoCompositeRetriever() domain.DomainRetriever {
	bcfg := rbrowser.DefaultConfig()
	composite := rbrowser.NewCompositeRetriever(
		rbrowser.NewGooglePatentsRetriever(*bcfg),
		rbrowser.NewCNIPARetriever(*bcfg),
		rbrowser.NewEspacenetRetriever(*bcfg),
	)
	if composite == nil {
		return nil
	}
	return composite
}

// combineExtensions 将多个扩展合并为单个扩展，按序执行 Init/Dispose，
// 聚合所有 ToolProvider 暴露的工具。任一 Init 失败即返回错误。
type combineExtensions []agentcore.Extension

func (c combineExtensions) Name() string {
	names := make([]string, 0, len(c))
	for _, ext := range c {
		names = append(names, ext.Name())
	}
	return "combined(" + strings.Join(names, "+") + ")"
}

func (c combineExtensions) Init(ctx context.Context, agent *agentcore.Agent) error {
	for _, ext := range c {
		if err := ext.Init(ctx, agent); err != nil {
			return fmt.Errorf("extension %q init: %w", ext.Name(), err)
		}
	}
	return nil
}

func (c combineExtensions) Dispose() error {
	var errs []error
	for _, ext := range c {
		if err := ext.Dispose(); err != nil {
			errs = append(errs, fmt.Errorf("extension %q dispose: %w", ext.Name(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("dispose errors: %v", errs)
	}
	return nil
}

// Tools 聚合所有子扩展的工具（ToolProvider）。
func (c combineExtensions) Tools() []*agentcore.Tool {
	var out []*agentcore.Tool
	for _, ext := range c {
		if tp, ok := ext.(agentcore.ToolProvider); ok {
			out = append(out, tp.Tools()...)
		}
	}
	return out
}

// 编译期接口合规检查。
var _ agentcore.Extension = (combineExtensions)(nil)
var _ agentcore.ToolProvider = (combineExtensions)(nil)

// buildLegalToolExt 为法律子 Agent 构建工具扩展（含文件/网络/视觉）。
func buildLegalToolExt(fc *frameworkContext) agentcore.Extension {
	cfg := baseToolConfig(fc)
	cfg.WebSearch = &tools.WebSearchToolConfig{}
	cfg.WebFetch = &tools.WebFetchToolConfig{}
	cfg.DisableTools = []string{
		tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
		tools.ToolBrowser, tools.ToolExecuteCode, tools.ToolComputerUse,
		tools.ToolProcess,
	}
	return tools.NewExtension(cfg)
}
