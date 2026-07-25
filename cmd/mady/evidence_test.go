package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRunEvidenceCLI_Triple(t *testing.T) {
	input := `{"source_uri":"patent:CN12345678A","snippet":"权利要求1公开了一种图像识别方法"}`
	r := strings.NewReader(input)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("triple", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}
	if _, ok := result["overall_score"]; !ok {
		t.Error("missing 'overall_score' in output")
	}
}

func TestRunEvidenceCLI_Burden(t *testing.T) {
	input := `{"scenario":"patent_infringement"}`
	r := strings.NewReader(input)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("burden", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["holder"] != "专利权人" {
		t.Errorf("expected holder 专利权人, got %v", result["holder"])
	}
}

func TestRunEvidenceCLI_InvalidAction(t *testing.T) {
	r := strings.NewReader("{}")
	var buf bytes.Buffer
	exitCode := runEvidenceAction("invalid_action", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for invalid action, got %d", exitCode)
	}
}

func TestRunEvidenceCLI_InvalidJSON(t *testing.T) {
	r := strings.NewReader("not json")
	var buf bytes.Buffer
	exitCode := runEvidenceAction("triple", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for invalid JSON, got %d", exitCode)
	}
}

func TestRunEvidenceCLI_EmptyInput(t *testing.T) {
	r := strings.NewReader("  ")
	var buf bytes.Buffer
	exitCode := runEvidenceAction("triple", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for empty input, got %d", exitCode)
	}
}

func TestRunEvidenceCLI_Standard(t *testing.T) {
	input := `{"standard":"preponderance","supporting_count":3,"total_count":5}`
	r := strings.NewReader(input)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("standard", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["standard"] != "preponderance" {
		t.Errorf("expected standard preponderance, got %v", result["standard"])
	}
}

func TestRunEvidenceCLI_Standard_MissingField(t *testing.T) {
	r := strings.NewReader(`{"supporting_count":3,"total_count":5}`)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("standard", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for missing standard field, got %d", exitCode)
	}
}

func TestRunEvidenceCLI_Conflict(t *testing.T) {
	input := `{"claims":[{"claim_id":"claim1","supporting":["s1","s2"],"contradicting":["c1"]}]}`
	r := strings.NewReader(input)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("conflict", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	conflicts, ok := result["conflicts"]
	if !ok {
		t.Fatal("missing 'conflicts' in output")
	}
	cList, ok := conflicts.([]any)
	if !ok {
		t.Fatalf("conflicts should be []any, got %T", conflicts)
	}
	if len(cList) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(cList))
	}
}

func TestRunEvidenceCLI_TypeSpecific(t *testing.T) {
	input := `{"source_uri":"https://example.com/document.pdf"}`
	r := strings.NewReader(input)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("type-specific", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if _, ok := result["evidence_type"]; !ok {
		t.Error("missing 'evidence_type' in output")
	}
}

func TestRunEvidenceCLI_TypeSpecific_MissingField(t *testing.T) {
	r := strings.NewReader(`{}`)
	var buf bytes.Buffer
	exitCode := runEvidenceAction("type-specific", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for missing source_uri, got %d", exitCode)
	}
}
