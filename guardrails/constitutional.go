package guardrails

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/agentcore/iface"
)

// =============================================================================
// ConstitutionalEngine — 宪法规则引擎
//
// 参考 BCIP 的 35+ 宪法规则系统 (codex-patent-constitutional crate)，
// 将 Mady 现有的 RulePipeline 升级为结构化的、YAML 驱动的规则引擎。
//
// 主要增强（相对现有 Pipeline）：
//  1. YAML 规则定义 — 规则从 Go 代码中解耦，通过配置管理
//  2. 8 种规则类型 — 关键词阻塞/模式分析/类别检测/说明书分析/章节结构/
//     结构分析/范围比较/权利要求清晰度
//  3. 自动扫描 — 在特定工具执行后自动检查输出
//  4. 合规上下文 — 规则可生成合规性上下文注入 Agent 提示词
// =============================================================================

// ---------------------------------------------------------------------------
// ConstitutionalRule — YAML 定义的宪法规则
// ---------------------------------------------------------------------------

// ConstitutionalRuleType 标识规则分析的类型。
type ConstitutionalRuleType string

const (
	RuleTypeKeywordBlocklist   ConstitutionalRuleType = "keyword_blocklist"
	RuleTypePatternAnalysis    ConstitutionalRuleType = "pattern_analysis"
	RuleTypeCategoryDetection  ConstitutionalRuleType = "category_detection"
	RuleTypeSpecification      ConstitutionalRuleType = "specification"
	RuleTypeSectionStructure   ConstitutionalRuleType = "section_structure"
	RuleTypeStructuralAnalysis ConstitutionalRuleType = "structural_analysis"
	RuleTypeScopeComparison    ConstitutionalRuleType = "scope_comparison"
	RuleTypeClaimClarity       ConstitutionalRuleType = "claim_clarity"
)

// ConstitutionalAction 映射到 pipeline.go 的 Action。
type ConstitutionalAction string

const (
	ConstitutionalBlock   ConstitutionalAction = "block"
	ConstitutionalWarn    ConstitutionalAction = "warn"
	ConstitutionalReview  ConstitutionalAction = "review"
	ConstitutionalEnforce ConstitutionalAction = "enforce"
	ConstitutionalLog     ConstitutionalAction = "log"
	ConstitutionalInfo    ConstitutionalAction = "info"
)

// ConstitutionalSeverity 映射到 pipeline.go 的 Severity。
type ConstitutionalSeverity string

const (
	ConstSeverityCritical ConstitutionalSeverity = "critical"
	ConstSeverityMajor    ConstitutionalSeverity = "major"
	ConstSeverityMinor    ConstitutionalSeverity = "minor"
)

// YAMLConstitutionalRule 是 YAML 定义的宪法规则结构。
type YAMLConstitutionalRule struct {
	Name        string                 `yaml:"name"`
	Type        ConstitutionalRuleType `yaml:"type"`
	Domain      []string               `yaml:"domain,omitempty"` // 适用的领域（空=全部）
	Phase       []string               `yaml:"phase,omitempty"`  // 适用的专利法阶段
	Severity    ConstitutionalSeverity `yaml:"severity"`
	Action      ConstitutionalAction   `yaml:"action"`
	Description string                 `yaml:"description"`
	Prompt      string                 `yaml:"prompt,omitempty"` // 合规上下文提示

	// 类型特定配置（YAML 内联，按 type 动态解析）
	Patterns       []string          `yaml:"patterns,omitempty"`        // PatternAnalysis
	Blocklist      []string          `yaml:"blocklist,omitempty"`       // KeywordBlocklist
	Categories     []string          `yaml:"categories,omitempty"`      // CategoryDetection
	SectionTitles  []string          `yaml:"section_titles,omitempty"`  // SectionStructure
	RequiredFields []string          `yaml:"required_fields,omitempty"` // Specification
	MinCount       int               `yaml:"min_count,omitempty"`       // 最小出现次数
	MaxCount       int               `yaml:"max_count,omitempty"`       // 最大出现次数
	StopWords      []string          `yaml:"stop_words,omitempty"`      // ScopeComparison
	CompareField   string            `yaml:"compare_field,omitempty"`   // ScopeComparison
	FieldRules     map[string]string `yaml:"field_rules,omitempty"`     // ClaimClarity
}

