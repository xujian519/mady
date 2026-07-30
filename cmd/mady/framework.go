package main

// 本文件是 bootstrap.Context 的类型别名与 shim 函数层。
// 所有共享装配逻辑已迁移到 bootstrap/，此处仅保留薄兼容层
// 供 cmd/mady 下其他文件以 package main 标识符引用。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/bootstrap"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/fileindex"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
)

// frameworkContext 是 bootstrap.Context 的类型别名，保持向后兼容。
type frameworkContext = bootstrap.Context

// setupFrameworkContext 是 bootstrap.Setup 的薄封装，兼容现有调用方。
func setupFrameworkContext(ctx context.Context, cmdName string) *frameworkContext {
	mode := bootstrap.ModeDeferred
	if cmdName != "tui" {
		mode = bootstrap.ModeSync
	}
	fc, err := bootstrap.Setup(ctx, bootstrap.Options{
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
	data, err := util.ReadFile(path) // path is from filepath.Walk or filepath.Join of CWD
	if err != nil {
		return ""
	}
	return string(data)
}

// 以下 shim 函数保持 cmd/mady 其他文件的无缝编译。

// buildReasoningRetriever 委托到 bootstrap.BuildReasoningRetriever。
func buildReasoningRetriever(fc *frameworkContext) *reasoning.MultiSourceRetriever {
	return bootstrap.BuildReasoningRetriever(fc)
}

// buildCitationSource 委托到 bootstrap.BuildCitationSource。
//
//nolint:unused // used in stage2_wiring_test.go
func buildCitationSource(wikiRoot string) guardrails.CitationSource {
	return bootstrap.BuildCitationSource(wikiRoot)
}

// loadKnowledgeBackend 委托到 bootstrap.LoadKnowledgeBackend。
//
//nolint:unused // used in stage2_wiring_test.go
func loadKnowledgeBackend(madyHome string) (knowledge.KnowledgeBackend, string) {
	return bootstrap.LoadKnowledgeBackend(madyHome)
}

// resolveWikiRoot 委托到 bootstrap.ResolveWikiRoot。
//
//nolint:unused // used in stage2_wiring_test.go
func resolveWikiRoot(madyHome string) string {
	return bootstrap.ResolveWikiRoot(madyHome)
}

// cwdPartitionName 委托到 bootstrap.CwdPartitionName。
func cwdPartitionName(cwd string) string {
	return bootstrap.CwdPartitionName(cwd)
}

// agentThinking 委托到 bootstrap.AgentThinking。
func agentThinking(cfg *agentconfig.ThinkingConfig) *agentcore.ThinkingConfig {
	return bootstrap.AgentThinking(cfg)
}

// tasklistDirForCWD 委托到 bootstrap.TasklistDirForCWD。
//
//nolint:unused // used in cmd/mady/framework_test.go
func tasklistDirForCWD(baseDir, cwd string) string {
	return bootstrap.TasklistDirForCWD(baseDir, cwd)
}

// denyDangerousToolsExtension 委托到 bootstrap.DenyDangerousToolsExtension。
func denyDangerousToolsExtension() agentcore.Extension {
	return bootstrap.DenyDangerousToolsExtension()
}
