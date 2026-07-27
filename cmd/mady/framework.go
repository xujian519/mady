package main

// 本文件是 pkg/framework.Context 的类型别名与 shim 函数层。
// 所有共享装配逻辑已迁移到 pkg/framework/，此处仅保留薄兼容层
// 供 cmd/mady 下其他文件以 package main 标识符引用。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/framework"
)

// frameworkContext 是 framework.Context 的类型别名，保持向后兼容。
type frameworkContext = framework.Context

// setupFrameworkContext 是 framework.Setup 的薄封装，兼容现有调用方。
func setupFrameworkContext(ctx context.Context, cmdName string) *frameworkContext {
	mode := framework.ModeDeferred
	if cmdName != "tui" {
		mode = framework.ModeSync
	}
	fc, err := framework.Setup(ctx, framework.Options{
		Mode:    mode,
		CmdName: cmdName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mady: %v\n", err)
		os.Exit(1)
	}
	return fc
}

// caseFileReader implements domains.FileContentReader by wrapping fileindex.FileReader
// with an os.ReadFile fallback. Lives in cmd/mady (application layer) to keep
// domains free of knowledge/fileindex imports.
type caseFileReader struct{}

func (caseFileReader) ReadText(path string) string {
	dir := filepath.Dir(path)
	reader := fileindex.NewFileReader(dir)
	if result, err := reader.ReadProjectFile(context.Background(), filepath.Base(path)); err == nil {
		return result.Content
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// 以下 shim 函数保持 cmd/mady 其他文件的无缝编译。

// buildReasoningRetriever 委托到 framework.BuildReasoningRetriever。
func buildReasoningRetriever(fc *frameworkContext) *reasoning.MultiSourceRetriever {
	return framework.BuildReasoningRetriever(fc)
}

// buildCitationSource 委托到 framework.BuildCitationSource。
func buildCitationSource(wikiRoot string) guardrails.CitationSource {
	return framework.BuildCitationSource(wikiRoot)
}

// loadKnowledgeBackend 委托到 framework.LoadKnowledgeBackend。
func loadKnowledgeBackend(madyHome string) (knowledge.KnowledgeBackend, string) {
	return framework.LoadKnowledgeBackend(madyHome)
}

// resolveWikiRoot 委托到 framework.ResolveWikiRoot。
func resolveWikiRoot(madyHome string) string {
	return framework.ResolveWikiRoot(madyHome)
}

// cwdPartitionName 委托到 framework.CwdPartitionName。
func cwdPartitionName(cwd string) string {
	return framework.CwdPartitionName(cwd)
}

// extSlice 委托到 framework.ExtSlice。
func extSlice(ext agentcore.Extension) []agentcore.Extension {
	return framework.ExtSlice(ext)
}

// agentThinking 委托到 framework.AgentThinking。
func agentThinking(cfg *agentconfig.ThinkingConfig) *agentcore.ThinkingConfig {
	return framework.AgentThinking(cfg)
}

// tasklistDirForCWD 委托到 framework.tasklistDirForCWD。
func tasklistDirForCWD(baseDir, cwd string) string {
	return framework.TasklistDirForCWD(baseDir, cwd)
}
