package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/pkg/util"
)

const ExtensionName = "rules"

// Engine is the query interface over loaded rule data.
type Engine struct {
	mu  sync.RWMutex
	set *RuleSet
}

// NewEngine creates an Engine from a loaded RuleSet.
func NewEngine(rs *RuleSet) *Engine {
	return &Engine{set: rs}
}

// LoadEngineFromMadyHome loads rules from $MADY_HOME/knowledge/rules/.
// Returns nil engine (no error) if the directory does not exist.
func LoadEngineFromMadyHome() (*Engine, error) {
	base, err := util.ResolveDataDir("knowledge")
	if err != nil {
		return nil, err
	}
	rs, err := LoadFromDir(base + "/rules")
	if err != nil {
		return nil, err
	}
	return NewEngine(rs), nil
}

// AllRules returns all loaded rules.
func (e *Engine) AllRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	out := make([]Rule, len(e.set.Rules))
	copy(out, e.set.Rules)
	return out
}

// RuleByID returns the rule with the given ID, or nil.
func (e *Engine) RuleByID(id string) *Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	return e.set.ruleIndex[id]
}

// RulesByDomain returns rules matching the given domain.
func (e *Engine) RulesByDomain(domain string) []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	out := make([]Rule, len(e.set.rulesByDomain[domain]))
	copy(out, e.set.rulesByDomain[domain])
	return out
}

// RulesBySeverity returns rules with the given severity.
func (e *Engine) RulesBySeverity(sev Severity) []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	out := make([]Rule, len(e.set.rulesBySeverity[sev]))
	copy(out, e.set.rulesBySeverity[sev])
	return out
}

// Article returns the article framework for the given article ID.
func (e *Engine) Article(id string) *ArticleFramework {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	return e.set.Articles[id]
}

// Orchestration returns the orchestration plan for the given case type.
func (e *Engine) Orchestration(caseType string) *Orchestration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	return e.set.Orchestrations[caseType]
}

// ReflectionIndicators returns the reflection indicators for a domain.
func (e *Engine) ReflectionIndicators(domain string) *ReflectionDomain {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil {
		return nil
	}
	return e.set.ReflectionDomains[domain]
}

// SearchRules performs a case-insensitive substring search across rule
// name, description, legalBasis, and domain.
func (e *Engine) SearchRules(keyword string) []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.set == nil || keyword == "" {
		return nil
	}
	kw := strings.ToLower(keyword)
	var out []Rule
	for _, r := range e.set.Rules {
		if strings.Contains(strings.ToLower(r.Name), kw) ||
			strings.Contains(strings.ToLower(r.Description), kw) ||
			strings.Contains(strings.ToLower(r.LegalBasis), kw) ||
			strings.Contains(strings.ToLower(r.Domain), kw) {
			out = append(out, r)
		}
	}
	return out
}

// ToRuleConstraints converts rules matching the given domain into
// reasoning.RuleConstraint objects for the reasoning framework.
func (e *Engine) ToRuleConstraints(domain string) []reasoning.RuleConstraint {
	rules := e.RulesByDomain(domain)
	out := make([]reasoning.RuleConstraint, 0, len(rules))
	for _, r := range rules {
		req := reasoning.ReqMust
		switch r.Severity {
		case SeverityCritical:
			req = reasoning.ReqMust
		case SeverityMajor:
			req = reasoning.ReqShould
		case SeverityMinor:
			req = reasoning.ReqNote
		}
		out = append(out, reasoning.RuleConstraint{
			ArticleID:        r.RuleID,
			ArticleName:      r.Name,
			Requirement:      req,
			Description:      r.Description,
			ApplicableStages: r.Check.Scope,
		})
	}
	return out
}

// --- Extension implementation ---

var (
	_ agentcore.Extension                = (*RulesExtension)(nil)
	_ agentcore.ToolProvider             = (*RulesExtension)(nil)
	_ agentcore.SystemPromptProvider     = (*RulesExtension)(nil)
	_ agentcore.TransformContextProvider = (*RulesExtension)(nil)
)

// RulesExtension adapts the rules Engine as an agentcore Extension.
type RulesExtension struct {
	engine *Engine
}

// NewExtension creates a rules extension from an Engine.
func NewExtension(engine *Engine) *RulesExtension {
	return &RulesExtension{engine: engine}
}

