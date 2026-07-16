package wiring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xujian519/mady/domains/reasoning"
)

// ConfirmedRuleWriter persists human-confirmed rule sets to disk so that
// future cases with similar case types can retrieve "historical confirmed
// rule combinations" as default suggestions (design-rule-acquisition-stage.md
// §3.2 Wiki 双向沉淀 — 确认后回写).
//
// Files are written as JSON under a dedicated directory (default
// $MADY_HOME/knowledge/confirmed-rules/), NOT into the wiki tree itself —
// the wiki is a symlink to the xiaonuo source corpus and writing there would
// pollute upstream data. A sibling directory keeps confirmed-rule history
// separate, structured, and reviewable without touching source material.
type ConfirmedRuleWriter struct {
	dir string
}

// NewConfirmedRuleWriter creates a writer targeting the given directory.
// The directory is created on first write if it does not exist. Returns nil
// if dir is empty (caller should skip write-back).
func NewConfirmedRuleWriter(dir string) *ConfirmedRuleWriter {
	if dir == "" {
		return nil
	}
	return &ConfirmedRuleWriter{dir: dir}
}

// confirmedRuleRecord is the on-disk JSON format. It wraps ConfirmedRuleSet
// with case metadata so historical records are self-describing.
type confirmedRuleRecord struct {
	CaseID      string                     `json:"case_id"`
	CaseType    string                     `json:"case_type"`
	TechField   string                     `json:"tech_field,omitempty"`
	ConfirmedAt string                     `json:"confirmed_at"`
	RuleSet     reasoning.ConfirmedRuleSet `json:"rule_set"`
}

// Write persists a confirmed rule set for the given case. The filename is
// derived from caseID + timestamp to avoid collisions across cases. Returns
// the path written.
func (w *ConfirmedRuleWriter) Write(caseID, caseType, techField string, rs reasoning.ConfirmedRuleSet) (string, error) {
	if w == nil || w.dir == "" {
		return "", fmt.Errorf("confirmed rule writer: no directory configured")
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return "", fmt.Errorf("confirmed rule writer: mkdir: %w", err)
	}

	rec := confirmedRuleRecord{
		CaseID:      caseID,
		CaseType:    caseType,
		TechField:   techField,
		ConfirmedAt: time.Now().UTC().Format(time.RFC3339),
		RuleSet:     rs,
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("confirmed rule writer: marshal: %w", err)
	}

	ts := time.Now().Unix()
	safeID := sanitizeFilename(caseID)
	filename := fmt.Sprintf("%s_%d.json", safeID, ts)
	path := filepath.Join(w.dir, filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("confirmed rule writer: write %s: %w", path, err)
	}
	return path, nil
}

// List returns all historical confirmed-rule record paths, oldest first.
// Useful for building "previously confirmed rule combinations" suggestions.
func (w *ConfirmedRuleWriter) List() ([]string, error) {
	if w == nil || w.dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("confirmed rule writer: list: %w", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			out = append(out, filepath.Join(w.dir, e.Name()))
		}
	}
	return out, nil
}

// Load reads one historical confirmed-rule record by path.
func (w *ConfirmedRuleWriter) Load(path string) (confirmedRuleRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return confirmedRuleRecord{}, fmt.Errorf("confirmed rule writer: load: %w", err)
	}
	var rec confirmedRuleRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("confirmed rule writer: unmarshal: %w", err)
	}
	return rec, nil
}

// sanitizeFilename replaces path separators and spaces in a case ID so it is
// safe to use as a filename component.
func sanitizeFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '/' || c == '\\' || c == ':' || c == ' ' {
			out = append(out, '_')
		} else {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "case"
	}
	return string(out)
}
