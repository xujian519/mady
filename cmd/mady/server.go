package main

// This file implements the `mady serve` subcommand: an HTTP/SSE API server
// with multi-domain routing built from the shared framework context.

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/xujian519/mady/agentcore"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
)

// runServer launches the HTTP/SSE API server with multi-domain routing.
func runServer(ctx context.Context) {
	fs := flag.NewFlagSet("mady serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	tlsCert := fs.String("tls-cert", "", "TLS 证书文件路径（与 -tls-key 同时提供时启用 HTTPS）")
	tlsKey := fs.String("tls-key", "", "TLS 私钥文件路径（与 -tls-cert 同时提供时启用 HTTPS）")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "flag: %v\n", err)
		return
	}
	// 仅提供其一属于配置错误，直接 fail-fast，避免静默降级为明文 HTTP。
	if (*tlsCert == "") != (*tlsKey == "") {
		fmt.Fprintln(os.Stderr, "server: -tls-cert 与 -tls-key 必须同时提供")
		return
	}

	fc := setupFrameworkContext(ctx)

	// Build Router config from manifests (or use hardcoded fallback).
	cfg := buildRouterConfig(fc.BaseConfig, fc.Manifests)

	// Attach wiki retrieval hook if available.
	if fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
	}
	if fc.KnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
	}

	// Session persistence via JSONL file store.
	// 优先级：$SESSION_DIR > ~/.mady/sessions。
	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		if fc.MadyHome != "" {
			sessionDir = filepath.Join(fc.MadyHome, "sessions")
		} else {
			sessionDir = "./sessions" // 降级兜底
		}
	}
	fileStore, err := session.NewFileStore(sessionDir)
	if err != nil {
		log.Printf("session: %v (continuing without persistence)", err)
	} else {
		// 修复：使用 fc.WorkspaceDir 而非硬编码 "./workspace"，
		// 确保与 ProjectRegistry、AgentStore 共用同一 workspace。
		cfg.Store = session.NewAgentStore(fileStore, fc.WorkspaceDir)
	}

	// Checkpoint for durable snapshots per thread.
	cfg.Checkpoint = &agentcore.CheckpointSettings{
		Saver:    agentcore.NewMemoryCheckpointSaver(),
		ThreadID: "default",
	}

	srv := server.New(cfg)

	// 审批留痕：disclosure 复核端点等 HITL 触点的人工决策（采纳/修改/拒绝）
	// 持久化到 SQLite，与 TUI 共用同一 approvals.db，供 P3 专家盲测与采纳率
	// 统计使用。打开失败仅降级为端点 503，不影响其余服务。
	approvalDir := fc.WorkspaceDir
	if approvalDir == "" {
		approvalDir = fc.MadyHome
	}
	if approvalDir != "" {
		if err := os.MkdirAll(approvalDir, 0o755); err == nil {
			if approvalStore, err := sqlitestore.NewApprovalStore(filepath.Join(approvalDir, "approvals.db")); err == nil {
				srv.SetApprovalStore(approvalStore)
			} else {
				log.Printf("approval store: %v（复核留痕不可用，/review 端点将返回 503）", err)
			}
		}
	}
	if *tlsCert != "" {
		log.Printf("Mady server starting on %s with TLS (multi-domain routing enabled)", *addr)
	} else {
		log.Printf("Mady server starting on %s (multi-domain routing enabled)", *addr)
		// 默认保持明文 HTTP 的本地开发体验；对外暴露时应使用 -tls-cert/-tls-key
		// 或在前置反向代理（nginx/caddy 等）处终止 TLS。
		log.Printf("server: 未启用 TLS；对外暴露请使用 -tls-cert/-tls-key 或 TLS 反向代理")
	}
	if fc.WikiStore != nil {
		st := fc.WikiStore.Stats()
		log.Printf("wiki: %d docs, %d chunks", st.TotalDocs, st.TotalChunks)
	}

	// Graceful shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if *tlsCert != "" {
		err := srv.ListenAndServeTLS(*addr, *tlsCert, *tlsKey)
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server: %v\n", err)
		}
		return
	}
	if err := srv.ListenAndServe(*addr); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		return
	}
}
