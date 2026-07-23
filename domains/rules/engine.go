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

// =============================================================================
// Engine — 规则引擎查询接口
// =============================================================================

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

// SearchRules performs a case-insensitive substring search.
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

// ToRuleConstraints converts rules into reasoning constraints.
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

// =============================================================================
// RulesExtension — agentcore 扩展实现
// =============================================================================

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
	b.WriteString("\n\n使用 search_rules 查询具体规则，get_article_framework 获取法条判断框架，parse_office_action 解析审查意见，validate_amendment 验证修改是否超范围。")
	return b.String()
}

func (e *RulesExtension) TransformContext(_ context.Context, msgs []agentcore.Message) []agentcore.Message {
	return msgs
}

// Tools 提供 6 个工具：search_rules, get_article_framework, get_orchestration,
// parse_office_action, validate_amendment, analyze_slop。
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
			Description: "获取事务级编排方案（发现阶段、可用法条、执行模板）。支持 invalidation(无效宣告)、infringement(侵权分析)、oa_response(审查意见答复)、re-examination(复审)。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"case_type": map[string]any{
						"type":        "string",
						"description": "事务类型: invalidation / infringement / oa_response / re-examination",
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
			Name:        "validate_amendment",
			Description: "验证专利申请文件的修改是否符合专利法第33条的规定（修改不超范围）。三步检查：修改依据合法性→直接毫无疑义确定判断→修改时机合规性。支持主动修改、被动修改（OA答复）、依职权修改三种场景。提供16条详细规则和13个典型案例参考。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"original_claims": map[string]any{
						"type":        "string",
						"description": "原始权利要求书的内容",
					},
					"original_specification": map[string]any{
						"type":        "string",
						"description": "原始说明书的内容（包括附图描述）",
					},
					"amended_claims": map[string]any{
						"type":        "string",
						"description": "修改后的权利要求书内容",
					},
					"amended_specification": map[string]any{
						"type":        "string",
						"description": "修改后的说明书内容",
					},
					"modification_type": map[string]any{
						"type":        "string",
						"enum":        []string{"active", "passive", "ex_officio"},
						"description": "修改类型：active（主动）/ passive（被动，针对审查意见）/ ex_officio（依职权）",
					},
					"office_action_text": map[string]any{
						"type":        "string",
						"description": "审查意见通知书全文（被动修改时必填）",
					},
				},
				"required": []string{"original_claims", "amended_claims", "modification_type"},
			},
			ReadOnly: true,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return e.handleValidateAmendment(args)
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
