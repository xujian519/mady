//go:build darwin

package main

// app_knowledge_settings.go — 知识库嵌入模型 / Rerank 模型设置(桌面端)。
//
// 嵌入/Rerank 模型在后端由环境变量控制(OMLX_BASE_URL / OMLX_API_KEY /
// OMLX_EMBED_MODEL / OMLX_RERANK_MODEL / KNOWLEDGE_RERANK),
// bootstrap.BuildEmbedder / BuildReranker 在应用启动时读取并装配到
// KnowledgeExtension。本文件提供:
//
//  1. 配置持久化(~/.mady/knowledge-settings.json,原子写);
//  2. 启动注入:applyKnowledgeModelEnv 在 initDeferred 调 LoadWikiStore
//     前写入环境变量,使 bootstrap 装配逻辑读到保存的配置(零改动 bootstrap);
//  3. 前端 binding:GetKnowledgeModelSettings / SetKnowledgeModelSettings /
//     GetOmlxServiceStatus。
//
// 语义:「保存后重启应用生效」(与 Q9 AI 设置一致);API Key 仅返回掩码,
// Set 时传回掩码表示保持原值,传空串表示清空(禁用向量检索)。
//
// 说明:切换嵌入模型不会重建预构建的 knowledge.db 向量索引(BGE-M3 1024 维)。
// 维度不匹配时 knowledge/sqlite.VectorSearch 返回 error、VectorIndex.Search
// 返回 nil,检索安全降级为 FTS-only,不崩溃。

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/pkg/omlx"
)

// KnowledgeModelSettings 是知识库嵌入/Rerank 模型设置。
// 持久化到 ~/.mady/knowledge-settings.json;缺省字段在读取时合并默认值。
type KnowledgeModelSettings struct {
	// BaseURL 是 OpenAI 兼容的本地推理服务地址(默认 http://127.0.0.1:8000/v1)。
	BaseURL string `json:"baseURL,omitempty"`
	// APIKey 是服务鉴权密钥。Get 返回掩码;Set 时传掩码表示保持原值。
	APIKey string `json:"apiKey,omitempty"`
	// EmbedModel 是嵌入模型名(默认 bge-m3-mlx-8bit)。
	EmbedModel string `json:"embedModel,omitempty"`
	// RerankModel 是 cross-encoder 重排模型名(默认 Qwen3-Reranker-4B-4bit-MLX)。
	RerankModel string `json:"rerankModel,omitempty"`
	// RerankEnabled 是否启用重排(对应 KNOWLEDGE_RERANK=on)。
	RerankEnabled bool `json:"rerankEnabled,omitempty"`
}

// OmlxServiceStatus 描述本地 oMLX 推理服务的运行状态(嵌入/Rerank 依赖)。
type OmlxServiceStatus struct {
	// Running 服务是否在运行(端口可达)。
	Running bool `json:"running"`
	// Installed oMLX 二进制是否已安装(brew install omlx)。
	Installed bool `json:"installed"`
	// Message 面向用户的中文状态说明。
	Message string `json:"message"`
}

// knowledgeSettingsPath 返回知识库模型设置文件路径。
func knowledgeSettingsPath(madyHome string) string {
	return filepath.Join(madyHome, "knowledge-settings.json")
}

// withKnowledgeDefaults 用内置默认值补齐未保存的字段。
// APIKey 无默认值(为空表示禁用向量检索)。
func withKnowledgeDefaults(s KnowledgeModelSettings) KnowledgeModelSettings {
	if s.BaseURL == "" {
		s.BaseURL = agentconfig.DefaultOMLXBaseURL
	}
	if s.EmbedModel == "" {
		s.EmbedModel = agentconfig.DefaultEmbedModel
	}
	if s.RerankModel == "" {
		s.RerankModel = agentconfig.DefaultRerankModel
	}
	return s
}

// maskAPIKey 打码 API Key:保留末尾 4 位,其余以星号替代。
// 空串或过短时返回全星号占位。掩码格式约定含 "****",Set 时据此识别「保持原值」。
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "********"
	}
	return "****" + key[len(key)-4:]
}

// isMaskedKey 判断 API Key 是否为掩码占位(表示「保持原值」)。
func isMaskedKey(key string) bool {
	return strings.Contains(key, "****")
}

// Validate 校验设置合法性。
func (s KnowledgeModelSettings) Validate() error {
	if strings.TrimSpace(s.BaseURL) == "" {
		return fmt.Errorf("知识库模型设置: 服务地址(BaseURL)不能为空")
	}
	if strings.TrimSpace(s.EmbedModel) == "" {
		return fmt.Errorf("知识库模型设置: 嵌入模型名不能为空")
	}
	if s.RerankEnabled && strings.TrimSpace(s.RerankModel) == "" {
		return fmt.Errorf("知识库模型设置: 启用 Rerank 时,重排模型名不能为空")
	}
	return nil
}

