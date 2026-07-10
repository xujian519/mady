package agentcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateManifest_Valid(t *testing.T) {
	tests := []struct {
		name string
		m    AgentManifest
	}{
		{
			name: "chat manifest",
			m:    AgentManifest{Name: "chat", Domain: "chat", Description: "聊天"},
		},
		{
			name: "patent with all fields",
			m: AgentManifest{
				Name:            "patent",
				Domain:          "patent",
				Description:     "专利分析",
				GuardrailLevel:  "strict",
				Tools:           []string{"search", "analyze"},
				HandoffTargets:  []string{"chat", "assistant"},
				KnowledgeDomain: "patent",
			},
		},
		{
			name: "compound name with hyphens",
			m:    AgentManifest{Name: "legal-advisor", Domain: "legal"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateManifest(tt.m); err != nil {
				t.Errorf("ValidateManifest() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateManifest_Invalid(t *testing.T) {
	tests := []struct {
		name string
		m    AgentManifest
	}{
		{
			name: "empty name",
			m:    AgentManifest{Name: "", Domain: "chat"},
		},
		{
			name: "name too long",
			m:    AgentManifest{Name: mkLongString(65), Domain: "chat"},
		},
		{
			name: "name with uppercase",
			m:    AgentManifest{Name: "ChatAgent", Domain: "chat"},
		},
		{
			name: "name with underscore",
			m:    AgentManifest{Name: "chat_agent", Domain: "chat"},
		},
		{
			name: "name starting with hyphen",
			m:    AgentManifest{Name: "-chat", Domain: "chat"},
		},
		{
			name: "name consecutive hyphens",
			m:    AgentManifest{Name: "chat--agent", Domain: "chat"},
		},
		{
			name: "invalid domain",
			m:    AgentManifest{Name: "foo", Domain: "invalid"},
		},
		{
			name: "invalid guardrail level",
			m:    AgentManifest{Name: "chat", Domain: "chat", GuardrailLevel: "extreme"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateManifest(tt.m); err == nil {
				t.Error("ValidateManifest() expected error, got nil")
			}
		})
	}
}

func TestScanManifests_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	manifests, errs, err := ScanManifests(dir)
	if err != nil {
		t.Fatalf("ScanManifests() unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
}

func TestScanManifests_NonExistentDir(t *testing.T) {
	manifests, errs, err := ScanManifests("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanManifests() unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
}

func TestScanManifests_EmptyStringDir(t *testing.T) {
	manifests, errs, err := ScanManifests("")
	if err != nil {
		t.Fatalf("ScanManifests() unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
}

func TestScanManifests_FileNotDir(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.json")
	if err := os.WriteFile(filePath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := ScanManifests(filePath)
	if err == nil {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestScanManifests_ValidAndInvalidFiles(t *testing.T) {
	dir := t.TempDir()

	// Valid manifest
	writeManifest(t, dir, "chat.json", `{
		"name": "chat",
		"domain": "chat",
		"description": "日常聊天"
	}`)

	// Invalid manifest - missing domain
	writeManifest(t, dir, "bad.json", `{
		"name": "bad",
		"domain": ""
	}`)

	// Non-manifest JSON file (skipped - empty name)
	writeManifest(t, dir, "config.json", `{
		"name": "",
		"domain": ""
	}`)

	// Non-JSON file (should be skipped)
	writeManifest(t, dir, "readme.txt", "not a manifest")

	// Nested valid manifest
	nested := filepath.Join(dir, "subdir")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, nested, "assistant.json", `{
		"name": "assistant",
		"domain": "assistant",
		"guardrail_level": "standard"
	}`)

	manifests, errs, err := ScanManifests(dir)
	if err != nil {
		t.Fatalf("ScanManifests() unexpected error: %v", err)
	}

	// Should have 2 valid manifests (chat + assistant)
	if len(manifests) != 2 {
		t.Errorf("expected 2 manifests, got %d: %+v", len(manifests), manifests)
	}

	// Should have 2 load errors (bad.json: empty domain; config.json: empty name)
	if len(errs) != 2 {
		t.Errorf("expected 2 load errors, got %d: %+v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Path == "" {
			t.Error("load error should have a Path")
		}
	}
}

func TestScanManifests_DuplicateName(t *testing.T) {
	dir := t.TempDir()

	writeManifest(t, dir, "chat.json", `{
		"name": "chat",
		"domain": "chat"
	}`)

	// Same name, different file — both pass validation, caller deduplicates
	writeManifest(t, dir, "chat-dup.json", `{
		"name": "chat",
		"domain": "chat"
	}`)

	manifests, errs, err := ScanManifests(dir)
	if err != nil {
		t.Fatalf("ScanManifests() unexpected error: %v", err)
	}
	if len(manifests) != 2 {
		t.Errorf("expected 2 manifests (caller dedup), got %d", len(manifests))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
}

func TestScanManifests_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	// Malformed JSON
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifests, errs, err := ScanManifests(dir)
	if err != nil {
		t.Fatalf("ScanManifests() unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 load error, got %d", len(errs))
	}
}

// --- helpers ---

func writeManifest(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkLongString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
