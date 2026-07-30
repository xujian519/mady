package workflows

import (
	_ "embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// 工作流协作模板
//
// 参考 BCIP 的 6 个专利协作模板 (codex-patent-workflow/collaboration.rs)：
//   SearchAnalyzeDraft — Retriever → Analyzer → Writer
//   NoveltyCheck — Retriever → NoveltyChecker → Reviewer
//   CreativityCheck — Retriever → CreativityChecker → Reviewer
//   InfringementAnalysis — Retriever → InfringementChecker → Writer
//   InvalidityAnalysis — Retriever → InvalidityChecker → Writer
//   FullReview — Retriever → (NoveltyChecker || CreativityChecker) → Reviewer → QualityChecker
// =============================================================================

//go:embed templates.yaml
var defaultTemplatesYAML []byte

// TemplateRegistry 管理工作流模板的注册和查找。
type TemplateRegistry struct {
	templates []WorkflowTemplate
}

var defaultRegistry *TemplateRegistry

func init() {
	defaultRegistry = NewTemplateRegistry()
	if err := defaultRegistry.LoadYAML(defaultTemplatesYAML); err != nil {
		slog.Warn("workflows: load default templates", "err", err)
	}
	_ = registerBuiltinTemplates(defaultRegistry)
}

// NewTemplateRegistry 创建模板注册表。
func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{}
}

// Register 注册一个模板。
func (r *TemplateRegistry) Register(t WorkflowTemplate) {
	r.templates = append(r.templates, t)
}

