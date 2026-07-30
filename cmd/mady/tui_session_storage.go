package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	reasoningsqlite "github.com/xujian519/mady/domains/reasoning/sqlite"
	sqlitestore "github.com/xujian519/mady/domains/sqlite"
	"github.com/xujian519/mady/pkg/util"
	// graph 包用于 PregelState 构建（斜杠命令直接调用工作流）
)

func (s *tuiSession) dbPath(name string) (string, error) {
	base := s.fc.WorkspaceDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "mady")
	}
	if err := os.MkdirAll(base, util.DefaultDirPerm); err != nil {
		return "", fmt.Errorf("db path: mkdir %s: %w", base, err)
	}
	return filepath.Join(base, name), nil
}

func (s *tuiSession) openApprovalStore() (domains.ApprovalStore, error) {
	p, err := s.dbPath("approvals.db")
	if err != nil {
		return nil, fmt.Errorf("approval store: %w", err)
	}
	return sqlitestore.NewApprovalStore(p)
}

func (s *tuiSession) openPendingStore() (domains.PendingStore, error) {
	p, err := s.dbPath("approvals.db")
	if err != nil {
		return nil, fmt.Errorf("pending store: %w", err)
	}
	return sqlitestore.NewApprovalStore(p)
}

func (s *tuiSession) openGraphCheckpointStore() (*sqlitestore.SQLiteGraphCheckpointStore, error) {
	p, err := s.dbPath("graph_checkpoints.db")
	if err != nil {
		return nil, fmt.Errorf("graph checkpoint: %w", err)
	}
	return sqlitestore.NewGraphCheckpointStore(p)
}

func (s *tuiSession) openEventStore() (*sqlitestore.SQLEventStore, error) {
	p, err := s.dbPath("events.db")
	if err != nil {
		return nil, fmt.Errorf("event store: %w", err)
	}
	return sqlitestore.NewEventStore(p)
}

// startEventLogger creates and starts an EventLogger for the given agent.
// If the event store cannot be opened, the logger is silently skipped.
// Previous eventLogger is closed before replacement.
func (s *tuiSession) startEventLogger(agent *agentcore.Agent) {
	// Close previous logger if any.
	if s.eventLogger != nil {
		s.eventLogger.Close()
		s.eventLogger = nil
	}
	store, err := s.openEventStore()
	if err != nil {
		log.Printf("[INFO] event store: unavailable (skipping event logging): %v", err)
		return
	}
	el := agentcore.NewEventLogger(store)
	el.Start(agent.EventBus())
	s.eventLogger = el
	log.Printf("[INFO] event logger: started")
}

// recoverPendingApprovals 在启动时检查是否有当前会话的待审批请求，
// 恢复最新的一个到 ApprovalGate 的运行时缓存中。
func (s *tuiSession) recoverPendingApprovals(ctx context.Context) {
	if s.pendingStore == nil || s.approvalGate == nil || s.currentThreadID == "" {
		return
	}
	pendings, err := s.pendingStore.ListPendingBySession(ctx, s.currentThreadID)
	if err != nil {
		log.Printf("[WARN] approval: recover pending: %v", err)
		return
	}
	if len(pendings) == 0 {
		return
	}
	if len(pendings) > 1 {
		log.Printf("[WARN] approval: %d pending for session %s, restoring most recent",
			len(pendings), s.currentThreadID)
	}
	latest := pendings[len(pendings)-1]
	s.approvalGate.RestorePending(latest)
	log.Printf("[INFO] approval: restored pending %s (keyword: %s)", latest.ID, latest.TriggerKeyword)
}

// startPendingExpirer 后台定期扫描过期待审批请求。
// 每 5 分钟检查一次 pending_approvals 表中已超时的行，将其标记为 expired。
// 启动时立刻执行一次，清理进程宕机期间积压的超时记录。
func (s *tuiSession) startPendingExpirer(ctx context.Context) {
	if s.pendingStore == nil {
		return
	}
	// Fire once immediately to clean up any backlog.
	if n, err := s.pendingStore.ExpirePending(ctx); err != nil {
		log.Printf("[WARN] approval: initial expire: %v", err)
	} else if n > 0 {
		log.Printf("[INFO] approval: expired %d pending approvals (backlog)", n)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.pendingStore.ExpirePending(ctx)
				if err != nil {
					log.Printf("[WARN] approval: expire scan: %v", err)
				} else if n > 0 {
					log.Printf("[INFO] approval: expired %d pending approvals", n)
				}
			}
		}
	}()
}

// openWorkflowCheckpointStore 打开 SQLite 工作流检查点存储。
// 参照 openApprovalStore 模式，使用 WorkspaceDir 作为基准路径。
// 返回错误时调用方应回退到内存存储。
func (s *tuiSession) openWorkflowCheckpointStore() (reasoning.CheckpointStore, error) {
	base := s.fc.WorkspaceDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "mady")
	}
	if err := os.MkdirAll(base, util.DefaultDirPerm); err != nil {
		return nil, fmt.Errorf("workflow checkpoint: mkdir %s: %w", base, err)
	}
	dbPath := filepath.Join(base, "workflow_checkpoints.db")
	store, err := reasoningsqlite.NewCheckpointStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("workflow checkpoint: open %s: %w", dbPath, err)
	}
	return store, nil
}
