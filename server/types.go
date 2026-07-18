package server

import (
	"context"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
)

type threadStore interface {
	CreateThread(ctx context.Context) (*session.ThreadSnapshot, error)
	ListThreads(ctx context.Context) ([]session.Info, error)
	GetThread(ctx context.Context, key string) (*session.ThreadSnapshot, error)
	BranchThread(ctx context.Context, key, entryID string) (*session.ThreadSnapshot, error)
	GetThreadConfig(ctx context.Context, key string) (*agentcore.CallConfig, bool, error)
	SetThreadConfig(ctx context.Context, key string, cfg *agentcore.CallConfig) (*session.ThreadSnapshot, error)
	GetThreadThinking(ctx context.Context, key string) (*agentcore.ThinkingConfig, bool, error)
	SetThreadThinking(ctx context.Context, key string, cfg *agentcore.ThinkingConfig) (*session.ThreadSnapshot, error)
}

// BranchThreadRequest 是创建分支会话的请求体。
type BranchThreadRequest struct {
	EntryID string `json:"entry_id,omitempty"`
}

// ThreadThinkingRequest 是查询思考链的请求体。
type ThreadThinkingRequest struct {
	Thinking *agentcore.ThinkingConfig `json:"thinking,omitempty"`
}

// ThreadThinkingResponse 是思考链的响应体。
type ThreadThinkingResponse struct {
	ThreadID string                    `json:"thread_id"`
	Thinking *agentcore.ThinkingConfig `json:"thinking,omitempty"`
}

// ThreadConfigRequest 是更新会话配置的请求体。
type ThreadConfigRequest struct {
	Config *agentcore.CallConfig `json:"config,omitempty"`
}

// ThreadConfigResponse 是会话配置的响应体。
type ThreadConfigResponse struct {
	ThreadID string                `json:"thread_id"`
	Config   *agentcore.CallConfig `json:"config,omitempty"`
}

// SkillSummary 是技能的概要信息。
type SkillSummary struct {
	Name                   string            `json:"name"`
	Description            string            `json:"description"`
	FilePath               string            `json:"file_path"`
	BaseDir                string            `json:"base_dir"`
	DisableModelInvocation bool              `json:"disable_model_invocation,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	SelectedByDefault      bool              `json:"selected_by_default,omitempty"`
}

// SkillsResponse 是技能列表的响应体。
type SkillsResponse struct {
	Skills []SkillSummary `json:"skills"`
}

// SkillDiagnosticsResponse 是技能诊断信息的响应体。
type SkillDiagnosticsResponse struct {
	Diagnostics []skill.Diagnostic `json:"diagnostics"`
}

// SkillRegistryStatusResponse 是技能注册表状态的响应体。
type SkillRegistryStatusResponse struct {
	Skills                  []SkillSummary     `json:"skills"`
	ThreadID                string             `json:"thread_id,omitempty"`
	HasThreadConfig         bool               `json:"has_thread_config,omitempty"`
	SelectedSkills          []string           `json:"selected_skills,omitempty"`
	EffectiveSelectedSkills []string           `json:"effective_selected_skills,omitempty"`
	MissingSelectedSkills   []string           `json:"missing_selected_skills,omitempty"`
	AddedSkills             []string           `json:"added_skills,omitempty"`
	RemovedSkills           []string           `json:"removed_skills,omitempty"`
	UpdatedSkills           []string           `json:"updated_skills,omitempty"`
	AddedDiagnostics        []skill.Diagnostic `json:"added_diagnostics,omitempty"`
	RemovedDiagnostics      []skill.Diagnostic `json:"removed_diagnostics,omitempty"`
	SkillPaths              []string           `json:"skill_paths,omitempty"`
	Reloadable              bool               `json:"reloadable"`
	Diagnostics             []skill.Diagnostic `json:"diagnostics"`
	TotalSkills             int                `json:"total_skills"`
	VisibleSkills           int                `json:"visible_skills"`
	HiddenSkills            int                `json:"hidden_skills"`
	DiagnosticsCount        int                `json:"diagnostics_count"`
}

// ChatRequest 是聊天 API 的请求体。
type ChatRequest struct {
	Message        string                    `json:"message"`
	Stream         bool                      `json:"stream"`
	ThreadID       string                    `json:"thread_id,omitempty"`
	Model          string                    `json:"model,omitempty"`
	ResponseFormat *agentcore.ResponseFormat `json:"response_format,omitempty"`
	Thinking       *agentcore.ThinkingConfig `json:"thinking,omitempty"`
	Skills         []string                  `json:"skills,omitempty"`
}

// ChatResponse 是聊天 API 的响应体。
type ChatResponse struct {
	Output   string `json:"output"`
	ThreadID string `json:"thread_id,omitempty"`
	Error    string `json:"error,omitempty"`
}
