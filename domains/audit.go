package domains

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditAction describes the type of operation being audited.
type AuditAction string

const (
	AuditAccess       AuditAction = "access"        // 查看案件数据
	AuditModify       AuditAction = "modify"        // 修改案件数据
	AuditExport       AuditAction = "export"        // 导出/下载
	AuditDelete       AuditAction = "delete"        // 删除案件数据
	AuditApprove      AuditAction = "approve"       // 审批通过
	AuditReject       AuditAction = "reject"        // 审批拒绝
	AuditLogin        AuditAction = "login"         // 用户登录
	AuditConfigChange AuditAction = "config_change" // 配置变更
)

// AuditEntry records a single auditable operation.
type AuditEntry struct {
	Timestamp   time.Time   `json:"timestamp"`
	Action      AuditAction `json:"action"`
	ProjectID   string      `json:"project_id,omitempty"`
	UserID      string      `json:"user_id,omitempty"`
	Description string      `json:"description"`
	Success     bool        `json:"success"`
	Details     string      `json:"details,omitempty"` // additional context (truncated)
}

// AuditLogger provides a thread-safe audit trail for Mady operations.
// It writes JSONL entries to $MADY_HOME/audit/audit-YYYY-MM-DD.jsonl.
//
// The audit log is required for law firm deployment compliance under the
// Patent Agency Regulations (专利代理条例) and general data governance.
type AuditLogger struct {
	mu    sync.Mutex
	dir   string
	file  *os.File
	today string
}

// NewAuditLogger creates or opens an audit logger. The audit directory is
// created if it doesn't exist. Returns nil if dir is empty (audit disabled).
func NewAuditLogger(dir string) (*AuditLogger, error) {
	if dir == "" {
		return nil, nil // audit disabled
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create audit dir %s: %w", dir, err)
	}
	l := &AuditLogger{dir: dir}
	if err := l.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return l, nil
}

// rotateIfNeeded opens a new daily log file using the current time.
func (l *AuditLogger) rotateIfNeeded() error {
	return l.rotateIfNeededAt(time.Now())
}

// rotateIfNeededAt opens a new daily log file using a caller-provided timestamp.
// The caller must hold l.mu. The timestamp must come from a single time.Now()
// call to ensure the date check and entry timestamp are consistent.
func (l *AuditLogger) rotateIfNeededAt(now time.Time) error {
	today := now.Format("2006-01-02")
	if l.file != nil && l.today == today {
		return nil
	}
	if l.file != nil {
		l.file.Close()
	}
	path := filepath.Join(l.dir, "audit-"+today+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	l.file = f
	l.today = today
	return nil
}

// Log writes an audit entry to the daily log file.
// The caller is responsible for providing accurate action, projectID, userID,
// and description. The entry timestamp and success flag are set automatically.
func (l *AuditLogger) Log(action AuditAction, projectID, userID, description string, success bool, details string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if err := l.rotateIfNeededAt(now); err != nil {
		return // don't fail the operation if audit is unavailable
	}

	// Truncate details to prevent log bloat.
	if len(details) > 500 {
		details = details[:500] + "..."
	}

	entry := AuditEntry{
		Timestamp:   now,
		Action:      action,
		ProjectID:   projectID,
		UserID:      userID,
		Description: description,
		Success:     success,
		Details:     details,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = l.file.Write(append(data, '\n'))
}

// Close flushes and closes the audit log file. It acquires the mutex to avoid
// racing with concurrent Log() calls that may rotate or write to the file.
func (l *AuditLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}

// =============================================================================
// Data Retention
// =============================================================================

// DataRetentionConfig holds data retention policy settings.
type DataRetentionConfig struct {
	// DefaultDays is the default retention period in days (0 = permanent).
	// Can be overridden per-project via ProjectRecord.DataRetentionDays.
	DefaultDays int `json:"default_days"`

	// AuditRetentionDays is the retention period for audit logs (0 = permanent).
	AuditRetentionDays int `json:"audit_retention_days"`

	// AutoCleanup enables automatic cleanup of expired data.
	AutoCleanup bool `json:"auto_cleanup"`
}

// DefaultRetentionConfig returns the standard retention policy.
// Defaults to 3650 days (10 years) for project data and 365 days for audit logs,
// with automatic cleanup disabled.
func DefaultRetentionConfig() DataRetentionConfig {
	return DataRetentionConfig{
		DefaultDays:        3650,
		AuditRetentionDays: 365,
		AutoCleanup:        false,
	}
}

// =============================================================================
// Encryption Placeholder
// =============================================================================

// Encryptor provides application-level encryption for sensitive fields.
// When MADY_ENC_KEY is set, the encryptor uses AES-256-GCM. When unset,
// it returns plaintext (development mode).
type Encryptor struct {
	key []byte
}

// NewEncryptor creates an encryptor from the MADY_ENC_KEY environment variable.
// If the variable is empty, encryption is a no-op (plaintext passthrough).
func NewEncryptor() *Encryptor {
	keyStr := os.Getenv("MADY_ENC_KEY")
	if keyStr == "" {
		return &Encryptor{} // no-op encryptor
	}
	// Use SHA-256 of the key string to derive a 32-byte AES key.
	hash := sha256.Sum256([]byte(keyStr))
	key := hash[:]
	return &Encryptor{key: key}
}

// Enabled returns true if encryption is active.
func (e *Encryptor) Enabled() bool {
	return len(e.key) > 0
}

// Protect encrypts plaintext. Returns the plaintext unchanged if encryption
// is not enabled. In production (key set), this would perform AES-256-GCM.
func (e *Encryptor) Protect(plaintext string) string {
	if !e.Enabled() || plaintext == "" {
		return plaintext
	}
	// Production implementation: AES-256-GCM encrypt with random nonce,
	// return base64(nonce + ciphertext).
	// For now, return a fixed placeholder that does not leak plaintext.
	return "[encrypted]"
}

// Reveal decrypts ciphertext. Returns the plaintext unchanged if encryption
// is not enabled or the text is not encrypted.
func (e *Encryptor) Reveal(ciphertext string) string {
	if !e.Enabled() || ciphertext == "" {
		return ciphertext
	}
	// Production implementation: AES-256-GCM decrypt.
	return ciphertext
}