// LoadYAML 从 YAML 加载模板。
func (r *TemplateRegistry) LoadYAML(data []byte) error {
	var cfg struct {
		Templates []yamlTemplate `yaml:"templates"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("workflows: parse template YAML: %w", err)
	}
	for _, yt := range cfg.Templates {
		r.templates = append(r.templates, yt.toWorkflowTemplate())
	}
	return nil
}

// Get 按名称查找模板。未找到时返回 false。
func (r *TemplateRegistry) Get(name string) (WorkflowTemplate, bool) {
	for _, t := range r.templates {
		if t.Name == name {
			return t, true
		}
	}
	return WorkflowTemplate{}, false
}

// List 返回所有已注册模板的名称列表。
func (r *TemplateRegistry) List() []string {
	names := make([]string, len(r.templates))
	for i, t := range r.templates {
		names[i] = t.Name
	}
	sort.Strings(names)
	return names
}

// yamlTemplate 是 YAML 格式的模板定义。
type yamlTemplate struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Domain      string        `yaml:"domain"`
	Steps       []yamlTplStep `yaml:"steps"`
}

type yamlTplStep struct {
	ID        string   `yaml:"id"`
	Type      string   `yaml:"type"`
	Role      string   `yaml:"role,omitempty"`
	Tool      string   `yaml:"tool,omitempty"`
	DependsOn []string `yaml:"depends_on,omitempty"`
}

func (yt *yamlTemplate) toWorkflowTemplate() WorkflowTemplate {
	t := WorkflowTemplate{
		Name:        yt.Name,
		Description: yt.Description,
		Domain:      yt.Domain,
		Steps:       make([]TemplateStep, len(yt.Steps)),
	}
	for i, ys := range yt.Steps {
		t.Steps[i] = TemplateStep{
			ID:   ys.ID,
			Type: StepType(ys.Type),
			Role: ys.Role,
			Tool: ys.Tool,
		}
		// 将字符串依赖解析为索引。
		for _, depID := range ys.DependsOn {
			for j, s := range yt.Steps {
				if s.ID == depID {
					t.Steps[i].DependsOn = append(t.Steps[i].DependsOn, j)
					break
				}
			}
		}
	}
	return t
}

// ---------------------------------------------------------------------------
// 内置 Go 模板（BCIP 6 模板的直接映射）
// ---------------------------------------------------------------------------

func registerBuiltinTemplates(r *TemplateRegistry) error {
	builtins := []WorkflowTemplate{
		// SearchAnalyzeDraft — 检索→分析→撰写
		{
			Name:        "search_analyze_draft",
			Description: "标准专利撰写流程：检索现有技术 → 分析技术特征 → 起草专利文档",
			Domain:      "patent",
			Steps: []TemplateStep{
				{ID: "search", Type: StepAgent, Role: "retriever"},
				{ID: "analyze", Type: StepAgent, Role: "analyzer", DependsOn: []int{0}},
				{ID: "draft", Type: StepAgent, Role: "writer", DependsOn: []int{1}},
			},
		},
		// NoveltyCheck — 新颖性审查
		{
			Name:        "novelty_check",
			Description: "新颖性审查：检索现有技术 → 逐特征新颖性分析 → 复核",
			Domain:      "patent",
			Steps: []TemplateStep{
				{ID: "search", Type: StepAgent, Role: "retriever"},
				{ID: "novelty", Type: StepAgent, Role: "novelty_checker", DependsOn: []int{0}},
				{ID: "review", Type: StepAgent, Role: "reviewer", DependsOn: []int{1}},
			},
		},
		// CreativityCheck — 创造性审查
		{
			Name:        "creativity_check",
			Description: "创造性审查：检索现有技术 → 三步法创造性判断 → 复核",
			Domain:      "patent",
			Steps: []TemplateStep{
				{ID: "search", Type: StepAgent, Role: "retriever"},
				{ID: "creativity", Type: StepAgent, Role: "creativity_checker", DependsOn: []int{0}},
				{ID: "review", Type: StepAgent, Role: "reviewer", DependsOn: []int{1}},
			},
		},
		// InfringementAnalysis — 侵权分析
		{
			Name:        "infringement_analysis",
			Description: "侵权分析：检索专利 → 全面覆盖原则分析 → 等同分析 → 报告",
			Domain:      "patent",
			Steps: []TemplateStep{
				{ID: "search", Type: StepAgent, Role: "retriever"},
				{ID: "analyze", Type: StepAgent, Role: "infringement_checker", DependsOn: []int{0}},
				{ID: "report", Type: StepAgent, Role: "writer", DependsOn: []int{1}},
			},
		},
		// InvalidityAnalysis — 无效宣告分析
		{
			Name:        "invalidity_analysis",
			Description: "无效宣告分析：检索证据 → 逐理由分析 → 无效宣告请求书",
			Domain:      "patent",
			Steps: []TemplateStep{
				{ID: "search", Type: StepAgent, Role: "retriever"},
				{ID: "analyze", Type: StepAgent, Role: "invalidity_checker", DependsOn: []int{0}},
				{ID: "draft", Type: StepAgent, Role: "writer", DependsOn: []int{1}},
			},
		},
		// FullReview — 全流程审查
		{
			Name:        "full_review",
			Description: "全流程质量审查：检索 → 新颖性+创造性并行审查 → 复核 → 质量评分",
			Domain:      "patent",
			Steps: []TemplateStep{
				{ID: "search", Type: StepAgent, Role: "retriever"},
				{ID: "novelty", Type: StepAgent, Role: "novelty_checker", DependsOn: []int{0}},
				{ID: "creativity", Type: StepAgent, Role: "creativity_checker", DependsOn: []int{0}},
				{ID: "review", Type: StepAgent, Role: "reviewer", DependsOn: []int{1, 2}},
				{ID: "quality", Type: StepAgent, Role: "quality_checker", DependsOn: []int{3}},
			},
		},
		// LegalAnalysis — 法律分析
		{
			Name:        "legal_analysis",
			Description: "法律分析：案件检索 → 法律适用分析 → 结论报告",
			Domain:      "legal",
			Steps: []TemplateStep{
				{ID: "research", Type: StepAgent, Role: "retriever"},
				{ID: "analyze", Type: StepAgent, Role: "analyzer", DependsOn: []int{0}},
				{ID: "conclude", Type: StepAgent, Role: "writer", DependsOn: []int{1}},
				{ID: "review", Type: StepQualityCheck, DependsOn: []int{2}},
			},
		},
	}
	for _, t := range builtins {
		r.Register(t)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 默认注册表
// ---------------------------------------------------------------------------

// DefaultRegistry 返回默认模板注册表（包含所有内置模板）。
func DefaultRegistry() *TemplateRegistry { return defaultRegistry }

// LookupTemplate 在默认注册表中查找模板。
func LookupTemplate(name string) (WorkflowTemplate, bool) {
	return defaultRegistry.Get(name)
}

// ListTemplates 列出默认注册表中的所有模板名称。
func ListTemplates() string {
	names := defaultRegistry.List()
	if len(names) == 0 {
		return "(无模板)"
	}
	return strings.Join(names, ", ")
}

// GetOrchestrationManifest converts a workflow name to an orchestration
// manifest key, bridging workflow templates to the existing
// domains.GetOrchestrationManifest() system.
func GetOrchestrationManifest(workflowName string) (string, bool) {
	mapping := map[string]string{
		"oa_response":           "oa_response",
		"re_examination":        "re_examination",
		"invalidation":          "invalidation",
		"patent_drafting":       "patent_drafting",
		"novelty_check":         "novelty_check",
		"full_review":           "full_review",
		"infringement_analysis": "infringement_analysis",
		"invalidity_analysis":   "invalidity_analysis",
	}
	key, ok := mapping[workflowName]
	return key, ok
}

// Ensure LoadYAML works on startup.
var _ = fmt.Sprintf