// YAMLConstitutionalConfig 是 YAML 配置文件的顶层结构。
type YAMLConstitutionalConfig struct {
	Rules []YAMLConstitutionalRule `yaml:"rules"`
}

// ---------------------------------------------------------------------------
// Ruleset — 编译后的规则集合（嵌入默认规则 + 加载用户规则）
// ---------------------------------------------------------------------------

//go:embed constitutional_default.yaml
var defaultConstitutionalYAML []byte

// CompiledRule 是编译后的可执行规则。
type CompiledRule struct {
	Config   YAMLConstitutionalRule
	Compiled []*regexp.Regexp // 编译后的模式（PatternAnalysis 适用）
	pipeline Rule
}

// Ruleset 管理一组宪法规则并提供批量检查。
type Ruleset struct {
	mu    sync.RWMutex
	rules []CompiledRule

	// pipeline 是适配后的标准 pipeline 链。
	pipeline *RulePipeline
}

// LoadDefaultRuleset 加载内嵌的默认宪法规则集。
func LoadDefaultRuleset() (*Ruleset, error) {
	return LoadConstitutionalYAML(defaultConstitutionalYAML)
}

// LoadConstitutionalYAML 从 YAML 数据加载宪法规则集。
func LoadConstitutionalYAML(data []byte) (*Ruleset, error) {
	var cfg YAMLConstitutionalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("guardrails: parse constitutional YAML: %w", err)
	}
	return compileRuleset(cfg)
}

// LoadConstitutionalFile 从文件路径加载宪法规则集。
func LoadConstitutionalFile(path string) (*Ruleset, error) {
	data, err := yamlFSReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("guardrails: read constitutional file %s: %w", path, err)
	}
	return LoadConstitutionalYAML(data)
}

// compileRuleset 将 YAML 规则编译为可执行规则。
func compileRuleset(cfg YAMLConstitutionalConfig) (*Ruleset, error) {
	rs := &Ruleset{
		rules:    make([]CompiledRule, 0, len(cfg.Rules)),
		pipeline: NewRulePipeline(),
	}
	for _, rc := range cfg.Rules {
		cr, err := compileRule(rc)
		if err != nil {
			slog.Warn("guardrails: skip constitutional rule", "name", rc.Name, "err", err)
			continue
		}
		rs.rules = append(rs.rules, cr)
		if cr.pipeline != nil {
			rs.pipeline.Add(cr.pipeline)
		}
	}
	slog.Info("guardrails: constitutional rules loaded",
		"total", len(cfg.Rules), "compiled", len(rs.rules))
	return rs, nil
}

// compileRule 将单个 YAML 规则编译为 CompiledRule。
func compileRule(rc YAMLConstitutionalRule) (CompiledRule, error) {
	cr := CompiledRule{Config: rc}

	// 编译正则模式（PatternAnalysis 类型）。
	if rc.Type == RuleTypePatternAnalysis {
		for _, p := range rc.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return cr, fmt.Errorf("compile pattern %q: %w", p, err)
			}
			cr.Compiled = append(cr.Compiled, re)
		}
	}

	// 编译为适配的 Rule 实现。
	rule, err := compileToPipelineRule(rc)
	if err != nil {
		return cr, err
	}
	cr.pipeline = rule
	return cr, nil
}

