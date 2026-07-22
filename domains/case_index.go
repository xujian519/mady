package domains

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
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
	DocConfirmation = "confirmation" // 专利申请确认书
	DocFiling       = "filing"       // 申请文件（定稿）
	DocAcceptance   = "acceptance"   // 受理通知书
	DocPublication  = "publication"  // 公开公告
	DocOfficeAction = "office_action" // 审查意见通知书
	DocGrant        = "grant"        // 授权通知书
	DocRejection    = "rejection"    // 驳回决定
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
	CaseID    string    `json:"case_id"`
	DocType   string    `json:"doc_type"`
	DocPath   string    `json:"doc_path"`
	DocHash   string    `json:"doc_hash"`
	ParsedAt  time.Time `json:"parsed_at"`
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

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("case_index: ping %s: %w", dbPath, err)
	}

	ci := &CaseIndex{db: db}
	if err := ci.initSchema(context.Background()); err != nil {
		db.Close()
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
	ci.db.ExecContext(ctx, `DELETE FROM cases_fts WHERE case_id = ?`, caseID)
	ci.db.ExecContext(ctx, `
		INSERT INTO cases_fts (case_id, client_name, patent_title, filing_number)
		VALUES (?, ?, ?, ?)
	`, caseID, clientName, patentTitle, filingNumber)
}

// --- Case CRUD ---

// CreateCase 创建一条新案件记录。
func (ci *CaseIndex) CreateCase(ctx context.Context, rec CaseRecord) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	rec.UpdatedAt = rec.CreatedAt

	_, err := ci.db.ExecContext(ctx, `
		INSERT INTO cases (case_id, identity_stage, filing_number, publication_number,
			client_name, patent_title, patent_type, year, domain, status, primary_path,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.CaseID, rec.IdentityStage, rec.FilingNumber, rec.PublicationNumber,
		rec.ClientName, rec.PatentTitle, rec.PatentType, rec.Year, rec.Domain,
		rec.Status, rec.PrimaryPath, rec.CreatedAt.Format(time.RFC3339), rec.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("case_index: create: %w", err)
	}

	// 写入 FTS 索引
	ci.syncFTS(ctx, rec.CaseID, rec.ClientName, rec.PatentTitle, rec.FilingNumber)

	// 写入 primary_path 到 case_paths
	if rec.PrimaryPath != "" {
		_, _ = ci.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO case_paths (case_id, path) VALUES (?, ?)
		`, rec.CaseID, rec.PrimaryPath)
	}

	// 记录事件
	_, _ = ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type) VALUES (?, 'created')
	`, rec.CaseID)

	return nil
}

// GetCase 按 CaseID 查询案件。
func (ci *CaseIndex) GetCase(ctx context.Context, caseID string) (*CaseRecord, error) {
	row := ci.db.QueryRowContext(ctx, `
		SELECT case_id, identity_stage, filing_number, publication_number,
			client_name, patent_title, patent_type, year, domain, status, primary_path,
			created_at, updated_at
		FROM cases WHERE case_id = ?
	`, caseID)

	rec, err := scanCase(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("case_index: get %s: %w", caseID, err)
	}
	return rec, nil
}

// UpdateCase 更新案件记录。所有字段以 rec 为准。
func (ci *CaseIndex) UpdateCase(ctx context.Context, rec CaseRecord) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	rec.UpdatedAt = time.Now()

	_, err := ci.db.ExecContext(ctx, `
		UPDATE cases SET
			identity_stage = ?, filing_number = ?, publication_number = ?,
			client_name = ?, patent_title = ?, patent_type = ?, year = ?,
			domain = ?, status = ?, primary_path = ?, updated_at = ?
		WHERE case_id = ?
	`,
		rec.IdentityStage, rec.FilingNumber, rec.PublicationNumber,
		rec.ClientName, rec.PatentTitle, rec.PatentType, rec.Year,
		rec.Domain, rec.Status, rec.PrimaryPath, rec.UpdatedAt.Format(time.RFC3339),
		rec.CaseID,
	)
	if err != nil {
		return fmt.Errorf("case_index: update %s: %w", rec.CaseID, err)
	}

	// 同步 FTS 索引
	ci.syncFTS(ctx, rec.CaseID, rec.ClientName, rec.PatentTitle, rec.FilingNumber)

	return nil
}

// DeleteCase 删除案件及其所有关联数据。
func (ci *CaseIndex) DeleteCase(ctx context.Context, caseID string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	tx, err := ci.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("case_index: delete begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, q := range []string{
		`DELETE FROM cases WHERE case_id = ?`,
		`DELETE FROM case_paths WHERE case_id = ?`,
		`DELETE FROM case_documents WHERE case_id = ?`,
		`DELETE FROM case_events WHERE case_id = ?`,
		`DELETE FROM cases_fts WHERE case_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, caseID); err != nil {
			return fmt.Errorf("case_index: delete: %w", err)
		}
	}
	return tx.Commit()
}

