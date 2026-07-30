package workflows

import (
	_ "embed"
	"fmt"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// 场景规则系统 (Scenario Rules)
//
// 参考 BCIP 的 场景规则 (codex-patent-agents/scenario.rs)，定义任务特定的
// 处理工作流，包含法律依据、提示词模板、处理步骤依赖和 HITL 标记。
//
// 6 个核心专利场景：
//   infringement_analysis    — 侵权分析
//   inventiveness_rejection  — 创造性驳回分析
//   novelty_analysis         — 新颖性分析
//   oa_strategy              — 审查意见策略
//   patent_drafting          — 专利撰写
//   quality_review           — 质量审查
// =============================================================================

//go:embed scenario_default.yaml
var defaultScenarioYAML []byte

// ScenarioRule 定义单个场景的处理规则。
type ScenarioRule struct {
	// Name 是场景标识（如 "novelty_analysis"）。
	Name string `yaml:"name" json:"name"`

	// Description 是场景的可读描述。
	Description string `yaml:"description" json:"description"`

	// Domain 是场景所属领域。
	Domain string `yaml:"domain" json:"domain"`

	// Phase 是场景对应的专利法阶段。
	Phase string `yaml:"phase" json:"phase"`

	// LegalBasis 是场景的法律依据引用。
	LegalBasis LegalBasis `yaml:"legal_basis,omitempty" json:"legal_basis,omitempty"`

	// SystemTemplate 是 Agent 系统提示词模板（支持 {{placeholder}}）。
	SystemTemplate string `yaml:"system_template" json:"system_template"`

	// UserTemplate 是用户提示词模板（支持 {{placeholder}}）。
	UserTemplate string `yaml:"user_template,omitempty" json:"user_template,omitempty"`

	// OutputFormat 指定输出格式要求。
	OutputFormat string `yaml:"output_format,omitempty" json:"output_format,omitempty"`

	// Steps 是处理步骤定义。
	Steps []ScenarioStep `yaml:"steps,omitempty" json:"steps,omitempty"`

	// HITL 标记此场景是否需要人工介入。
	HITL bool `yaml:"hitl,omitempty" json:"hitl,omitempty"`

	// SuggestedRoles 列出建议使用的 Agent 角色。
	SuggestedRoles []string `yaml:"suggested_roles,omitempty" json:"suggested_roles,omitempty"`
}

// LegalBasis 是场景的法律依据引用。
type LegalBasis struct {
	// Laws 列出引用的法律法规。
	Laws []string `yaml:"laws,omitempty" json:"laws,omitempty"`

	// Articles 列出引用的具体条款。
	Articles []string `yaml:"articles,omitempty" json:"articles,omitempty"`

	// ReferenceCases 列出参考案例。
	ReferenceCases []string `yaml:"reference_cases,omitempty" json:"reference_cases,omitempty"`
}

// ScenarioStep 是处理步骤中的一个阶段。
type ScenarioStep struct {
	// ID 是步骤标识。
	ID string `yaml:"id" json:"id"`

	// Name 是步骤名称。
	Name string `yaml:"name" json:"name"`

	// DependsOn 列出此步骤依赖的前序步骤 ID。
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`

	// Tool 指定此步骤使用的工具。
	Tool string `yaml:"tool,omitempty" json:"tool,omitempty"`

	// AgentRole 指定此步骤使用的 Agent 角色。
	AgentRole string `yaml:"agent_role,omitempty" json:"agent_role,omitempty"`

	// HITL 标记此步骤是否需要人工介入。
	HITL bool `yaml:"hitl,omitempty" json:"hitl,omitempty"`
}

// ScenarioResult 是场景执行的结果。
type ScenarioResult struct {
	// Scenario 是执行的场景规则。
	Scenario *ScenarioRule `json:"scenario"`

	// SystemPrompt 是渲染后的系统提示词。
	SystemPrompt string `json:"system_prompt"`

	// Output 是 Agent 输出。
	Output string `json:"output"`

	// CompletedSteps 记录已完成的步骤。
	CompletedSteps []string `json:"completed_steps"`

	// LegalReferences 记录使用的法律依据。
	LegalReferences []string `json:"legal_references,omitempty"`
}

// ---------------------------------------------------------------------------
// ScenarioEngine — 场景规则引擎
// ---------------------------------------------------------------------------

// ScenarioEngine 管理场景规则并提供场景匹配和提示词渲染。
type ScenarioEngine struct {
	rules map[string]*ScenarioRule
}

// NewScenarioEngine 加载默认场景规则。
func NewScenarioEngine() *ScenarioEngine {
	se := &ScenarioEngine{
		rules: make(map[string]*ScenarioRule),
	}
	if err := se.LoadYAML(defaultScenarioYAML); err != nil {
		slog.Warn("scenario: load default rules", "err", err)
	}
	return se
}

// LoadYAML 从 YAML 数据加载场景规则。
func (se *ScenarioEngine) LoadYAML(data []byte) error {
	var cfg struct {
		Scenarios []ScenarioRule `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("scenario: parse YAML: %w", err)
	}
	for i := range cfg.Scenarios {
		r := &cfg.Scenarios[i]
		if r.Name == "" {
			return fmt.Errorf("scenario: scenario %d has empty name", i)
		}
		se.rules[r.Name] = r
	}
	slog.Info("scenario: rules loaded", "count", len(cfg.Scenarios))
	return nil
}

