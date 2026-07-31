package main

// This file implements the `mady acp` subcommand: run as an ACP
// (Agent Client Protocol) server over stdio JSON-RPC, for editors like Zed.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/util"
)

func runAcp(ctx context.Context) error {
	fs := flag.NewFlagSet("mady acp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("mady acp: %w", err)
	}

	fc := setupFrameworkContext(ctx, "acp")

	// 构建三个工具扩展（统一 Agent、专利子 Agent、法律子 Agent），
	// 通过被动注入模式传入域层配置函数。
	unifiedExt := buildUnifiedToolExt(fc)
	patentExt := buildPatentToolExt(fc)
	legalExt := buildLegalToolExt(fc)
	cfg := domains.UnifiedAgentConfig(fc.BaseConfig, unifiedExt, patentExt, legalExt, withPlantaskAutoEnter(fc))
	// ACP 运行在无交互的编辑器环境中，危险工具默认拒绝。
	cfg.Extensions = append(cfg.Extensions,
		denyDangerousToolsExtension(),
	)
	if fc.KnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
	}
	if fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
	}

	opts := acp.RunOptions{
		Provider:   fc.Provider,
		Model:      agentconfig.DefaultModel(),
		Thinking:   agentThinking(agentconfig.ThinkingFromEnv()),
		BaseConfig: cfg,
		AgentInfo: acp.AgentInfo{
			Name:    "mady",
			Version: "0.1.0",
		},
	}
	// 设置 MADY_ACP_TOKEN 环境变量即启用静态令牌认证（客户端须在
	// authenticate 请求中携带同一令牌）；未设置时保持本地开发体验，
	// 服务端启动日志会给出未认证警告。
	if token := os.Getenv("MADY_ACP_TOKEN"); token != "" {
		opts.AuthProvider = acp.NewTokenAuthProvider(token)
	}

	// 工具授权等人工决策留痕到 SQLite（与 TUI/Server 共用 approvals.db），
	// 供 P3 专家盲测的 HITL 触点数据收集；打开失败降级为不留痕。
	if fc.WorkspaceDir != "" {
		if err := os.MkdirAll(fc.WorkspaceDir, util.DefaultDirPerm); err == nil {
			if store, err := sqlitestore.NewApprovalStore(filepath.Join(fc.WorkspaceDir, "approvals.db")); err == nil {
				opts.ApprovalStore = store
			} else {
				fmt.Fprintf(os.Stderr, "mady acp: approval store 不可用（%v），工具授权决策将不留痕\n", err)
			}
		}
	}

	err := acp.RunServer(ctx, opts)
	if err != nil && err != context.Canceled {
		return fmt.Errorf("mady acp: %w", err)
	}
	return nil
}