// compileToPipelineRule 将 YAML 规则适配到 Rule 接口。
func compileToPipelineRule(rc YAMLConstitutionalRule) (Rule, error) {
	name := rc.Name
	severity := mapConstitutionalSeverity(rc.Severity)
	action := mapConstitutionalAction(rc.Action)

	switch rc.Type {
	case RuleTypeKeywordBlocklist:
		return &keywordBlocklistRule{name: name, severity: severity, action: action, keywords: rc.Blocklist}, nil

	case RuleTypePatternAnalysis:
		compiled := make([]*regexp.Regexp, 0, len(rc.Patterns))
		for _, p := range rc.Patterns {
			if re, err := regexp.Compile(p); err == nil {
				compiled = append(compiled, re)
			}
		}
		return &patternAnalysisRule{name: name, severity: severity, action: action, compiled: compiled}, nil

	case RuleTypeCategoryDetection:
		return &categoryDetectionRule{name: name, severity: severity, action: action, categories: rc.Categories}, nil

	case RuleTypeSectionStructure:
		return &sectionStructureRule{name: name, severity: severity, action: action, titles: rc.SectionTitles, minCount: rc.MinCount}, nil

	case RuleTypeSpecification:
		return &specificationRule{name: name, severity: severity, action: action, required: rc.RequiredFields}, nil

	case RuleTypeStructuralAnalysis:
		return &structuralAnalysisRule{name: name, severity: severity, action: action, minCount: rc.MinCount, maxCount: rc.MaxCount}, nil

	case RuleTypeScopeComparison:
		return &scopeComparisonRule{name: name, severity: severity, action: action, stopWords: rc.StopWords}, nil

	case RuleTypeClaimClarity:
		return &claimClarityRule{name: name, severity: severity, action: action, fieldRules: rc.FieldRules}, nil

	default:
		return nil, fmt.Errorf("unknown rule type: %s", rc.Type)
	}
}

// ---------------------------------------------------------------------------
// 宪法规则 Rule 实现
// ---------------------------------------------------------------------------

// keywordBlocklistRule 禁止特定关键词出现。
type keywordBlocklistRule struct {
	name     string
	severity Severity
	action   Action
	keywords []string
}

func (r *keywordBlocklistRule) Name() string { return r.name }