func (e *RulesExtension) Name() string                                     { return ExtensionName }
func (e *RulesExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *RulesExtension) Dispose() error                                   { return nil }

func (e *RulesExtension) SystemPromptSuffix() string {
	if e.engine == nil {
		return ""
	}
	rules := e.engine.AllRules()
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## 专利法律规则引擎（已加载 %d 条规则）\n", len(rules))
	b.WriteString("可用规则域: ")
	domains := make(map[string]bool)
	for _, r := range rules {
		domains[r.Domain] = true
	}
	first := true
	for d := range domains {
		if !first {
			b.WriteString(", ")
		}
		b.WriteString(d)
		first = false
	}
	b.WriteString("\n\n使用 search_rules 工具查询具体规则，使用 get_article_framework 获取法条判断框架。")
	return b.String()
}

func (e *RulesExtension) TransformContext(_ context.Context, msgs []agentcore.Message) []agentcore.Message {
	return msgs
}

func (e *RulesExtension) Tools() []*agentcore.Tool {
	if e.engine == nil {
		return nil
	}
	return []*agentcore.Tool{
		{
			Name:        "search_rules",
			Description: "搜索专利法律规则库，按关键词查询规则（新颖性、创造性、充分公开、权利要求等）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{
						"type":        "string",
						"description": "搜索关键词，如'新颖性'、'创造性'、'充分公开'",
					},
					"domain": map[string]any{
						"type":        "string",
						"description": "可选：按规则域过滤，如 patent_novelty、patent_inventiveness",
					},
				},
				"required": []string{"keyword"},
			},
			ReadOnly: true,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleSearch(args)
			},
		},
		{
			Name:        "get_article_framework",
			Description: "获取法条判断框架（步骤、输入输出、结论模式）。支持 A22.2(新颖性)、A22.3(创造性)、A26.3(充分公开)、A26.4(清楚支持)、A33(修改超范围)、A25(授权客体)、A67(全面覆盖)、equivalence(等同原则)。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"article_id": map[string]any{
						"type":        "string",
						"description": "法条ID，如 A22.2、A22.3、equivalence",
					},
				},
				"required": []string{"article_id"},
			},
			ReadOnly: true,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleArticle(args)
			},
		},
		{
			Name:        "get_orchestration",
			Description: "获取事务级编排方案（发现阶段、可用法条、执行模板）。支持 invalidation(无效宣告)、infringement(侵权分析)。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"case_type": map[string]any{
						"type":        "string",
						"description": "事务类型: invalidation 或 infringement",
					},
				},
				"required": []string{"case_type"},
			},
			ReadOnly: true,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleOrchestration(args)
			},
		},
		{
			Name:        "parse_office_action",
			Description: "解析审查意见通知书文本，提取驳回类型（新颖性/创造性/清楚/支持/充分公开/保护范围/形式）、引用的对比文献、受影响的权利要求编号。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "审查意见通知书的原文文本",
					},
				},
				"required": []string{"text"},
			},
			ReadOnly: true,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				var p struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return fmt.Sprintf("参数解析错误: %v", err), nil
				}
				oa := ParseOfficeAction(p.Text)
				return FormatOaSummary(oa), nil
			},
		},
		{
			Name:        "analyze_slop",
			Description: "反AI套话润色分析。三层检测：短语替换（填充词/空泛修饰/元叙述等）→结构缺陷（假三步法/假对比表/假转折等）→50分制评分+交付前快检。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "待分析的专利法律文书文本",
					},
				},
				"required": []string{"text"},
			},
			ReadOnly: true,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				var p struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return fmt.Sprintf("参数解析错误: %v", err), nil
				}
				a := AnalyzeSlop(p.Text)
				return FormatSlopAnalysis(a), nil
			},
		},
	}
}

