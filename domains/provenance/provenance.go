// Package provenance provides PROV-O-lite style runtime trace logging for
// patent workflows: every workflow step, HITL suspension, plan lifecycle or
// contract validation writes a JSONL provenance event, so a run can be
// reconstructed end-to-end for review and audit. It mirrors the design of
// audit.AuditLogger and is deliberately fail-open — provenance must never
// break the workflow step it traces.
package provenance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/xujian519/mady/domains/audit"
	"github.com/xujian519/mady/pkg/util"
)

// Event kinds for provenance events. Kind scopes the event so consumers can
// filter a trace by what happened, independently of which tool emitted it.
const (
	KindWorkflowStep      = "workflow_step"
	KindOutputgateSuspend = "outputgate_suspend"
	KindPlanLifecycle     = "plan_lifecycle"
	KindContractValidate  = "contract_validation"
)

// ProvenanceEvent is one traced step (or suspension) in a patent workflow run.
// CaseID / WorkflowID are optional; when provided they scope the event to a
// specific case or orchestration so the trace can be filtered per case.
//
//nolint:revive // 类型名带 Provenance 前缀与包名略重叠，但表意清晰且与计划文档命名一致
type ProvenanceEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	Kind       string    `json:"kind"`
	Tool       string    `json:"tool"`
	CaseID     string    `json:"case_id,omitempty"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	Details    string    `json:"details,omitempty"`
}

// ProvenanceLogger is a thread-safe JSONL provenance trail writer. It writes to
// a per-day file under a base directory.
//
// Encryption: when MADY_ENC_KEY is set the Details field is encrypted with
// AES-256-GCM (via audit.Encryptor) before writing; when unset it passes
// through as plaintext (development mode).
//
//nolint:revive // 类型名带 Provenance 前缀与包名略重叠，但表意清晰且与计划文档命名一致
type ProvenanceLogger struct {
	mu    sync.Mutex
	dir   string
	file  *os.File
	today string
	enc   *audit.Encryptor
}

// NewProvenanceLogger opens a provenance logger rooted at dir. An empty dir
// disables provenance (returns nil) so callers can quietly no-op.
func NewProvenanceLogger(dir string) (*ProvenanceLogger, error) {
	if dir == "" {
		return nil, nil // provenance disabled
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create provenance dir %s: %w", dir, err)
	}
	l := &ProvenanceLogger{dir: dir, enc: audit.NewEncryptor()}
	if err := l.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return l, nil
}

// DefaultProvenanceLogger opens the global provenance logger at
// $MADY_HOME/provenance/. Returns nil when MadyHome is unavailable (disabled).
func DefaultProvenanceLogger() (*ProvenanceLogger, error) {
	home, err := util.MadyHome()
	if err != nil || home == "" {
		return nil, nil
	}
	return NewProvenanceLogger(filepath.Join(home, "provenance"))
}

// rotateIfNeeded opens a new daily log file using the current time.
func (l *ProvenanceLogger) rotateIfNeeded() error {
	return l.rotateIfNeededAt(time.Now())
}

// rotateIfNeededAt opens a new daily log file for the provided timestamp. The
// caller must hold l.mu. The timestamp must come from a single time.Now() call
// to keep the date check and entry timestamp consistent.
func (l *ProvenanceLogger) rotateIfNeededAt(now time.Time) error {
	today := now.Format("2006-01-02")
	if l.file != nil && l.today == today {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	path := filepath.Join(l.dir, "provenance-"+today+".jsonl")
	f, err := util.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	l.file = f
	l.today = today
	return nil
}

// Log writes a provenance event to the daily log file. It is safe for concurrent
// use, and a nil logger is a silent no-op. Errors are returned so tests can
// assert; production callers may ignore them (fail-open).
func (l *ProvenanceLogger) Log(e ProvenanceEvent) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if err := l.rotateIfNeededAt(now); err != nil {
		return err
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	if e.Details != "" && l.enc.Enabled() {
		e.Details = l.enc.Protect(e.Details)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = l.file.Write(append(data, '\n'))
	return err
}

// Close flushes and closes the log file. A nil logger is a no-op.
func (l *ProvenanceLogger) Close() error {
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
