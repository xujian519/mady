package domains

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ProjectRecord 是 registry.json 中的一条案件注册记录。
// 它只记录"案件在哪"，不存"案件里有什么"。
// RootPath 指向用户已有的物理案件文件夹（磁盘任意位置），
// Mady 不做移动/复制，通过动态 WorkingDir 在该文件夹内读写。
type ProjectRecord struct {
	ProjectID    string    `json:"project_id"`
	Domain       string    `json:"domain"`    // DomainPatent / DomainLegal
	Alias        string    `json:"alias"`     // 用户习惯称呼，如"老王的伸缩支架案"
	RootPath     string    `json:"root_path"` // 绝对路径，磁盘任意位置
	RegisteredAt time.Time `json:"registered_at"`
	LastAccessed time.Time `json:"last_accessed"`
	Status       string    `json:"status"` // "active" | "archived" | "unreachable"

	// --- 业务字段（v0.4 新增） ---

	// CaseType 是案件类型枚举，如 "发明专利"、"实用新型"、"外观设计"、"商标"、"著作权"。
	// 为空时表示未分类。
	CaseType string `json:"case_type,omitempty"`

	// FilingNumber 是官方申请号/注册号，如 "CN202410123456.7"。
	FilingNumber string `json:"filing_number,omitempty"`

	// ClientRef 是客户内部的案卷编号。
	ClientRef string `json:"client_ref,omitempty"`

	// Jurisdiction 是法域，如 "CN"、"US"、"EP"、"JP"。
	Jurisdiction string `json:"jurisdiction,omitempty"`

	// Confidentiality 是保密级别。
	// "public" | "internal" | "confidential" | "strictly_confidential"
	Confidentiality string `json:"confidentiality,omitempty"`

	// DataRetentionDays 是数据保留天数。0 表示永久保留。
	DataRetentionDays int `json:"data_retention_days,omitempty"`
}

// ProjectMeta 是案件元数据，存于 workspace 下，不写入用户物理文件夹。
type ProjectMeta struct {
	ProjectID  string     `json:"project_id"`
	Domain     string     `json:"domain"`
	Alias      string     `json:"alias"`
	RootPath   string     `json:"root_path"`
	MatterType string     `json:"matter_type,omitempty"`
	ClientName string     `json:"client_name,omitempty"`
	Deadlines  []Deadline `json:"deadlines,omitempty"`
	Status     string     `json:"status"`
}

// Deadline 代表一个法定期限提醒。
type Deadline struct {
	Type     string `json:"type"`     // 如 "答复审查意见通知书"
	DueDate  string `json:"due_date"` // ISO 8601 日期
	Reminded bool   `json:"reminded"`
}

const (
	registryFileName = "registry.json"
	metaFileName     = "meta.json"

	StatusActive      = "active"
	StatusArchived    = "archived"
	StatusUnreachable = "unreachable"
)

// ProjectRegistry 管理案件注册表和元数据。
// registry.json 存案件"指针"（ProjectRecord），
// workspace/projects/{projectID}/meta.json 存案件元数据（ProjectMeta）。
// 使用读写锁保证并发安全，registry 的持久化不走 agentcore.Store，
// 因为 ProjectRecord 不是 StateSnapshot。
type ProjectRegistry struct {
	mu      sync.RWMutex
	baseDir string // workspace/projects/ 目录
	records map[string]ProjectRecord
	dirty   bool
}