// --- 路径管理 ---

// AddPath 为案件添加一个关联路径。
func (ci *CaseIndex) AddPath(ctx context.Context, caseID, path, label string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	_, err := ci.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO case_paths (case_id, path, label) VALUES (?, ?, ?)
	`, caseID, path, label)
	if err != nil {
		return fmt.Errorf("case_index: add path: %w", err)
	}
	return nil
}

// GetPaths 返回案件的所有关联路径。
func (ci *CaseIndex) GetPaths(ctx context.Context, caseID string) ([]CasePath, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT case_id, path, label, created_at FROM case_paths WHERE case_id = ?
	`, caseID)
	if err != nil {
		return nil, fmt.Errorf("case_index: get paths: %w", err)
	}
	defer rows.Close()

	var paths []CasePath
	for rows.Next() {
		var p CasePath
		var createdStr string
		if err := rows.Scan(&p.CaseID, &p.Path, &p.Label, &createdStr); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// FindByPath 按路径查找案件。匹配规则：case_paths.path == absPath 或 case_paths.path 是 absPath 的父目录。
func (ci *CaseIndex) FindByPath(ctx context.Context, absPath string) ([]CaseRecord, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT DISTINCT c.case_id, c.identity_stage, c.filing_number, c.publication_number,
			c.client_name, c.patent_title, c.patent_type, c.year, c.domain, c.status,
			c.primary_path, c.created_at, c.updated_at
		FROM cases c
		JOIN case_paths cp ON c.case_id = cp.case_id
		WHERE cp.path = ? OR ? LIKE cp.path || '/%'
	`, absPath, absPath)
	if err != nil {
		return nil, fmt.Errorf("case_index: find by path: %w", err)
	}
	defer rows.Close()

	var cases []CaseRecord
	for rows.Next() {
		rec, err := scanCase(rows.Scan)
		if err != nil {
			return nil, err
		}
		cases = append(cases, *rec)
	}
	return cases, rows.Err()
}

// --- 文档管理 ---

// RecordDocument 记录或更新已解析的权威文档。
func (ci *CaseIndex) RecordDocument(ctx context.Context, doc CaseDocument) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	if doc.ParsedAt.IsZero() {
		doc.ParsedAt = time.Now()
	}

	_, err := ci.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO case_documents (case_id, doc_type, doc_path, doc_hash, parsed_at)
		VALUES (?, ?, ?, ?, ?)
	`, doc.CaseID, doc.DocType, doc.DocPath, doc.DocHash, doc.ParsedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("case_index: record doc: %w", err)
	}
	return nil
}

// GetDocuments 返回案件的所有已记录文档。
func (ci *CaseIndex) GetDocuments(ctx context.Context, caseID string) ([]CaseDocument, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT case_id, doc_type, doc_path, doc_hash, parsed_at
		FROM case_documents WHERE case_id = ?
	`, caseID)
	if err != nil {
		return nil, fmt.Errorf("case_index: get docs: %w", err)
	}
	defer rows.Close()

	var docs []CaseDocument
	for rows.Next() {
		var d CaseDocument
		var parsedStr string
		if err := rows.Scan(&d.CaseID, &d.DocType, &d.DocPath, &d.DocHash, &parsedStr); err != nil {
			return nil, err
		}
		d.ParsedAt, _ = time.Parse(time.RFC3339, parsedStr)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// GetDocument 获取特定类型的文档。返回 sql.ErrNoRows 如果不存在。
func (ci *CaseIndex) GetDocument(ctx context.Context, caseID, docType string) (*CaseDocument, error) {
	var d CaseDocument
	var parsedStr string
	err := ci.db.QueryRowContext(ctx, `
		SELECT case_id, doc_type, doc_path, doc_hash, parsed_at
		FROM case_documents WHERE case_id = ? AND doc_type = ?
	`, caseID, docType).Scan(&d.CaseID, &d.DocType, &d.DocPath, &d.DocHash, &parsedStr)
	if err != nil {
		return nil, err
	}
	d.ParsedAt, _ = time.Parse(time.RFC3339, parsedStr)
	return &d, nil
}

// --- 事件管理 ---

// AddEvent 记录一条案件事件。
func (ci *CaseIndex) AddEvent(ctx context.Context, caseID, eventType, eventData string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	_, err := ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type, event_data) VALUES (?, ?, ?)
	`, caseID, eventType, eventData)
	if err != nil {
		return fmt.Errorf("case_index: add event: %w", err)
	}
	return nil
}

