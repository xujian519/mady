package casemgmt

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xujian519/mady/domains/config"
	_ "modernc.org/sqlite" // register sqlite driver
)

// 案件标识阶段。随着权威文档的提交，案件身份逐步升级。
const (
	StageDrafting  = "drafting"  // 撰写期：客户名+专利名称+类型+年份
	StageFiled     = "filed"     // 已申请：获得申请号
	StagePublished = "published" // 已公开：获得公开号
)

// 案件状态。
const (
	CaseStatusActive   = "active"
	CaseStatusArchived = "archived"
	CaseStatusGranted  = "granted"
	CaseStatusRejected = "rejected"
)

// 文档类型（权威信息来源）。
const (
	DocConfirmation = "confirmation"  // 专利申请确认书
	DocFiling       = "filing"        // 申请文件（定稿）
	DocAcceptance   = "acceptance"    // 受理通知书
	DocPublication  = "publication"   // 公开公告
	DocOfficeAction = "office_action" // 审查意见通知书
	DocGrant        = "grant"         // 授权通知书
	DocRejection    = "rejection"     // 驳回决定
	DocOther        = "other"
)

// CaseRecord 是案件索引库中的核心记录。
// 案件标识分两阶段：撰写期用复合键（ClientName+PatentTitle+PatentType+Year），
// 获得申请号后升级为 FilingNumber 作为唯一标识。
type CaseRecord struct {
	CaseID            string    `json:"case_id"`
	IdentityStage     string    `json:"identity_stage"`
	FilingNumber      string    `json:"filing_number,omitempty"`
	PublicationNumber string    `json:"publication_number,omitempty"`
	ClientName        string    `json:"client_name"`
	PatentTitle       string    `json:"patent_title"`
	PatentType        string    `json:"patent_type"`
	Year              int       `json:"year"`
	Domain            string    `json:"domain"`
	Status            string    `json:"status"`
	PrimaryPath       string    `json:"primary_path"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CasePath 是一个案件关联的文件目录。一个案件可关联多个路径。
type CasePath struct {
	CaseID    string    `json:"case_id"`
	Path      string    `json:"path"`
	Label     string    `json:"label,omitempty"` // 描述，如"客户提供的交底书"
	CreatedAt time.Time `json:"created_at"`
}

// CaseDocument 记录已解析的权威文档。
type CaseDocument struct {
	CaseID   string    `json:"case_id"`
	DocType  string    `json:"doc_type"`
	DocPath  string    `json:"doc_path"`
	DocHash  string    `json:"doc_hash"`
	ParsedAt time.Time `json:"parsed_at"`
}

// CaseEvent 记录案件状态变更日志。
type CaseEvent struct {
	CaseID    string    `json:"case_id"`
	EventType string    `json:"event_type"`
	EventData string    `json:"event_data,omitempty"` // JSON
	EventDate time.Time `json:"event_date"`
}

// CaseIndex 是基于 SQLite 的案件索引库。
// 替代原 ProjectRegistry 的 JSON 扁平文件，支持多维度检索和全文搜索。
type CaseIndex struct {
	mu sync.Mutex
	db *sql.DB
}

// NewCaseIndex 打开或创建案件索引库。dbPath 通常为 ~/.mady/workspace/cases.db。
func NewCaseIndex(dbPath string) (*CaseIndex, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("case_index: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)

	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("case_index: ping %s: %w", dbPath, err)
	}

	ci := &CaseIndex{db: db}
	if err := ci.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("case_index: init schema: %w", err)
	}
	return ci, nil
}

func (ci *CaseIndex) initSchema(ctx context.Context) error {
	_, err := ci.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS cases (
			case_id            TEXT PRIMARY KEY,
			identity_stage     TEXT NOT NULL DEFAULT 'drafting',
			filing_number      TEXT NOT NULL DEFAULT '',
			publication_number TEXT NOT NULL DEFAULT '',
			client_name        TEXT NOT NULL DEFAULT '',
			patent_title       TEXT NOT NULL DEFAULT '',
			patent_type        TEXT NOT NULL DEFAULT '',
			year               INTEGER NOT NULL DEFAULT 0,
			domain             TEXT NOT NULL DEFAULT 'patent',
			status             TEXT NOT NULL DEFAULT 'active',
			primary_path       TEXT NOT NULL DEFAULT '',
			created_at         TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_cases_filing  ON cases(filing_number);
		CREATE INDEX IF NOT EXISTS idx_cases_client  ON cases(client_name);
		CREATE INDEX IF NOT EXISTS idx_cases_year    ON cases(year);
		CREATE INDEX IF NOT EXISTS idx_cases_status  ON cases(status);

		CREATE TABLE IF NOT EXISTS case_paths (
			case_id    TEXT NOT NULL,
			path       TEXT NOT NULL,
			label      TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (case_id, path)
		);
		CREATE INDEX IF NOT EXISTS idx_paths_case ON case_paths(case_id);
		CREATE INDEX IF NOT EXISTS idx_paths_path ON case_paths(path);

		CREATE TABLE IF NOT EXISTS case_documents (
			case_id   TEXT NOT NULL,
			doc_type  TEXT NOT NULL,
			doc_path  TEXT NOT NULL,
			doc_hash  TEXT NOT NULL DEFAULT '',
			parsed_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (case_id, doc_type)
		);
		CREATE INDEX IF NOT EXISTS idx_docs_case ON case_documents(case_id);

		CREATE TABLE IF NOT EXISTS case_events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			case_id    TEXT NOT NULL,
			event_type TEXT NOT NULL,
			event_data TEXT NOT NULL DEFAULT '',
			event_date TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_events_case ON case_events(case_id);

		CREATE VIRTUAL TABLE IF NOT EXISTS cases_fts USING fts5(
			case_id,
			client_name,
			patent_title,
			filing_number,
			tokenize='trigram'
		);
	`)
	return err
}