// Get 按名称查找场景规则。
func (se *ScenarioEngine) Get(name string) (*ScenarioRule, bool) {
	r, ok := se.rules[name]
	return r, ok
}

// Match 尝试将用户查询匹配到最合适的场景规则。
// 匹配逻辑：搜索查询中是否包含场景的关键词特征。
// 返回匹配的场景名称和相关度分数。
func (se *ScenarioEngine) Match(query string) []struct {
	Name  string
	Score float64
} {
	type match struct {
		Name  string
		Score float64
	}
	var matches []match
	qlower := strings.ToLower(query)

	for name, rule := range se.rules {
		score := scoreScenarioMatch(qlower, name, rule)
		if score > 0 {
			matches = append(matches, match{Name: name, Score: score})
		}
	}
	// 按分数降序排序。
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Score > matches[i].Score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	if len(matches) > 3 {
		matches = matches[:3]
	}
	out := make([]struct {
		Name  string
		Score float64
	}, len(matches))
	for i, m := range matches {
		out[i] = struct {
			Name  string
			Score float64
		}{m.Name, m.Score}
	}
	return out
}

// scoreScenarioMatch 计算查询与场景的匹配分数。
func scoreScenarioMatch(query, name string, rule *ScenarioRule) float64 {
	score := 0.0

	// 场景名称精确匹配 → 最高分。
	if strings.Contains(query, name) {
		score += 1.0
	}

	// 场景描述关键词匹配。
	descWords := strings.Fields(strings.ToLower(rule.Description))
	for _, word := range descWords {
		if len(word) > 1 && strings.Contains(query, word) {
			score += 0.3
		}
	}

	// 法律依据关键词匹配。
	for _, law := range rule.LegalBasis.Laws {
		if strings.Contains(query, strings.ToLower(law)) {
			score += 0.5
		}
	}
	for _, art := range rule.LegalBasis.Articles {
		if strings.Contains(query, art) {
			score += 0.4
		}
	}

	return score
}

// RenderSystemPrompt 渲染场景的系统提示词，填充占位符。
func (se *ScenarioEngine) RenderSystemPrompt(name string, params map[string]string) (string, error) {
	rule, ok := se.rules[name]
	if !ok {
		return "", fmt.Errorf("scenario: unknown rule %q", name)
	}
	prompt := rule.SystemTemplate
	for k, v := range params {
		prompt = strings.ReplaceAll(prompt, "{{"+k+"}}", v)
	}
	// 附加法律依据。
	if len(rule.LegalBasis.Laws) > 0 || len(rule.LegalBasis.Articles) > 0 {
		var b strings.Builder
		b.WriteString("\n\n## 法律依据\n")
		for _, law := range rule.LegalBasis.Laws {
			fmt.Fprintf(&b, "- %s\n", law)
		}
		for _, art := range rule.LegalBasis.Articles {
			fmt.Fprintf(&b, "- %s\n", art)
		}
		prompt += b.String()
	}
	return prompt, nil
}

// RenderUserPrompt 渲染场景的用户提示词，填充占位符。
func (se *ScenarioEngine) RenderUserPrompt(name string, params map[string]string) (string, error) {
	rule, ok := se.rules[name]
	if !ok {
		return "", fmt.Errorf("scenario: unknown rule %q", name)
	}
	if rule.UserTemplate == "" {
		return "", nil
	}
	prompt := rule.UserTemplate
	for k, v := range params {
		prompt = strings.ReplaceAll(prompt, "{{"+k+"}}", v)
	}
	return prompt, nil
}

// ListScenarios 列出所有已注册的场景规则名称。
func (se *ScenarioEngine) ListScenarios() []string {
	var names []string
	for name := range se.rules {
		names = append(names, name)
	}
	// 排序。
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

// Ensure package-level imports are used.
var _ = fmt.Sprintf
