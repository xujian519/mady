package agentcore

import (
	"fmt"
	"regexp"
	"sync"
)

// AgentManifest 是 Agent 的声明式描述文件结构。
// 以 JSON 文件形式存放在 manifests/ 目录中，由 ScanManifests 扫描加载。
// 它是"声明式契约"——只描述 Agent 的身份和能力，不包含运行时逻辑。
// 运行时行为由 domains 包中的工厂函数（如 ChatAgentConfig）决定。
type AgentManifest struct {
	// Name 是 Agent 的唯一标识符，同时也是 manifest 文件所在目录名。
	// 必须匹配 [a-z0-9]+(-[a-z0-9]+)* 模式，最长 64 字符。
	Name string `json:"name"`

	// Domain 标识功能领域，决定使用哪个工厂函数构建 Agent。
	// 有效值：chat / assistant / patent / legal
	Domain string `json:"domain"`

	// Description 是该 Agent 职责范围的人类可读描述。
	// 会注入到 Router 的 SystemPrompt 以及 HandoffConfig.Description。
	Description string `json:"description,omitempty"`

	// GuardrailLevel 控制安全护栏严格等级。
	// 有效值：light / standard / strict；空值使用领域默认等级。
	GuardrailLevel string `json:"guardrail_level,omitempty"`

	// Tools 列出该 Agent 可用的工具名称（可选）。
	Tools []string `json:"tools,omitempty"`

	// HandoffTargets 列出此 Agent 可以委托的目标 Agent 名称（可选）。
	HandoffTargets []string `json:"handoff_targets,omitempty"`

	// KnowledgeDomain 指定知识检索领域（可选）。
	KnowledgeDomain string `json:"knowledge_domain,omitempty"`
}

var manifestNameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var (
	validDomainsMu sync.RWMutex
	validDomains   = map[string]bool{
		"chat":      true,
		"assistant": true,
		"patent":    true,
		"legal":     true,
	}

	validGuardrailLevelsMu sync.RWMutex
	validGuardrailLevels   = map[string]bool{
		"light":    true,
		"standard": true,
		"strict":   true,
	}
)

// RegisterValidDomain registers an additional valid manifest domain.
// Empty names are ignored. This allows extension packages to add new
// functional domains without modifying the core manifest definitions.
func RegisterValidDomain(name string) {
	if name == "" {
		return
	}
	validDomainsMu.Lock()
	defer validDomainsMu.Unlock()
	validDomains[name] = true
}

// RegisterValidGuardrailLevel registers an additional valid guardrail level
// name for manifest validation. Empty names are ignored.
func RegisterValidGuardrailLevel(name string) {
	if name == "" {
		return
	}
	validGuardrailLevelsMu.Lock()
	defer validGuardrailLevelsMu.Unlock()
	validGuardrailLevels[name] = true
}

// ValidateManifest 校验 AgentManifest 的字段合法性。
// 返回 nil 表示校验通过。
func ValidateManifest(m AgentManifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name 字段为必填")
	}
	if len(m.Name) > 64 {
		return fmt.Errorf("manifest: name %q 超过 64 字符上限", m.Name)
	}
	if !manifestNameRE.MatchString(m.Name) {
		return fmt.Errorf("manifest: name %q 必须匹配 [a-z0-9-]+ 模式", m.Name)
	}

	validDomainsMu.RLock()
	ok := validDomains[m.Domain]
	validDomainsMu.RUnlock()
	if !ok {
		return fmt.Errorf("manifest: %q 的 domain %q 无效（有效值：chat/assistant/patent/legal）",
			m.Name, m.Domain)
	}

	if m.GuardrailLevel != "" {
		validGuardrailLevelsMu.RLock()
		ok := validGuardrailLevels[m.GuardrailLevel]
		validGuardrailLevelsMu.RUnlock()
		if !ok {
			return fmt.Errorf("manifest: %q 的 guardrail_level %q 无效（有效值：light/standard/strict）",
				m.Name, m.GuardrailLevel)
		}
	}
	return nil
}
