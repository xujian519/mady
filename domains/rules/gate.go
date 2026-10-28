package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// 本文件实现规则驱动的两道运行时门禁：
//
//   - OutputGate（AfterToolExecution）：对工具结果文本跑确定性规则。warn 级在结果末尾
//     追加合规提示；block 级在结果前部标注「不可作为交付依据」——工具已经执行，
//     此处不能撤销执行，但必须让模型与用户看到结论不可采信。
//   - StructuralGate（BeforeToolExecution）：对交付工具的入参跑结构规则，block 级
//     命中直接拒绝该次调用，使不合格制品不会被渲染或落盘。
//
// 两道门禁只消费确定性检查（keyword_blocklist / structural_analysis 等），需要 LLM
// 判断的规则不参与——那类规则由 reasoning 引擎在生成阶段处理。

const gateMarker = "【规则门禁】"

// OutputGate 在工具执行后对结果文本追加合规提示或阻断标注。
type OutputGate struct {
	agentcore.BaseLifecycleHook

	engine       *Engine
	warnTitle    string
	blockMessage string
}

// NewOutputGate 构造输出门禁；engine 为 nil 时门禁不生效。
func NewOutputGate(engine *Engine) *OutputGate {
	return &OutputGate{
		engine:       engine,
		warnTitle:    "合规提示",
		blockMessage: "以下结果违反硬性规则，不可直接作为交付依据，须修正后重新生成。",
	}
}

var _ agentcore.LifecycleHook = (*OutputGate)(nil)

// AfterToolExecution 评估每个工具结果并按规则动作追加标注。
func (g *OutputGate) AfterToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) {
	if g == nil || g.engine == nil || tec == nil {
		return
	}
	for i := range tec.Results {
		result := &tec.Results[i]
		if result.Err != nil || strings.TrimSpace(result.Result) == "" {
			continue
		}
		if strings.Contains(result.Result, gateMarker) || strings.Contains(result.ForLLM, gateMarker) {
			continue
		}
		annotations := g.evaluate(result.Result)
		if annotations == "" {
			continue
		}
		if result.ForLLM == "" {
			result.ForLLM = result.Result
		}
		result.ForLLM += "\n\n" + annotations
	}
}

