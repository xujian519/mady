package domainconfig

import (
	"fmt"

	"github.com/xujian519/mady/pkg/agentconfig"
)

// DomainConfig 是领域 Agent 的声明式配置。支持 YAML/JSON。
//
// 与 Manifest 的分工：Manifest 是轻量元数据（路由/描述）,
// DomainConfig 包含完整运行时配置（模型参数、工具列表、技能路径等）。
//
// 运行时配置共用 agentconfig.Config 结构定义，避免字段重复维护。
// YAML 中的 config: 键映射到该字段。
//
// 用法：
//
//	# patent.yaml
//	name: patent-agent
//	domain: patent
//	description: "专利代理与知识产权分析"
//	guardrail_level: strict
//	config:
//	  model: gpt-4o
//	  temperature: 0.1
//	  max_tokens: 4096
//	  tools:
//	    - patent_search
//	    - drafting
//	  skill_paths:
//	    - skills/patent
type DomainConfig struct {
	// 元数据
	Name        string `json:"name" yaml:"name"`
	Domain      string `json:"domain" yaml:"domain"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// 安全与知识域
	GuardrailLevel  string   `json:"guardrail_level,omitempty" yaml:"guardrail_level,omitempty"`
	KnowledgeDomain string   `json:"knowledge_domain,omitempty" yaml:"knowledge_domain,omitempty"`
	HandoffTargets  []string `json:"handoff_targets,omitempty" yaml:"handoff_targets,omitempty"`

	// 提示词文件路径（领域特有，区别于 Config.SystemPrompt）
	SystemPromptPath string `json:"system_prompt_path,omitempty" yaml:"system_prompt_path,omitempty"`

	// 附加配置（自由键值对，供扩展使用）
	Extra map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`

	// 运行时配置（共用 agentconfig.Config 定义）
	Config agentconfig.Config `json:"config,omitempty" yaml:"config,omitempty"`
}

// Validate 校验配置合法性。
func (c *DomainConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("domainconfig: name is required")
	}
	if c.Domain == "" {
		return fmt.Errorf("domainconfig: domain is required for %q", c.Name)
	}
	return nil
}
