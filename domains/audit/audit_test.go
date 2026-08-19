package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAuditLogger_LogWritesJSONL verifies the core audit write path: a log
// entry is appended as one JSONL line to the daily file.
func TestAuditLogger_LogWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	l, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	defer l.Close()

	l.Log(AuditAccess, "proj-1", "user-1", "查看案件数据", true, "details")

	path := filepath.Join(dir, "audit-"+time.Now().Format("2006-01-02")+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %q", len(lines), string(data))
	}
	var entry AuditEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if entry.Action != AuditAccess || entry.ProjectID != "proj-1" || entry.UserID != "user-1" {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if !entry.Success || entry.Description != "查看案件数据" {
		t.Errorf("unexpected entry: %+v", entry)
	}
}

// TestAuditLogger_DetailsTruncated verifies long details are truncated to
// prevent log bloat.
func TestAuditLogger_DetailsTruncated(t *testing.T) {
	dir := t.TempDir()
	l, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	defer l.Close()

	long := strings.Repeat("x", 1000)
	l.Log(AuditModify, "", "", "modify", true, long)

	path := filepath.Join(dir, "audit-"+time.Now().Format("2006-01-02")+".jsonl")
	data, _ := os.ReadFile(path)
	if len(data) >= 1000 {
		t.Fatalf("details not truncated, file size %d", len(data))
	}
	if !strings.Contains(string(data), "...") {
		t.Errorf("expected truncation marker in %q", string(data))
	}
}

// TestAuditLogger_NilSafe verifies nil receiver calls are no-ops.
func TestAuditLogger_NilSafe(t *testing.T) {
	var l *AuditLogger
	l.Log(AuditAccess, "", "", "desc", true, "")
	if err := l.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}

// TestAuditLogger_DisabledWhenEmptyDir verifies an empty dir disables auditing.
func TestAuditLogger_DisabledWhenEmptyDir(t *testing.T) {
	l, err := NewAuditLogger("")
	if err != nil {
		t.Fatalf("NewAuditLogger(''): %v", err)
	}
	if l != nil {
		t.Fatalf("expected nil logger for empty dir, got %+v", l)
	}
}

// TestEncryptor_NoopWithoutKey verifies encryption is a no-op when no key is set.
func TestEncryptor_NoopWithoutKey(t *testing.T) {
	t.Setenv("MADY_ENC_KEY", "")
	e := NewEncryptor()
	if e.Enabled() {
		t.Fatal("expected disabled encryptor without key")
	}
	if got := e.Protect("敏感数据"); got != "敏感数据" {
		t.Errorf("Protect no-op = %q", got)
	}
	if got := e.Reveal("敏感数据"); got != "敏感数据" {
		t.Errorf("Reveal no-op = %q", got)
	}
}

// TestEncryptor_RoundTrip verifies AES-256-GCM encryption round-trips.
func TestEncryptor_RoundTrip(t *testing.T) {
	t.Setenv("MADY_ENC_KEY", "test-passphrase")
	e := NewEncryptor()
	if !e.Enabled() {
		t.Fatal("expected enabled encryptor with key")
	}
	plain := "案件敏感信息"
	ct := e.Protect(plain)
	if ct == plain || ct == "" {
		t.Fatalf("Protect produced no ciphertext: %q", ct)
	}
	if got := e.Reveal(ct); got != plain {
		t.Errorf("round-trip = %q, want %q", got, plain)
	}
	// Tampered ciphertext must not reveal.
	if got := e.Reveal(ct + "tampered"); got == plain {
		t.Errorf("tampered ciphertext unexpectedly revealed plaintext")
	}
}

// TestEncryptor_RevealInvalidInput verifies Reveal returns input unchanged for
// non-AES-GCM input (migration compatibility).
func TestEncryptor_RevealInvalidInput(t *testing.T) {
	t.Setenv("MADY_ENC_KEY", "test-passphrase")
	e := NewEncryptor()
	if got := e.Reveal("not-base64!!!"); got != "not-base64!!!" {
		t.Errorf("Reveal invalid = %q", got)
	}
}