// evaluate 返回该文本的标注区块；无违规则返回空串。
func (g *OutputGate) evaluate(text string) string {
	results := g.engine.EvaluateAllRules(text)
	if len(results) == 0 {
		return ""
	}
	var blocks, warns []string
	for ruleID, result := range results {
		if result == nil || result.Passed {
			continue
		}
		rule := g.engine.RuleByID(ruleID)
		if rule == nil {
			continue
		}
		// 结构规则判「要素是否齐全」，对任意工具结果全文都会报缺失；
		// 它只能在交付制品的入参上判定（StructuralGate），不参与输出门禁。
		if rule.Check.Type == "structural_analysis" {
			continue
		}
		line := fmt.Sprintf("- %s（%s）：%s", rule.RuleID, rule.Name, summarize(result.Details))
		switch rule.Action {
		case ActionBlock, ActionReview:
			blocks = append(blocks, line)
		case ActionWarn:
			warns = append(warns, line)
		}
	}
	var builder strings.Builder
	if len(blocks) > 0 {
		fmt.Fprintf(&builder, "%s %s\n", gateMarker, g.blockMessage)
		for _, line := range blocks {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	if len(warns) > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		fmt.Fprintf(&builder, "%s %s：\n", gateMarker, g.warnTitle)
		for _, line := range warns {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}

// summarize 取前三条明细，避免把整篇检查结果塞进工具输出。
func summarize(details []string) string {
	if len(details) == 0 {
		return "未通过"
	}
	if len(details) > 3 {
		details = details[:3]
	}
	return strings.Join(details, "；")
}

// StructuralEntry 是一条制品结构门禁声明。
type StructuralEntry struct {
	// Tool 是被门禁的工具名。
	Tool string
	// TextArgs 是承载制品文本的入参名。
	TextArgs []string
	// RuleIDs 是参与判定的规则 id。
	RuleIDs []string
	// WhenArgs 是精确匹配才生效的入参（如 template=claims-spec）。
	WhenArgs map[string]string
}

// StructuralGate 在交付工具执行前对制品文本跑结构规则。
type StructuralGate struct {
	agentcore.BaseLifecycleHook

	engine  *Engine
	entries []StructuralEntry
}

// NewStructuralGate 构造结构门禁；没有任何声明时不生效。
func NewStructuralGate(engine *Engine, entries []StructuralEntry) *StructuralGate {
	return &StructuralGate{engine: engine, entries: entries}
}

var _ agentcore.LifecycleHook = (*StructuralGate)(nil)

// BeforeToolExecution 对命中的工具调用做结构判定，block 级未通过时拒绝整次调用。
func (g *StructuralGate) BeforeToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) error {
	if g == nil || g.engine == nil || tec == nil {
		return nil
	}
	for _, call := range tec.ToolCalls {
		for _, entry := range g.entries {
			if !strings.EqualFold(call.Name, entry.Tool) {
				continue
			}
			args, ok := parseArgsObject(call.Arguments)
			if !ok || !matchesWhen(args, entry.WhenArgs) {
				continue
			}
			text := collectTextArgs(args, entry.TextArgs)
			if strings.TrimSpace(text) == "" {
				continue
			}
			for _, ruleID := range entry.RuleIDs {
				rule := g.engine.RuleByID(ruleID)
				if rule == nil || rule.Action != ActionBlock {
					continue
				}
				result, err := rule.Check.Evaluate(text)
				if err != nil || result == nil || result.Passed {
					continue
				}
				return fmt.Errorf("规则 %s（%s）未通过，已拒绝 %s：%s",
					rule.RuleID, rule.Name, call.Name, summarize(result.Details))
			}
		}
	}
	return nil
}

// parseArgsObject 解析工具入参为对象。
func parseArgsObject(raw string) (map[string]any, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, false
	}
	return args, true
}

// matchesWhen 检查精确匹配条件。
func matchesWhen(args map[string]any, conditions map[string]string) bool {
	for key, want := range conditions {
		got, _ := args[key].(string)
		if got != want {
			return false
		}
	}
	return true
}

// collectTextArgs 收集入参中声明字段的全部字符串值。
func collectTextArgs(args map[string]any, names []string) string {
	var builder strings.Builder
	for _, name := range names {
		collectStrings(args[name], &builder)
	}
	return builder.String()
}

// collectStrings 递归收集字符串值。
func collectStrings(value any, builder *strings.Builder) {
	switch typed := value.(type) {
	case string:
		builder.WriteString(typed)
		builder.WriteString("\n")
	case []any:
		for _, item := range typed {
			collectStrings(item, builder)
		}
	case map[string]any:
		for _, item := range typed {
			collectStrings(item, builder)
		}
	}
}

// ValidateGateRules 校验规则集合能否被门禁消费。
//
// 批 A 的规则只允许确定性检查类型；声明了 LLM 类型却放进批 A，会让门禁静默跳过
// 该条规则（Evaluate 对未知类型默认放行），因此在这里显式报错。
func ValidateGateRules(engine *Engine, ruleIDs []string) error {
	if engine == nil {
		return fmt.Errorf("规则门禁：未提供规则引擎")
	}
	supported := map[string]bool{
		"presence": true, "exist": true, "must_include": true,
		"absence": true, "must_not": true, "forbidden": true,
		"numeric": true, "range": true, "numerical": true,
		"composition": true, "compound": true,
		"keyword_blocklist": true, "structural_analysis": true,
	}
	for _, id := range ruleIDs {
		rule := engine.RuleByID(id)
		if rule == nil {
			return fmt.Errorf("规则门禁：规则 %s 不在规则集中", id)
		}
		if !supported[rule.Check.Type] {
			return fmt.Errorf("规则门禁：规则 %s 的检查类型 %q 不支持确定性门禁", id, rule.Check.Type)
		}
	}
	return nil
}