// GetEvents 返回案件的事件历史。
func (ci *CaseIndex) GetEvents(ctx context.Context, caseID string) ([]CaseEvent, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT case_id, event_type, event_data, event_date
		FROM case_events WHERE case_id = ? ORDER BY event_date
	`, caseID)
	if err != nil {
		return nil, fmt.Errorf("case_index: get events: %w", err)
	}
	defer rows.Close()

	var events []CaseEvent
	for rows.Next() {
		var e CaseEvent
		var dateStr string
		if err := rows.Scan(&e.CaseID, &e.EventType, &e.EventData, &dateStr); err != nil {
			return nil, err
		}
		e.EventDate, _ = time.Parse(time.RFC3339, dateStr)
		events = append(events, e)
	}
	return events, rows.Err()
}

// --- 标识升级 ---

// UpgradeToFiled 将案件从 drafting 升级为 filed，写入申请号。
// 如果 filingNumber 与已有关联案件的申请号冲突则返回错误。
func (ci *CaseIndex) UpgradeToFiled(ctx context.Context, caseID, filingNumber string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	// 检查申请号冲突
	var count int
	err := ci.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM cases WHERE filing_number = ? AND case_id != ?
	`, filingNumber, caseID).Scan(&count)
	if err != nil {
		return fmt.Errorf("case_index: upgrade check: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("申请号 %s 已被其他案件使用", filingNumber)
	}

	_, err = ci.db.ExecContext(ctx, `
		UPDATE cases SET identity_stage = ?, filing_number = ?, updated_at = ?
		WHERE case_id = ?
	`, StageFiled, filingNumber, time.Now().Format(time.RFC3339), caseID)
	if err != nil {
		return fmt.Errorf("case_index: upgrade to filed: %w", err)
	}

	// 同步 FTS（申请号已变更，需更新全文索引）
	var clientName, patentTitle, filingNum string
	_ = ci.db.QueryRowContext(ctx,
		`SELECT client_name, patent_title, filing_number FROM cases WHERE case_id = ?`, caseID,
	).Scan(&clientName, &patentTitle, &filingNum)
	ci.syncFTS(ctx, caseID, clientName, patentTitle, filingNum)

	// 记录事件
	_, _ = ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type, event_data)
		VALUES (?, 'filed', ?)
	`, caseID, filingNumber)

	return nil
}

// UpgradeToPublished 将案件升级为 published，写入公开号。
func (ci *CaseIndex) UpgradeToPublished(ctx context.Context, caseID, publicationNumber string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	_, err := ci.db.ExecContext(ctx, `
		UPDATE cases SET identity_stage = ?, publication_number = ?, updated_at = ?
		WHERE case_id = ?
	`, StagePublished, publicationNumber, time.Now().Format(time.RFC3339), caseID)
	if err != nil {
		return fmt.Errorf("case_index: upgrade to published: %w", err)
	}

	// 同步 FTS（保持一致性）
	var clientName, patentTitle, filingNum string
	_ = ci.db.QueryRowContext(ctx,
		`SELECT client_name, patent_title, filing_number FROM cases WHERE case_id = ?`, caseID,
	).Scan(&clientName, &patentTitle, &filingNum)
	ci.syncFTS(ctx, caseID, clientName, patentTitle, filingNum)

	_, _ = ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type, event_data)
		VALUES (?, 'published', ?)
	`, caseID, publicationNumber)

	return nil
}

// --- 检索 ---

// CaseSearchQuery 定义案件检索条件。空字段表示不过滤。
type CaseSearchQuery struct {
	FilingNumber   string
	ClientName     string
	PatentTitle    string
	PatentType     string
	Year           int
	Status         string
	IdentityStage  string
	Text           string // 全文模糊匹配（走 FTS5）
}

