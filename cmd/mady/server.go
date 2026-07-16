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
	"github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
)

// runServer launches the HTTP/SSE API server with multi-domain routing.
func runServer(ctx context.Context) {
	fs := flag.NewFlagSet("mady serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "flag: %v\n", err)
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
	log.Printf("Mady server starting on %s (multi-domain routing enabled)", *addr)
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

	if err := srv.ListenAndServe(*addr); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		return
	}
}
