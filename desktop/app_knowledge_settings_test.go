//go:build darwin

package main

import (
	"os"
	"testing"

	"github.com/xujian519/mady/bootstrap"
	"github.com/xujian519/mady/bootstrap/agentconfig"
)

// --- 知识库模型设置持久化 ---

func TestKnowledgeSettingsPersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := knowledgeSettingsPath(dir)

	want := KnowledgeModelSettings{
		BaseURL:       "http://127.0.0.1:8000/v1",
		APIKey:        "sk-test-1234",
		EmbedModel:    "bge-m3-mlx-8bit",
		RerankModel:   "Qwen3-Reranker-4B-4bit-MLX",
		RerankEnabled: true,
	}
	if err := saveJSONFile(path, want); err != nil {
		t.Fatalf("saveJSONFile: %v", err)
	}
	got := loadJSONFile[KnowledgeModelSettings](path)
	if got != want {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, want)
	}
	// 原子写不应残留 tmp 文件（saveJSONFile 用 CreateTemp 前缀 .mady-settings-*）
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "knowledge-settings.json" {
			t.Errorf("unexpected file %s in settings dir", e.Name())
		}
	}
}

func TestLoadKnowledgeModelSettings_MissingOrInvalid(t *testing.T) {
	dir := t.TempDir()

	// 文件不存在 → 零值，不视为错误
	if got := loadJSONFile[KnowledgeModelSettings](knowledgeSettingsPath(dir)); got != (KnowledgeModelSettings{}) {
		t.Errorf("missing file should yield zero value, got %+v", got)
	}

	// 非法 JSON → 零值，不视为错误
	bad := knowledgeSettingsPath(dir)
	if err := os.WriteFile(bad, []byte("{oops"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := loadJSONFile[KnowledgeModelSettings](bad); got != (KnowledgeModelSettings{}) {
		t.Errorf("invalid JSON should yield zero value, got %+v", got)
	}
}

// --- API Key 掩码语义 ---

func TestMaskAPIKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"short", "********"},
		{"sk-test-1234", "****1234"},
	}
	for _, c := range cases {
		if got := maskAPIKey(c.in); got != c.want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsMaskedKey(t *testing.T) {
	if !isMaskedKey("****1234") {
		t.Error("masked key should be detected")
	}
	if isMaskedKey("sk-test-1234") {
		t.Error("plain key should not be treated as masked")
	}
	if isMaskedKey("") {
		t.Error("empty key should not be treated as masked")
	}
}

// --- 校验规则 ---

func TestKnowledgeModelSettings_Validate(t *testing.T) {
	base := KnowledgeModelSettings{
		BaseURL:     "http://127.0.0.1:8000/v1",
		EmbedModel:  "bge-m3-mlx-8bit",
		RerankModel: "Qwen3-Reranker-4B-4bit-MLX",
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	if err := (KnowledgeModelSettings{}).Validate(); err == nil {
		t.Error("empty settings should be rejected (missing BaseURL)")
	}
	if err := (KnowledgeModelSettings{BaseURL: "x"}).Validate(); err == nil {
		t.Error("missing embed model should be rejected")
	}
	if err := (KnowledgeModelSettings{BaseURL: "x", EmbedModel: "m", RerankEnabled: true}).Validate(); err == nil {
		t.Error("rerank enabled without model should be rejected")
	}
}

// --- App 层：默认值合并 / 掩码保持原值 / 清空语义 ---

func TestGetKnowledgeModelSettings_Defaults(t *testing.T) {
	dir := t.TempDir()
	app := &App{fc: &bootstrap.Context{MadyHome: dir}}
	got := app.GetKnowledgeModelSettings()
	if got.BaseURL != agentconfig.DefaultOMLXBaseURL {
		t.Errorf("default BaseURL = %q, want %q", got.BaseURL, agentconfig.DefaultOMLXBaseURL)
	}
	if got.EmbedModel != agentconfig.DefaultEmbedModel {
		t.Errorf("default EmbedModel = %q, want %q", got.EmbedModel, agentconfig.DefaultEmbedModel)
	}
	if got.RerankModel != agentconfig.DefaultRerankModel {
		t.Errorf("default RerankModel = %q, want %q", got.RerankModel, agentconfig.DefaultRerankModel)
	}
}

func TestSetKnowledgeModelSettings_MaskedKeyKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	app := &App{fc: &bootstrap.Context{MadyHome: dir}}

	// 首次保存明文 key
	if err := app.SetKnowledgeModelSettings(KnowledgeModelSettings{
		BaseURL:     "http://127.0.0.1:8000/v1",
		APIKey:      "sk-secret-9999",
		EmbedModel:  "bge-m3-mlx-8bit",
		RerankModel: "Qwen3-Reranker-4B-4bit-MLX",
	}); err != nil {
		t.Fatalf("SetKnowledgeModelSettings: %v", err)
	}

	// 第二次用掩码 key → 保持原值，其他字段正常更新
	if err := app.SetKnowledgeModelSettings(KnowledgeModelSettings{
		BaseURL:     "http://127.0.0.1:9000/v1",
		APIKey:      maskAPIKey("sk-secret-9999"),
		EmbedModel:  "bge-m3-mlx-8bit",
		RerankModel: "Qwen3-Reranker-4B-4bit-MLX",
	}); err != nil {
		t.Fatalf("SetKnowledgeModelSettings(masked): %v", err)
	}
	got := loadJSONFile[KnowledgeModelSettings](knowledgeSettingsPath(dir))
	if got.BaseURL != "http://127.0.0.1:9000/v1" {
		t.Errorf("BaseURL not updated: %q", got.BaseURL)
	}
	if got.APIKey != "sk-secret-9999" {
		t.Errorf("masked set should keep original APIKey, got %q", got.APIKey)
	}

	// Get 返回掩码，不泄露明文
	if api := app.GetKnowledgeModelSettings().APIKey; api != "****9999" {
		t.Errorf("Get should return masked APIKey, got %q", api)
	}
}

func TestSetKnowledgeModelSettings_EmptyKeyClears(t *testing.T) {
	dir := t.TempDir()
	app := &App{fc: &bootstrap.Context{MadyHome: dir}}
	if err := app.SetKnowledgeModelSettings(KnowledgeModelSettings{
		BaseURL:    "http://127.0.0.1:8000/v1",
		APIKey:     "sk-secret-9999",
		EmbedModel: "bge-m3-mlx-8bit",
	}); err != nil {
		t.Fatal(err)
	}
	// 传空串 = 清空（禁用向量检索）
	if err := app.SetKnowledgeModelSettings(KnowledgeModelSettings{
		BaseURL:    "http://127.0.0.1:8000/v1",
		APIKey:     "",
		EmbedModel: "bge-m3-mlx-8bit",
	}); err != nil {
		t.Fatal(err)
	}
	got := loadJSONFile[KnowledgeModelSettings](knowledgeSettingsPath(dir))
	if got.APIKey != "" {
		t.Errorf("empty key should clear APIKey, got %q", got.APIKey)
	}
}

// --- 启动注入：保存的配置覆盖环境变量 ---

func TestGetOmlxServiceStatus_NoAPIKey(t *testing.T) {
	dir := t.TempDir()
	// 清空环境变量，避免本机真实 OMLX_API_KEY 触发网络健康检查（测试必须离线可跑）
	t.Setenv("OMLX_API_KEY", "")
	app := &App{fc: &bootstrap.Context{MadyHome: dir}}

	status := app.GetOmlxServiceStatus()
	if status.Running {
		t.Error("without API key, omlx service must not report running")
	}
	if status.Message == "" {
		t.Error("status message should not be empty")
	}
}

func TestApplyKnowledgeModelEnv(t *testing.T) {
	dir := t.TempDir()
	if err := saveJSONFile(knowledgeSettingsPath(dir), KnowledgeModelSettings{
		BaseURL:       "http://127.0.0.1:9000/v1",
		APIKey:        "sk-env-0001",
		EmbedModel:    "custom-embed",
		RerankModel:   "custom-rerank",
		RerankEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	app := &App{fc: &bootstrap.Context{MadyHome: dir}}

	// 先污染环境变量，验证注入会覆盖（t.Setenv 在测试结束后自动恢复）
	t.Setenv("OMLX_BASE_URL", "http://stale:1/v1")
	t.Setenv("OMLX_EMBED_MODEL", "stale-embed")
	t.Setenv("OMLX_RERANK_MODEL", "stale-rerank")
	t.Setenv("OMLX_API_KEY", "stale-key")
	t.Setenv("KNOWLEDGE_RERANK", "off")

	app.applyKnowledgeModelEnv()

	if got := os.Getenv("OMLX_BASE_URL"); got != "http://127.0.0.1:9000/v1" {
		t.Errorf("OMLX_BASE_URL = %q, want %q", got, "http://127.0.0.1:9000/v1")
	}
	if got := os.Getenv("OMLX_EMBED_MODEL"); got != "custom-embed" {
		t.Errorf("OMLX_EMBED_MODEL = %q, want custom-embed", got)
	}
	if got := os.Getenv("OMLX_RERANK_MODEL"); got != "custom-rerank" {
		t.Errorf("OMLX_RERANK_MODEL = %q, want custom-rerank", got)
	}
	if got := os.Getenv("OMLX_API_KEY"); got != "sk-env-0001" {
		t.Errorf("OMLX_API_KEY = %q, want sk-env-0001", got)
	}
	if got := os.Getenv("KNOWLEDGE_RERANK"); got != "on" {
		t.Errorf("KNOWLEDGE_RERANK = %q, want on", got)
	}
}