// SearchCases 按条件检索案件。
func (ci *CaseIndex) SearchCases(ctx context.Context, q CaseSearchQuery) ([]CaseRecord, error) {
	// 全文搜索走 FTS5
	if q.Text != "" {
		return ci.searchByFTS(ctx, q.Text)
	}

	// 结构化条件查询
	var conditions []string
	var args []any
	if q.FilingNumber != "" {
		conditions = append(conditions, "filing_number = ?")
		args = append(args, q.FilingNumber)
	}
	if q.ClientName != "" {
		conditions = append(conditions, "client_name LIKE ?")
		args = append(args, "%"+q.ClientName+"%")
	}
	if q.PatentTitle != "" {
		conditions = append(conditions, "patent_title LIKE ?")
		args = append(args, "%"+q.PatentTitle+"%")
	}
	if q.PatentType != "" {
		conditions = append(conditions, "patent_type = ?")
		args = append(args, q.PatentType)
	}
	if q.Year > 0 {
		conditions = append(conditions, "year = ?")
		args = append(args, q.Year)
	}
	if q.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, q.Status)
	}
	if q.IdentityStage != "" {
		conditions = append(conditions, "identity_stage = ?")
		args = append(args, q.IdentityStage)
	}

	query := `
		SELECT case_id, identity_stage, filing_number, publication_number,
			client_name, patent_title, patent_type, year, domain, status, primary_path,
			created_at, updated_at
		FROM cases`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY updated_at DESC"

	rows, err := ci.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("case_index: search: %w", err)
	}
	defer rows.Close()

	var cases []CaseRecord
	for rows.Next() {
		rec, err := scanCase(rows.Scan)
		if err != nil {
			return nil, err
		}
		cases = append(cases, *rec)
	}
	return cases, rows.Err()
}

func (ci *CaseIndex) searchByFTS(ctx context.Context, text string) ([]CaseRecord, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT c.case_id, c.identity_stage, c.filing_number, c.publication_number,
			c.client_name, c.patent_title, c.patent_type, c.year, c.domain, c.status,
			c.primary_path, c.created_at, c.updated_at
		FROM cases_fts fts
		JOIN cases c ON c.case_id = fts.case_id
		WHERE cases_fts MATCH ?
		ORDER BY rank
	`, text)
	if err != nil {
		return nil, fmt.Errorf("case_index: fts search: %w", err)
	}
	defer rows.Close()

	var cases []CaseRecord
	for rows.Next() {
		rec, err := scanCase(rows.Scan)
		if err != nil {
			return nil, err
		}
		cases = append(cases, *rec)
	}
	return cases, rows.Err()
}

// ListAll 返回所有案件，按更新时间倒序。
func (ci *CaseIndex) ListAll(ctx context.Context) ([]CaseRecord, error) {
	return ci.SearchCases(ctx, CaseSearchQuery{})
}

// FindByDraftingIdentity 按撰写期复合标识查找案件。
// 用于去重：同一客户+名称+类型+年份应只建一个案件。
func (ci *CaseIndex) FindByDraftingIdentity(ctx context.Context, clientName, patentTitle, patentType string, year int) ([]CaseRecord, error) {
	return ci.SearchCases(ctx, CaseSearchQuery{
		ClientName:  clientName,
		PatentTitle: patentTitle,
		PatentType:  patentType,
		Year:        year,
	})
}

// FindByFilingNumber 按申请号精确查找。
func (ci *CaseIndex) FindByFilingNumber(ctx context.Context, filingNumber string) (*CaseRecord, error) {
	cases, err := ci.SearchCases(ctx, CaseSearchQuery{FilingNumber: filingNumber})
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, sql.ErrNoRows
	}
	return &cases[0], nil
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
func (rec *CaseRecord) ToProjectRecord() ProjectRecord {
	rootPath := rec.PrimaryPath
	return ProjectRecord{
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
func (rec *CaseRecord) ToProjectMeta() ProjectMeta {
	return ProjectMeta{
		ProjectID:  rec.CaseID,
		Domain:     rec.Domain,
		Alias:      rec.DisplayLabel(),
		RootPath:   rec.PrimaryPath,
		MatterType: rec.PatentType,
		Status:     rec.Status,
	}
}