// Close 关闭数据库连接。
func (ci *CaseIndex) Close() error {
	if ci.db != nil {
		return ci.db.Close()
	}
	return nil
}

// syncFTS 删除并重建指定案件的全文索引行。
// 调用方必须已持有 ci.mu（所有调用点都在锁内）。
func (ci *CaseIndex) syncFTS(ctx context.Context, caseID, clientName, patentTitle, filingNumber string) {
	if _, err := ci.db.ExecContext(ctx, `DELETE FROM cases_fts WHERE case_id = ?`, caseID); err != nil {
		slog.Warn("case_index: syncFTS delete failed", "case_id", caseID, "error", err)
	}
	if _, err := ci.db.ExecContext(ctx, `
		INSERT INTO cases_fts (case_id, client_name, patent_title, filing_number)
		VALUES (?, ?, ?, ?)
	`, caseID, clientName, patentTitle, filingNumber); err != nil {
		slog.Warn("case_index: syncFTS insert failed", "case_id", caseID, "error", err)
	}
}

// --- 检索 ---

// CaseSearchQuery 定义案件检索条件。空字段表示不过滤。
type CaseSearchQuery struct {
	FilingNumber  string
	ClientName    string
	PatentTitle   string
	PatentType    string
	Year          int
	Status        string
	IdentityStage string
	Text          string // 全文模糊匹配（走 FTS5）
}

// --- 辅助函数 ---

type scanFn func(dest ...any) error

func scanCase(scan scanFn) (*CaseRecord, error) {
	var rec CaseRecord
	var createdStr, updatedStr string
	err := scan(
		&rec.CaseID, &rec.IdentityStage, &rec.FilingNumber, &rec.PublicationNumber,
		&rec.ClientName, &rec.PatentTitle, &rec.PatentType, &rec.Year,
		&rec.Domain, &rec.Status, &rec.PrimaryPath,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	rec.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	rec.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &rec, nil
}

// PrimaryIdentity 返回案件当前阶段的唯一标识。
// drafting: "客户名-专利名称（类型·年份）"
// filed:    申请号
// published: 公开号
func (rec *CaseRecord) PrimaryIdentity() string {
	switch rec.IdentityStage {
	case StageFiled:
		return rec.FilingNumber
	case StagePublished:
		if rec.PublicationNumber != "" {
			return rec.PublicationNumber
		}
		return rec.FilingNumber
	default:
		return fmt.Sprintf("%s-%s（%s·%d）", rec.ClientName, rec.PatentTitle, rec.PatentType, rec.Year)
	}
}

// DisplayLabel 返回人类可读的案件标签（用于 UI 展示）。
func (rec *CaseRecord) DisplayLabel() string {
	if rec.FilingNumber != "" {
		return fmt.Sprintf("%s（%s）", rec.PatentTitle, rec.FilingNumber)
	}
	return fmt.Sprintf("%s（%s·%s）", rec.ClientName, rec.PatentTitle, rec.PatentType)
}

// ToProjectRecord 将 CaseRecord 转换为 TUI/Agent 层使用的 ProjectRecord。
// 桥接 SQLite 索引库与现有的 ProjectRecord 消费方，避免手动字段拷贝导致漂移。
func (rec *CaseRecord) ToProjectRecord() config.ProjectRecord {
	rootPath := rec.PrimaryPath
	return config.ProjectRecord{
		ProjectID:    rec.CaseID,
		Domain:       rec.Domain,
		Alias:        rec.DisplayLabel(),
		RootPath:     rootPath,
		Status:       rec.Status,
		CaseType:     rec.PatentType,
		FilingNumber: rec.FilingNumber,
	}
}

// ToProjectMeta 将 CaseRecord 转换为 TUI 层使用的 ProjectMeta。
func (rec *CaseRecord) ToProjectMeta() config.ProjectMeta {
	return config.ProjectMeta{
		ProjectID:  rec.CaseID,
		Domain:     rec.Domain,
		Alias:      rec.DisplayLabel(),
		RootPath:   rec.PrimaryPath,
		MatterType: rec.PatentType,
		Status:     rec.Status,
	}
}
