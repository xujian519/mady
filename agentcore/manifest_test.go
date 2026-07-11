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

// --- LoadManifests: embed + 外部覆盖合并 ---

func TestLoadManifests_EmbeddedOnly(t *testing.T) {
	// userDir 为空 → 仅返回内置 4 个
	res := LoadManifests("")
	if len(res.Manifests) < 4 {
		t.Errorf("expected at least 4 embedded manifests, got %d", len(res.Manifests))
	}
	if res.EmbeddedCount < 4 {
		t.Errorf("EmbeddedCount=%d, want >=4", res.EmbeddedCount)
	}
	if res.ExternalCount != 0 {
		t.Errorf("ExternalCount=%d, want 0", res.ExternalCount)
	}
	if len(res.Overridden) != 0 || len(res.Added) != 0 {
		t.Errorf("Overridden/Added should be empty, got %v / %v", res.Overridden, res.Added)
	}
	// 验证内置的 4 个已知领域都在
	names := manifestNames(res.Manifests)
	for _, want := range []string{"chat-agent", "assistant-agent", "patent-agent", "legal-advisor"} {
		if _, ok := names[want]; !ok {
			t.Errorf("missing embedded manifest %q in %v", want, res.Manifests)
		}
	}
}

func TestLoadManifests_ExternalOverride(t *testing.T) {
	dir := t.TempDir()
	// 覆盖内置 chat-agent（改 description）
	writeManifest(t, dir, "chat.json", `{
		"name": "chat-agent",
		"domain": "chat",
		"description": "外部覆盖版本"
	}`)

	res := LoadManifests(dir)
	if len(res.Overridden) != 1 || res.Overridden[0] != "chat-agent" {
		t.Errorf("Overridden=%v, want [chat-agent]", res.Overridden)
	}
	// 找到 chat-agent，确认是外部版本
	for _, m := range res.Manifests {
		if m.Name == "chat-agent" {
			if m.Description != "外部覆盖版本" {
				t.Errorf("override not applied: description=%q", m.Description)
			}
		}
	}
	// 总数应仍为 4（覆盖不增加）
	if len(res.Manifests) != 4 {
		t.Errorf("expected 4 manifests (override), got %d", len(res.Manifests))
	}
	if res.ExternalCount != 1 {
		t.Errorf("ExternalCount=%d, want 1", res.ExternalCount)
	}
}

func TestLoadManifests_ExternalAddNew(t *testing.T) {
	dir := t.TempDir()
	// 新增一个内置不存在的领域
	writeManifest(t, dir, "finance.json", `{
		"name": "finance-agent",
		"domain": "assistant",
		"description": "财务领域自定义 agent"
	}`)

	res := LoadManifests(dir)
	if len(res.Added) != 1 || res.Added[0] != "finance-agent" {
		t.Errorf("Added=%v, want [finance-agent]", res.Added)
	}
	// 总数应为 5（4 内置 + 1 新增）
	if len(res.Manifests) != 5 {
		t.Errorf("expected 5 manifests (4 embedded + 1 new), got %d", len(res.Manifests))
	}
}

func TestLoadManifests_NonExistentUserDir(t *testing.T) {
	// userDir 不存在 → 静默回退到纯内置
	res := LoadManifests("/nonexistent/path/xyz")
	if len(res.Manifests) < 4 {
		t.Errorf("expected >=4 embedded manifests, got %d", len(res.Manifests))
	}
	if res.ExternalCount != 0 {
		t.Errorf("ExternalCount=%d, want 0", res.ExternalCount)
	}
}

func TestLoadManifests_ExternalBrokenJSON(t *testing.T) {
	// 外部目录有损坏 JSON → 记入 Errors 但不中断，内置 manifest 仍可用
	dir := t.TempDir()
	writeManifest(t, dir, "broken.json", `{invalid json}`)

	res := LoadManifests(dir)
	// 内置 4 个应仍在
	if len(res.Manifests) < 4 {
		t.Errorf("embedded manifests lost: got %d, want >=4", len(res.Manifests))
	}
	// 应有至少 1 个非致命错误
	if len(res.Errors) == 0 {
		t.Error("expected non-fatal errors for broken JSON, got 0")
	}
}

func TestLoadManifests_RootManifestsNotDrifted(t *testing.T) {
	// 防漂移：根目录 manifests/（用户参考示例）应与 agentcore/manifests/（embed 源）一致。
	// 测试运行时 cwd 为 agentcore/ 包目录，根目录 manifests/ 在 ../manifests/。
	rootDir := filepath.Join("..", "manifests")
	embeddedDir := "manifests"

	rootEntries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Skipf("根目录 manifests/ 不可读（%v），跳过漂移检查", err)
		return
	}
	for _, entry := range rootEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		rootData, err := os.ReadFile(filepath.Join(rootDir, entry.Name()))
		if err != nil {
			t.Errorf("读取根 manifest %s 失败: %v", entry.Name(), err)
			continue
		}
		embeddedData, err := embeddedManifestsFS.ReadFile(filepath.Join(embeddedDir, entry.Name()))
		if err != nil {
			t.Errorf("读取 embed manifest %s 失败: %v", entry.Name(), err)
			continue
		}
		if string(rootData) != string(embeddedData) {
			t.Errorf("manifest 漂移: %s\n根目录(用户示例)与 agentcore/manifests/(embed 源)内容不一致。\n"+
				"请同步两份文件，或删除根目录那份。", entry.Name())
		}
	}
}

func manifestNames(ms []AgentManifest) map[string]struct{} {
	out := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		out[m.Name] = struct{}{}
	}
	return out
}

func mkLongString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