func (e *RulesExtension) handleSearch(args json.RawMessage) (any, error) {
	var p struct {
		Keyword string `json:"keyword"`
		Domain  string `json:"domain"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Keyword == "" && p.Domain == "" {
		return "请提供搜索关键词或规则域", nil
	}
	var rules []Rule
	if p.Domain != "" {
		rules = e.engine.RulesByDomain(p.Domain)
	} else {
		rules = e.engine.SearchRules(p.Keyword)
	}
	if len(rules) == 0 {
		return "未找到匹配的规则", nil
	}
	return formatRules(rules), nil
}

func (e *RulesExtension) handleArticle(args json.RawMessage) (any, error) {
	var p struct {
		ArticleID string `json:"article_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	af := e.engine.Article(p.ArticleID)
	if af == nil {
		return fmt.Sprintf("未找到法条框架: %s", p.ArticleID), nil
	}
	return formatArticle(af), nil
}

func (e *RulesExtension) handleOrchestration(args json.RawMessage) (any, error) {
	var p struct {
		CaseType string `json:"case_type"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	orch := e.engine.Orchestration(p.CaseType)
	if orch == nil {
		return fmt.Sprintf("未找到事务编排: %s", p.CaseType), nil
	}
	return formatOrchestration(orch), nil
}

// --- Formatters ---

func formatRules(rules []Rule) string {
	var b strings.Builder
	for _, r := range rules {
		fmt.Fprintf(&b, "### %s — %s\n", r.RuleID, r.Name)
		fmt.Fprintf(&b, "- 描述: %s\n", r.Description)
		fmt.Fprintf(&b, "- 法律依据: %s\n", r.LegalBasis)
		fmt.Fprintf(&b, "- 域: %s\n", r.Domain)
		fmt.Fprintf(&b, "- 严重度: %s | 动作: %s\n", r.Severity, r.Action)
		fmt.Fprintf(&b, "- 检查类型: %s\n", r.Check.Type)
		if len(r.Check.Principles) > 0 {
			b.WriteString("- 原则:\n")
			for _, p := range r.Check.Principles {
				fmt.Fprintf(&b, "  - %s\n", p)
			}
		}
		if len(r.Check.Rules) > 0 {
			b.WriteString("- 规则:\n")
			for _, r2 := range r.Check.Rules {
				fmt.Fprintf(&b, "  - %s\n", r2)
			}
		}
		if len(r.Check.Assessment) > 0 {
			b.WriteString("- 评估:\n")
			for k, v := range r.Check.Assessment {
				fmt.Fprintf(&b, "  - %s → %s\n", k, v)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatArticle(af *ArticleFramework) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n", af.ArticleID, af.Name)
	fmt.Fprintf(&b, "法律依据: %s\n", af.LawRef)
	if af.GuidelineRef != "" {
		fmt.Fprintf(&b, "审查指南: %s\n", af.GuidelineRef)
	}
	b.WriteString("\n## 判断步骤\n")
	for _, step := range af.Steps {
		fmt.Fprintf(&b, "### 步骤%d: %s\n", step.Order, step.Name)
		fmt.Fprintf(&b, "规则参考: %s\n", step.RuleRef)
		fmt.Fprintf(&b, "输入提示: %s\n", step.InputHint)
		if len(step.OutputSchema) > 0 {
			b.WriteString("输出:\n")
			for k, v := range step.OutputSchema {
				fmt.Fprintf(&b, "  - %s: %s\n", k, v)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 结论模式\n")
	for k, v := range af.ConclusionSchema {
		fmt.Fprintf(&b, "- %s: %s\n", k, v)
	}
	return b.String()
}

func formatOrchestration(orch *Orchestration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n", orch.ID, orch.Name)
	fmt.Fprintf(&b, "事务类型: %s\n", orch.CaseType)
	fmt.Fprintf(&b, "描述: %s\n\n", orch.Description)
	b.WriteString("## 发现阶段\n")
	for i, stage := range orch.DiscoveryStages {
		fmt.Fprintf(&b, "### %d. %s\n", i+1, stage.Name)
		fmt.Fprintf(&b, "目标: %s\n", stage.Goal)
		if len(stage.Suggestions) > 0 {
			b.WriteString("建议:\n")
			for _, s := range stage.Suggestions {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "## 可用法条\n")
	for _, aa := range orch.AvailableArticles {
		fmt.Fprintf(&b, "%d. %s — %s\n", aa.Priority, aa.ArticleID, aa.Description)
	}
	fmt.Fprintf(&b, "\n## 执行模板\n")
	fmt.Fprintf(&b, "产出物: %s\n", orch.ExecutionTemplate.ArtifactType)
	fmt.Fprintf(&b, "章节:\n")
	for _, s := range orch.ExecutionTemplate.Sections {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	return b.String()
}
