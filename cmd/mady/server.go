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
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/domains"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/knowledge"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/pkg/util"
	rsqlite "github.com/xujian519/mady/retrieval/domain/sqlite"
	"github.com/xujian519/mady/server"
	"github.com/xujian519/mady/session"
)

func preflightWritableSQLitePath(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := util.EnsureDir(dir); err != nil {
		return fmt.Errorf("prepare db dir: %w", err)
	}

	probePath := filepath.Join(dir, fmt.Sprintf(".mady-write-probe-%d.tmp", time.Now().UnixNano()))
	probeFile, err := os.OpenFile(probePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("probe db dir writable: %w", err)
	}
	if _, err := probeFile.WriteString("probe"); err != nil {
		_ = probeFile.Close()
		_ = os.Remove(probePath)
		return fmt.Errorf("probe db dir write: %w", err)
	}
	if err := probeFile.Close(); err != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("close dir probe: %w", err)
	}
	if err := os.Remove(probePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleanup dir probe: %w", err)
	}

	if _, err := os.Stat(dbPath); err == nil {
		dbFile, openErr := os.OpenFile(dbPath, os.O_RDWR, 0)
		if openErr != nil {
			return fmt.Errorf("open existing db read-write: %w", openErr)
		}
		if closeErr := dbFile.Close(); closeErr != nil {
			return fmt.Errorf("close existing db handle: %w", closeErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat db path: %w", err)
	}

	return nil
}

func openEvalStore(evalDB string) (*knowledge.EvalStore, error) {
	if err := preflightWritableSQLitePath(evalDB); err != nil {
		return nil, fmt.Errorf("preflight eval store path: %w", err)
	}
	store, err := knowledge.NewEvalStore(knowledge.EvalStoreConfig{DSN: evalDB})
	if err != nil {
		return nil, fmt.Errorf("open eval store: %w", err)
	}
	return store, nil
}

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

	fc := setupFrameworkContext(ctx, "serve")

	// Build unified agent config.
	cfg := domains.UnifiedAgentConfig(fc.BaseConfig)

	// Attach wiki retrieval hook if available.
	if fc.WikiHook != nil {
		cfg.Lifecycle = agentcore.AppendLifecycle(cfg.Lifecycle, fc.WikiHook)
	}
	if fc.KnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, fc.KnowledgeExt)
	}

	// Session persistence via JSONL file store.
	// 优先级：$SESSION_DIR > ~/.mady/sessions。
	// MadyHome() 最终回退已调 filepath.Abs，不会出现 cwd 相对路径。
	sessionDir := os.Getenv("SESSION_DIR")
	if sessionDir == "" {
		if fc.MadyHome != "" {
			sessionDir = filepath.Join(fc.MadyHome, "sessions")
		} else {
			// 不可达兜底：MadyHome() 仅在 filepath.Abs 自身失败时返错。
			// 走 ResolveDataDir 以保证最终路径仍经过 filepath.Abs 规范化。
			dir, err := util.ResolveDataDir("sessions")
			if err != nil {
				log.Printf("resolve sessions dir: %v (falling back to empty)", err)
			}
			sessionDir = dir
		}
	}
	fileStore, err := session.NewFileStore(sessionDir)
	if err != nil {
		log.Printf("session: %v (continuing without persistence)", err)
	} else {
		// 使用 fc.WorkspaceDir 而非硬编码 "./workspace"，
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
		approvalDB := filepath.Join(approvalDir, "approvals.db")
		preflightErr := preflightWritableSQLitePath(approvalDB)
		if preflightErr == nil {
			if approvalStore, err := sqlitestore.NewApprovalStore(approvalDB); err == nil {
				srv.SetApprovalStore(approvalStore)
			} else {
				log.Printf("approval store: %s 不可用: %v（复核留痕不可用，/review 端点将返回 503）", approvalDB, err)
			}
		} else {
			log.Printf("approval store: %s 不可写: %v（复核留痕不可用，/review 端点将返回 503）", approvalDB, preflightErr)
		}
	}
	// 技术交底书分析现有技术检索器：从已打开的知识库构建 PatentDomainRetriever。
	// 配置后 disclosure 管线的 retrieve_prior_art 节点将使用本地专利知识库的 FTS5 检索
	// 返回专利文献作为证据，替代纯 LLM 自身知识的默认降级路径。
	// 当知识库不可用或类型不匹配时静默跳过（不影响现有行为）。
	if fc.KnowledgeBackend != nil {
		if store, ok := fc.KnowledgeBackend.(*ksqlite.SQLiteStore); ok {
			retriever := rsqlite.NewPatentDomainRetriever(store)
			if retriever != nil {
				srv.SetDisclosureRetriever(retriever)
				log.Printf("disclosure: PatentDomainRetriever 已接入（证据源: %s）", retriever.SourceName())
			}
		}
	}

	// Eval 评估数据持久化：启动 EvalConsumer 监听 eval_result 事件。
	// 评估指标（Faithfulness/AnswerRelevancy/ContextPrecision）原本只发送到
	// 事件总线但无人消费；现在写入 SQLite 并触发阈值告警。
	if fc.MadyHome != "" {
		evalDB := filepath.Join(fc.MadyHome, "eval.db")
		evalStore, err := openEvalStore(evalDB)
		if err != nil {
			log.Printf("eval store: %s 不可写: %v（仅禁用评估数据持久化，不影响主服务）", evalDB, err)
		} else {
			evalCfg := knowledge.DefaultEvalConfig()
			consumer := knowledge.NewEvalConsumer(evalStore,
				knowledge.WithAlertThreshold(evalCfg.AlertThreshold),
			)
			srv.OnAll(func(e iface.Event) {
				raw, ok := e.Payload().(agentcore.Event)
				if ok {
					consumer.OnEvent(raw)
				}
			})
			log.Printf("eval: 评估数据持久化已启用（%s），忠实度阈值: %.2f", evalDB, evalCfg.AlertThreshold)
		}
	}

	// 创造性分析触发器：disclosure 管线完成后自动运行三步法创造性评估。
	// 结果可通过 GET /v1/disclosure/analyze/{task_id} 的 inventiveness 字段查询。
	{
		provider := fc.BaseConfig.Provider
		if provider != nil {
			trigger := server.NewInventivenessTrigger(provider, srv.EventBus(),
				server.WithInventivenessResultHandler(srv.SetInventivenessResult),
			)
			trigger.Start()
			log.Printf("inventiveness: 创造性分析触发器已启动（独立子图模式）")
		}
	}
	// 26.3 充分公开评估触发器：disclosure 管线完成后自动运行充分公开判断。
	{
		provider := fc.BaseConfig.Provider
		if provider != nil {
			trigger := server.NewEnablementTrigger(provider, srv.EventBus(),
				server.WithEnablementResultHandler(srv.SetEnablementResult),
			)
			trigger.Start()
			log.Printf("enablement: 26.3 充分公开评估触发器已启动（独立子图模式）")
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