func (r *keywordBlocklistRule) Check(content string, _ map[string]any) RuleResult {
	for _, kw := range r.keywords {
		if strings.Contains(content, kw) {
			return RuleResult{
				Passed:   false,
				Severity: r.severity,
				Action:   r.action,
				Message:  fmt.Sprintf("包含禁止关键词: %q", kw),
			}
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// patternAnalysisRule 用预编译正则模式检查内容。
type patternAnalysisRule struct {
	name     string
	severity Severity
	action   Action
	compiled []*regexp.Regexp
}

func (r *patternAnalysisRule) Name() string { return r.name }

func (r *patternAnalysisRule) Check(content string, _ map[string]any) RuleResult {
	for _, re := range r.compiled {
		if re.MatchString(content) {
			return RuleResult{
				Passed:   false,
				Severity: r.severity,
				Action:   r.action,
				Message:  fmt.Sprintf("模式匹配: %s", re.String()),
			}
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// categoryDetectionRule 检测内容是否包含特定类别。
type categoryDetectionRule struct {
	name       string
	severity   Severity
	action     Action
	categories []string
}

func (r *categoryDetectionRule) Name() string { return r.name }

func (r *categoryDetectionRule) Check(content string, _ map[string]any) RuleResult {
	// 类别检测：扫描内容中是否包含类别关键词。
	found := make([]string, 0)
	for _, cat := range r.categories {
		if strings.Contains(strings.ToLower(content), strings.ToLower(cat)) {
			found = append(found, cat)
		}
	}
	if len(found) > 0 {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("检测到类别: %s", strings.Join(found, ", ")),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// sectionStructureRule 检查必要章节的完整性。
type sectionStructureRule struct {
	name     string
	severity Severity
	action   Action
	titles   []string
	minCount int
}

func (r *sectionStructureRule) Name() string { return r.name }

func (r *sectionStructureRule) Check(content string, _ map[string]any) RuleResult {
	contentLower := strings.ToLower(content)
	var found int
	var missing []string
	for _, title := range r.titles {
		if strings.Contains(contentLower, strings.ToLower(title)) {
			found++
		} else {
			missing = append(missing, title)
		}
	}
	if found < r.minCount {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("缺少必要章节，已含 %d/%d 个，缺失: %s", found, r.minCount, strings.Join(missing, ", ")),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// specificationRule 检查说明书的必要字段。
type specificationRule struct {
	name     string
	severity Severity
	action   Action
	required []string
}

func (r *specificationRule) Name() string { return r.name }

func (r *specificationRule) Check(content string, _ map[string]any) RuleResult {
	contentLower := strings.ToLower(content)
	var missing []string
	for _, field := range r.required {
		if !strings.Contains(contentLower, strings.ToLower(field)) {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("缺少必要字段: %s", strings.Join(missing, ", ")),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// structuralAnalysisRule 基于结构特征分析内容。
type structuralAnalysisRule struct {
	name     string
	severity Severity
	action   Action
	minCount int
	maxCount int
}

func (r *structuralAnalysisRule) Name() string { return r.name }

func (r *structuralAnalysisRule) Check(content string, _ map[string]any) RuleResult {
	// 计算段落数作为结构复杂度指标。
	paras := strings.Split(strings.TrimSpace(content), "\n\n")
	paraCount := len(paras)

	if r.minCount > 0 && paraCount < r.minCount {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("段落数 %d 低于最低要求 %d", paraCount, r.minCount),
		}
	}
	if r.maxCount > 0 && paraCount > r.maxCount {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("段落数 %d 超过上限 %d", paraCount, r.maxCount),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// scopeComparisonRule 检查范围比较的合理性。
type scopeComparisonRule struct {
	name      string
	severity  Severity
	action    Action
	stopWords []string
}

func (r *scopeComparisonRule) Name() string { return r.name }

func (r *scopeComparisonRule) Check(content string, _ map[string]any) RuleResult {
	// 检查是否包含范围比较词汇但无具体范围描述。
	hasComparison := strings.Contains(content, "范围") ||
		strings.Contains(content, "超出") ||
		strings.Contains(content, "涵盖") ||
		strings.Contains(content, "落入")
	hasStop := false
	for _, sw := range r.stopWords {
		if strings.Contains(content, sw) {
			hasStop = true
			break
		}
	}
	if hasComparison && !hasStop {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  "涉及范围比较但缺少具体范围描述",
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// claimClarityRule 检查权利要求的清晰度。
type claimClarityRule struct {
	name       string
	severity   Severity
	action     Action
	fieldRules map[string]string
}

func (r *claimClarityRule) Name() string { return r.name }

func (r *claimClarityRule) Check(content string, _ map[string]any) RuleResult {
	var violations []string
	for field, expected := range r.fieldRules {
		if strings.Contains(content, field) {
			if !strings.Contains(content, expected) {
				violations = append(violations, fmt.Sprintf("%q 附近缺少 %q", field, expected))
			}
		}
	}
	if len(violations) > 0 {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  strings.Join(violations, "; "),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// ---------------------------------------------------------------------------
// 宪法引擎类型检查
// ---------------------------------------------------------------------------

// ProvideComplianceContext 生成合规上下文，供 Agent 系统提示注入。
// 该方法收集所有规则的 Prompt 字段，生成结构化的合规要求段落。
func (rs *Ruleset) ProvideComplianceContext(domains []string) string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	var b strings.Builder
	b.WriteString("## 合规要求\n")
	b.WriteString("以下合规规则必须在输出中严格遵守：\n\n")

	for _, cr := range rs.rules {
		// 域过滤。
		if len(cr.Config.Domain) > 0 && !domainMatchAny(cr.Config.Domain, domains) {
			continue
		}
		if cr.Config.Prompt != "" {
			fmt.Fprintf(&b, "- [%s] %s\n", cr.Config.Severity, cr.Config.Prompt)
		}
	}
	return b.String()
}

// domainMatchAny 检查是否至少有一个域匹配。
func domainMatchAny(allowed, actual []string) bool {
	for _, a := range actual {
		for _, al := range allowed {
			if strings.EqualFold(a, al) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// AutoScanHook — 工具输出自动扫描
// ---------------------------------------------------------------------------

// ConstitutionalHook 实现 iface.LifecycleHook，在 AfterModelCall 阶段
// 自动运行宪法规则扫描。嵌入 BaseLifecycleHook 确保所有生命周期方法
// 有默认空实现，仅重写 AfterModelCall。
//
// 对应 BCIP 的 auto-scan mode（工具执行后自动检查输出）。
type ConstitutionalHook struct {
	iface.BaseLifecycleHook
	Ruleset  *Ruleset
	ToolName string // 触发此钩子的工具名（空=全部）
}

// AfterModelCall 在模型输出后运行宪法规则检查。
// 实现 iface.LifecycleHook 接口标准签名。
func (h *ConstitutionalHook) AfterModelCall(ctx context.Context, arc *iface.AgentRunContext, mcc *iface.ModelCallContext) {
	_ = ctx
	_ = arc
	if h.Ruleset == nil || mcc == nil {
		return
	}
	metadata := make(map[string]any)
	if h.ToolName != "" {
		metadata["tool"] = h.ToolName
	}
	modified, _ := h.Ruleset.pipeline.Apply(mcc.Content, metadata)
	mcc.Content = modified
}

// ---------------------------------------------------------------------------
// 映射函数
// ---------------------------------------------------------------------------

// mapConstitutionalSeverity 将 ConstitutionalSeverity 映射到 pipeline.Severity。
func mapConstitutionalSeverity(s ConstitutionalSeverity) Severity {
	switch s {
	case ConstSeverityCritical:
		return SeverityError
	case ConstSeverityMajor:
		return SeverityWarning
	case ConstSeverityMinor:
		return SeverityInfo
	default:
		return SeverityWarning
	}
}

// mapConstitutionalAction 将 ConstitutionalAction 映射到 pipeline.Action。
func mapConstitutionalAction(a ConstitutionalAction) Action {
	switch a {
	case ConstitutionalBlock:
		return ActionBlock
	case ConstitutionalWarn:
		return ActionAlert
	case ConstitutionalReview:
		return ActionInject
	case ConstitutionalEnforce:
		return ActionBlock
	case ConstitutionalLog:
		return ActionLog
	case ConstitutionalInfo:
		return ActionLog
	default:
		return ActionAlert
	}
}

// Ensure interface compliance.
var (
	_ iface.LifecycleHook = (*ConstitutionalHook)(nil)
)

// ---------------------------------------------------------------------------
// 文件读取辅助（可被测试替换）
// ---------------------------------------------------------------------------

var yamlFSReadFile = func(path string) ([]byte, error) {
	return nil, fmt.Errorf("file reading not available: %s", path)
}

// SetYAMLFileReader 允许外部注入文件读取函数（用于测试或自定义加载）。
func SetYAMLFileReader(fn func(path string) ([]byte, error)) {
	yamlFSReadFile = fn
}

// Ensure interface compliance.
var (
	_ Rule = (*keywordBlocklistRule)(nil)
	_ Rule = (*patternAnalysisRule)(nil)
	_ Rule = (*categoryDetectionRule)(nil)
	_ Rule = (*sectionStructureRule)(nil)
	_ Rule = (*specificationRule)(nil)
	_ Rule = (*structuralAnalysisRule)(nil)
	_ Rule = (*scopeComparisonRule)(nil)
	_ Rule = (*claimClarityRule)(nil)
)
