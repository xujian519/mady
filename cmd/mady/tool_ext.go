package main

// 本文件提供工具扩展的构建函数，供 cmd/mady 下各入口（tui/serve/acp）共享。
// 遵循被动注入模式：域层不再导入 tools 包创建扩展，由入口层装配后注入。

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
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
	return tools.NewExtension(cfg)
}

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
