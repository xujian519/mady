package provenance

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/xujian519/mady/domains/audit"
)

func TestNewProvenanceLogger_EmptyDirDisabled(t *testing.T) {
	l, err := NewProvenanceLogger("")
	if err != nil {
		t.Fatalf("NewProvenanceLogger(\"\"): %v", err)
	}
	if l != nil {
		t.Fatal("expected nil logger for empty dir (provenance disabled)")
	}
}

func TestNewProvenanceLogger_LogAndReadBack(t *testing.T) {
	dir := t.TempDir()
	l, err := NewProvenanceLogger(dir)
	if err != nil {
		t.Fatalf("NewProvenanceLogger: %v", err)
	}
	ev := ProvenanceEvent{
		Kind:       KindWorkflowStep,
		Tool:       "patent_plan_task",
		CaseID:     "case-1",
		WorkflowID: "wf-9",
		Details:    "创建计划",
	}
	if err := l.Log(ev); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "provenance-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one provenance file, got %v (err=%v)", files, err)
	}
	f, err := os.Open(files[0])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected at least one line")
	}
	var got ProvenanceEvent
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Tool != ev.Tool || got.Kind != ev.Kind || got.CaseID != ev.CaseID || got.WorkflowID != ev.WorkflowID {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	// 无 MADY_ENC_KEY 时 Details 明文。
	if got.Details != "创建计划" {
		t.Errorf("details: got %q, want plaintext 创建计划", got.Details)
	}
}

func TestProvenanceLogger_EncryptedDetails(t *testing.T) {
	t.Setenv("MADY_ENC_KEY", "secret")
	dir := t.TempDir()
	l, err := NewProvenanceLogger(dir)
	if err != nil {
		t.Fatalf("NewProvenanceLogger: %v", err)
	}
	if err := l.Log(ProvenanceEvent{Kind: KindContractValidate, Tool: "patent_worker_validate", Details: "敏感细节"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "provenance-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected one file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got ProvenanceEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Details == "敏感细节" {
		t.Fatal("details should be encrypted (not plaintext)")
	}
	// 解密回读须还原原文。
	enc := audit.NewEncryptor()
	if revealed := enc.Reveal(got.Details); revealed != "敏感细节" {
		t.Errorf("reveal: got %q, want 敏感细节", revealed)
	}
}

func TestProvenanceLogger_Concurrent(t *testing.T) {
	dir := t.TempDir()
	l, err := NewProvenanceLogger(dir)
	if err != nil {
		t.Fatalf("NewProvenanceLogger: %v", err)
	}

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			_ = l.Log(ProvenanceEvent{Kind: KindWorkflowStep, Tool: "t"}) //nolint:errcheck // concurrent noise is fine
		}()
	}
	wg.Wait()
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "provenance-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected one file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != n {
		t.Errorf("expected %d lines, got %d", n, lines)
	}
}

func TestDefaultProvenanceLogger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MADY_HOME", home)
	l, err := DefaultProvenanceLogger()
	if err != nil {
		t.Fatalf("DefaultProvenanceLogger: %v", err)
	}
	if l == nil {
		t.Fatal("expected a logger under a valid MADY_HOME")
	}
	if err := l.Log(ProvenanceEvent{Kind: KindOutputgateSuspend, Tool: "outputgate"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(home, "provenance", "provenance-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected one provenance file under $MADY_HOME/provenance, got %v", files)
	}
}