// GetKnowledgeModelSettings 返回保存的知识库模型设置(未保存时返回默认值)。
// APIKey 仅返回掩码,不泄露明文。
func (a *App) GetKnowledgeModelSettings() KnowledgeModelSettings {
	home := a.resolveMadyHome()
	if home == "" {
		return withKnowledgeDefaults(KnowledgeModelSettings{})
	}
	a.settingsMu.Lock()
	s := loadJSONFile[KnowledgeModelSettings](knowledgeSettingsPath(home))
	a.settingsMu.Unlock()
	s = withKnowledgeDefaults(s)
	s.APIKey = maskAPIKey(s.APIKey)
	return s
}

// SetKnowledgeModelSettings 保存知识库模型设置。
//
// 行为契约:
//  1. APIKey 传回掩码(含 "****")时保持原值;传空串表示清空(禁用向量检索);
//  2. 原子写入 ~/.mady/knowledge-settings.json(settingsMu 与 setCurrentProject
//     等 load-modify-save 路径互斥);
//  3. 保存后重启应用生效(嵌入/Rerank 组件在启动时装配)。
func (a *App) SetKnowledgeModelSettings(s KnowledgeModelSettings) error {
	home := a.resolveMadyHome()
	if home == "" {
		return fmt.Errorf("setKnowledgeModelSettings: MadyHome 不可用")
	}

	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()

	path := knowledgeSettingsPath(home)
	prev := loadJSONFile[KnowledgeModelSettings](path)
	if isMaskedKey(s.APIKey) {
		s.APIKey = prev.APIKey // 掩码 = 保持原值
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if err := saveJSONFile(path, s); err != nil {
		return fmt.Errorf("setKnowledgeModelSettings: 保存配置失败: %w", err)
	}
	log.Printf("[mady-desktop] knowledge model settings saved: base=%s embed=%s rerank=%s enabled=%v",
		s.BaseURL, s.EmbedModel, s.RerankModel, s.RerankEnabled)
	return nil
}

// applyKnowledgeModelEnv 将保存的知识库模型设置写入环境变量,
// 使 bootstrap.BuildEmbedder / BuildReranker 在启动装配时读到新值。
// 必须在 initDeferred 调用 LoadWikiStore 之前调用。
func (a *App) applyKnowledgeModelEnv() {
	home := a.resolveMadyHome()
	if home == "" {
		return
	}
	a.settingsMu.Lock()
	s := loadJSONFile[KnowledgeModelSettings](knowledgeSettingsPath(home))
	a.settingsMu.Unlock()

	if s.BaseURL != "" {
		_ = os.Setenv("OMLX_BASE_URL", s.BaseURL)
	}
	if s.APIKey != "" {
		_ = os.Setenv("OMLX_API_KEY", s.APIKey)
	}
	if s.EmbedModel != "" {
		_ = os.Setenv("OMLX_EMBED_MODEL", s.EmbedModel)
	}
	if s.RerankModel != "" {
		_ = os.Setenv("OMLX_RERANK_MODEL", s.RerankModel)
	}
	if s.RerankEnabled {
		_ = os.Setenv("KNOWLEDGE_RERANK", "on")
	} else {
		_ = os.Setenv("KNOWLEDGE_RERANK", "off")
	}
	log.Printf("[mady-desktop] knowledge model env applied: base=%s embed=%s rerank=%s enabled=%v",
		s.BaseURL, s.EmbedModel, s.RerankModel, s.RerankEnabled)
}

// resolveOMLXAPIKey 返回生效的 oMLX API Key:保存的配置优先,回退环境变量。
func (a *App) resolveOMLXAPIKey() string {
	if home := a.resolveMadyHome(); home != "" {
		a.settingsMu.Lock()
		s := loadJSONFile[KnowledgeModelSettings](knowledgeSettingsPath(home))
		a.settingsMu.Unlock()
		if s.APIKey != "" {
			return s.APIKey
		}
	}
	return os.Getenv("OMLX_API_KEY")
}

// GetOmlxServiceStatus 检测本地 oMLX 推理服务状态(嵌入/Rerank 依赖)。
// 复用 pkg/omlx.Manager 的健康检查;apiKey 为空时 IsRunning 恒为 false,
// 因此先解析生效的 API Key。
func (a *App) GetOmlxServiceStatus() OmlxServiceStatus {
	mgr := omlx.NewManager(8000, a.resolveOMLXAPIKey())
	if mgr.IsRunning() {
		return OmlxServiceStatus{
			Running:   true,
			Installed: true,
			Message:   "oMLX 服务运行中(http://127.0.0.1:8000)",
		}
	}
	if _, err := exec.LookPath("omlx"); err == nil {
		return OmlxServiceStatus{
			Running:   false,
			Installed: true,
			Message:   "oMLX 已安装但未运行,可运行 mady start-embeddings 启动",
		}
	}
	return OmlxServiceStatus{
		Running:   false,
		Installed: false,
		Message:   "oMLX 未安装(brew install omlx),向量检索将降级为关键词检索",
	}
}
