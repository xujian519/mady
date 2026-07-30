package agentconfig

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// RoleConfig — Agent 角色配置
//
// RoleConfig 定义了一个专业化 Agent 角色的参数：Temperature、Token 预算、
// 工具域范围、系统提示词等。借鉴 BCIP 的 9 角色 TOML 配置体系
// (codex-patent-agents), 但用 YAML 格式以保持与 Mady 现有配置系统一致。
//
// 使用场景：
//   - 角色级 Temperature 控制：Writer/Reviewer 用低 Temperature (0.3) 确保格式稳定，
//     Retriever 用高 Temperature (0.5) 增加搜索多样性。
//   - 工具域过滤：每个角色只暴露其工作所需的工具，减少 LLM 的工具选择干扰。
//   - Token 预算差异化：Search/Review 等简单角色用小预算，Drafting/Analysis 用大预算。
//
// 角色配置 YAML 示例 (roles.yaml)：
//
//	roles:
//	  retriever:
//	    description: "专利检索与分析员"
//	    temperature: 0.5
//	    max_tokens: 8192
//	    tool_domains:
//	      primary: [search, web_search]
//	      secondary: [document]
//	    system_prompt: "你是一位专利检索专家..."
//
//	  novelty_checker:
//	    description: "新颖性审查员"
//	    temperature: 0.4
//	    max_tokens: 4096
//	    tool_domains:
//	      primary: [search, analysis]
//	    system_prompt: "你负责新颖性判断..."
// =============================================================================

// RoleConfig 定义单个 Agent 角色的参数。
type RoleConfig struct {
	// ID 是角色唯一标识（如 retriever/novelty_checker），不影响运行时行为。
	// 仅在从 YAML 加载后用作 map key 的副本。
	ID string `json:"id,omitempty" yaml:"-"`

	// Description 是角色职责的可读描述。
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Temperature 覆盖 Agent 默认采样温度。0 表示使用 Agent 全局设置。
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`

	// MaxTokens 覆盖 Agent 默认最大输出 token 数。0 表示使用全局设置。
	MaxTokens int64 `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`

	// ToolDomains 声明角色使用的工具域范围。
	// 工具域通过 tools.ToolDomains 映射工具名称到域，实现角色级工具过滤。
	ToolDomains ToolDomainSet `json:"tool_domains,omitempty" yaml:"tool_domains,omitempty"`

	// SystemPrompt 追加到 Agent SystemPrompt 末尾（可选）。
	// 用于注入角色特定的工作方法和专业约束。
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
}

// ToolDomainSet 声明一个角色使用的工具域。
type ToolDomainSet struct {
	// Primary 是核心工具域 —— 角色主要依赖的工具类别。
	// 在严格过滤模式下，仅 Primary 域的工具对模型可见。
	Primary []string `json:"primary,omitempty" yaml:"primary,omitempty"`

	// Secondary 是辅助工具域 —— 角色偶尔使用但不必须的工具类别。
	// 在宽松过滤模式下，Secondary 域的工具也对模型可见。
	Secondary []string `json:"secondary,omitempty" yaml:"secondary,omitempty"`
}

// AllDomains 返回所有域（Primary + Secondary）的合并列表（去重）。
func (ts ToolDomainSet) AllDomains() []string {
	seen := make(map[string]bool, len(ts.Primary)+len(ts.Secondary))
	for _, d := range ts.Primary {
		seen[d] = true
	}
	for _, d := range ts.Secondary {
		seen[d] = true
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	return out
}

// HasDomain 检查域名是否在 Primary 或 Secondary 中。
func (ts ToolDomainSet) HasDomain(domain string) bool {
	if slices.Contains(ts.Primary, domain) {
		return true
	}
	return slices.Contains(ts.Secondary, domain)
}

// ---------------------------------------------------------------------------
// RoleSet — 角色集合
// ---------------------------------------------------------------------------

// RoleSet 是一组命名的角色配置，按角色 ID 索引。
type RoleSet map[string]RoleConfig

// NewRoleSet 从 YAML 文件加载角色集合。
// 文件格式：
//
//	roles:
//	  <role_id>:
//	    description: "..."
//	    temperature: 0.5
//	    ...
//
// 顶层也可以有无 roles 包裹的扁平形式（与 BCIP 一致），
// 但推荐使用 roles 命名空间以避免与全局配置冲突。
func NewRoleSet(path string) (RoleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agentconfig: read role file %s: %w", path, err)
	}
	return ParseRoleYAML(data)
}

// ParseRoleYAML 从 YAML 字节解析角色配置。
// 支持两种格式：
//
// 格式 A（推荐）:
//
//	roles:
//	  retriever:
//	    ...
//
// 格式 B（扁平）:
//
//	retriever:
//	  ...
func ParseRoleYAML(data []byte) (RoleSet, error) {
	// 尝试格式 A：检查是否存在顶层 "roles" 键。
	var wrapped struct {
		Roles map[string]RoleConfig `yaml:"roles"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err == nil && len(wrapped.Roles) > 0 {
		for id := range wrapped.Roles {
			r := wrapped.Roles[id]
			r.ID = id
			wrapped.Roles[id] = r
		}
		return RoleSet(wrapped.Roles), nil
	}

	// 尝试格式 B：扁平结构。
	var flat map[string]RoleConfig
	if err := yaml.Unmarshal(data, &flat); err != nil {
		return nil, fmt.Errorf("agentconfig: parse role YAML: %w", err)
	}
	for id := range flat {
		r := flat[id]
		r.ID = id
		flat[id] = r
	}
	return RoleSet(flat), nil
}

// Get 按 ID 获取角色配置及是否存在的标志。
func (rs RoleSet) Get(id string) (RoleConfig, bool) {
	if rs == nil {
		return RoleConfig{}, false
	}
	r, ok := rs[id]
	return r, ok
}

// IDs 返回所有已注册的角色 ID 列表（排序）。
func (rs RoleSet) IDs() []string {
	if rs == nil {
		return nil
	}
	out := make([]string, 0, len(rs))
	for id := range rs {
		out = append(out, id)
	}
	// 排序确保确定性输出。
	sort.Strings(out)
	return out
}

// Merge 将另一个 RoleSet 并入当前集合。相同 ID 的配置会被覆盖。
// 若 rs 为 nil，Merge 是空操作（不 panic）。
func (rs RoleSet) Merge(other RoleSet) {
	if rs == nil {
		return
	}
	maps.Copy(rs, other)
}

// ---------------------------------------------------------------------------
// RoleConfig 校验
// ---------------------------------------------------------------------------

// Validate 检查角色配置的字段合法性。
func (r RoleConfig) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("role: ID must not be empty")
	}
	if r.Temperature < 0 || r.Temperature > 2.0 {
		return fmt.Errorf("role %q: temperature %.2f out of range [0, 2.0]", r.ID, r.Temperature)
	}
	if r.MaxTokens < 0 {
		return fmt.Errorf("role %q: max_tokens %d must be non-negative", r.ID, r.MaxTokens)
	}
	for _, d := range r.ToolDomains.Primary {
		if strings.TrimSpace(d) == "" {
			return fmt.Errorf("role %q: primary domain must not be empty", r.ID)
		}
	}
	for _, d := range r.ToolDomains.Secondary {
		if strings.TrimSpace(d) == "" {
			return fmt.Errorf("role %q: secondary domain must not be empty", r.ID)
		}
	}
	return nil
}

// ValidateAll 校验角色集合中所有角色，返回首个错误。
func (rs RoleSet) ValidateAll() error {
	for id, r := range rs {
		r.ID = id // 确保 map key 同步
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}