// NewProjectRegistry 创建或加载一个 ProjectRegistry。
// baseDir 是 workspace/projects/ 目录路径，该目录由调用方确保已创建。
func NewProjectRegistry(baseDir string) (*ProjectRegistry, error) {
	r := &ProjectRegistry{
		baseDir: baseDir,
		records: make(map[string]ProjectRecord),
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// NewProjectRegistryOrEmpty 创建 ProjectRegistry，加载失败时返回空的（不报错）。
// 仅用于单元测试和入口尚未初始化的场景。
// 注意：若 registry.json 损坏，加载失败会静默使用空注册表，但不会覆盖已损坏的文件。
// 生产代码应使用 NewProjectRegistry。
func NewProjectRegistryOrEmpty(baseDir string) *ProjectRegistry {
	r := &ProjectRegistry{
		baseDir: baseDir,
		records: make(map[string]ProjectRecord),
	}
	if err := r.load(); err != nil {
		log.Printf("[project] registry load failed, starting empty: %v", err)
	}
	return r
}

// Register 注册一个新案件。如果 RootPath 已存在则更新记录。
func (r *ProjectRegistry) Register(rec ProjectRecord) error {
	if err := ValidateProjectPath(rec.RootPath); err != nil {
		return fmt.Errorf("案件目录校验失败: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查重复 RootPath
	for id, existing := range r.records {
		if id == rec.ProjectID {
			continue
		}
		if filepath.Clean(existing.RootPath) == filepath.Clean(rec.RootPath) {
			return fmt.Errorf("该路径已被案件 %q (%s) 使用", id, existing.Alias)
		}
	}

	if rec.RegisteredAt.IsZero() {
		rec.RegisteredAt = time.Now()
	}
	rec.LastAccessed = time.Now()
	rec.Status = StatusActive

	r.records[rec.ProjectID] = rec
	r.dirty = true
	return r.persistLocked()
}

// Lookup 按 ProjectID 查询案件记录。
func (r *ProjectRegistry) Lookup(projectID string) (ProjectRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[projectID]
	return rec, ok
}

// List 返回所有案件的快照。
func (r *ProjectRegistry) List() []ProjectRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProjectRecord, 0, len(r.records))
	for _, rec := range r.records {
		result = append(result, rec)
	}
	return result
}

// Delete 删除一条案件记录。不会触碰用户的物理文件夹。
func (r *ProjectRegistry) Delete(projectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.records[projectID]; !ok {
		return fmt.Errorf("案件 %q 不存在", projectID)
	}
	delete(r.records, projectID)
	r.dirty = true
	return r.persistLocked()
}

// Touch 更新案件最后访问时间。
func (r *ProjectRegistry) Touch(projectID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.records[projectID]; ok {
		rec.LastAccessed = time.Now()
		r.records[projectID] = rec
		r.dirty = true
	}
}

// RefreshStatus 检查所有案件的 RootPath 可用性，更新 Status。
func (r *ProjectRegistry) RefreshStatus() {
	// Collect paths under lock, validate outside lock (I/O), then update under lock.
	r.mu.Lock()
	type pathCheck struct {
		id      string
		rootDir string
	}
	checks := make([]pathCheck, 0, len(r.records))
	for id, rec := range r.records {
		checks = append(checks, pathCheck{id: id, rootDir: rec.RootPath})
	}
	r.mu.Unlock()

	type statusUpdate struct {
		id     string
		status string
	}
	var updates []statusUpdate
	for _, c := range checks {
		if err := ValidateProjectPath(c.rootDir); err != nil {
			updates = append(updates, statusUpdate{id: c.id, status: StatusUnreachable})
		} else {
			updates = append(updates, statusUpdate{id: c.id, status: StatusActive})
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range updates {
		rec := r.records[u.id]
		if rec.Status != u.status {
			rec.Status = u.status
			r.records[u.id] = rec
			r.dirty = true
		}
	}
	if r.dirty {
		if err := r.persistLocked(); err != nil {
			log.Printf("[project] registry persist failed: %v", err)
		}
	}
}

// --- 元数据 ---

// projectDir 返回案件数据目录（workspace/projects/{projectID}/）。
func (r *ProjectRegistry) projectDir(projectID string) string {
	return filepath.Join(r.baseDir, projectID)
}

// SaveMeta 保存案件元数据到 workspace/projects/{projectID}/meta.json。
func (r *ProjectRegistry) SaveMeta(projectID string, meta *ProjectMeta) error {
	dir := r.projectDir(projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建案件数据目录: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 meta: %w", err)
	}

	path := filepath.Join(dir, metaFileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 meta: %w", err)
	}
	return nil
}

// LoadMeta 加载案件元数据。
func (r *ProjectRegistry) LoadMeta(projectID string) (*ProjectMeta, error) {
	path := filepath.Join(r.projectDir(projectID), metaFileName)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("meta 不存在: %s", projectID)
		}
		return nil, fmt.Errorf("打开 meta: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("读取 meta: %w", err)
	}

	var meta ProjectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析 meta: %w", err)
	}
	return &meta, nil
}

// --- 持久化 ---

// registryPath 返回 registry.json 的完整路径。
func (r *ProjectRegistry) registryPath() string {
	return filepath.Join(r.baseDir, registryFileName)
}

// load 从磁盘加载 registry.json。
func (r *ProjectRegistry) load() error {
	path := r.registryPath()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在 = 空注册表
		}
		return fmt.Errorf("打开 registry: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("读取 registry: %w", err)
	}

	// 兼容空文件
	if len(data) == 0 {
		return nil
	}

	var records map[string]ProjectRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("解析 registry: %w", err)
	}
	r.records = records
	r.dirty = false
	return nil
}

// persistLocked 写入 registry.json（调用者持有锁）。
func (r *ProjectRegistry) persistLocked() error {
	if err := os.MkdirAll(r.baseDir, 0o755); err != nil {
		return fmt.Errorf("创建 registry 目录: %w", err)
	}

	data, err := json.MarshalIndent(r.records, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 registry: %w", err)
	}

	path := r.registryPath()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 registry: %w", err)
	}
	r.dirty = false
	return nil
}

// --- 路径校验（导出以供外部逻辑复用） ---

// ValidateProjectPath 校验路径是否存在、是否可访问、是否为目录。
func ValidateProjectPath(p string) error {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return fmt.Errorf("路径解析失败: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("路径不可访问: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("必须是文件夹路径: %s", absPath)
	}
	return nil
}
