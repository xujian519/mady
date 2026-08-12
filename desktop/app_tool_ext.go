//go:build darwin

package main

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/tools"

	"github.com/xujian519/mady/bootstrap"
)

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
