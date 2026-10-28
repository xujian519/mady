package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func loadGateEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewBundledEngine()
	if err != nil {
		t.Fatalf("加载随包门禁规则: %v", err)
	}
	return engine
}

func TestBatchARulesLoaded(t *testing.T) {
	engine := loadGateEngine(t)
	for _, id := range []string{"PAT-RISK-001", "PAT-APPROVAL-001", "PAT-ABS-001", "CON-COMP-0106"} {
		if engine.RuleByID(id) == nil {
			t.Fatalf("规则 %s 未加载", id)
		}
	}
	if err := ValidateGateRules(engine, []string{"PAT-RISK-001", "CON-COMP-0106"}); err != nil {
		t.Fatalf("ValidateGateRules: %v", err)
	}
}

// 随包门禁规则全部可被门禁消费：数量下限 + 类型白名单校验。
func TestBundledGateRulesAllConsumable(t *testing.T) {
	engine := loadGateEngine(t)
	rules := engine.AllRules()
	if len(rules) < 100 {
		t.Fatalf("随包门禁规则只有 %d 条，批 A + 批 B 应覆盖 block、结构性规则与 warn 提示", len(rules))
	}
	ids := make([]string, 0, len(rules))
	blocks := 0
	for _, rule := range rules {
		ids = append(ids, rule.RuleID)
		if rule.Action == ActionBlock {
			blocks++
		}
	}
	if blocks < 30 {
		t.Fatalf("block 级规则只有 %d 条", blocks)
	}
	if err := ValidateGateRules(engine, ids); err != nil {
		t.Fatalf("存在不可被门禁消费的规则：%v", err)
	}
}

// 结构要素的 pattern 按正则解释：CON-103 要求引用专利法第二十二条。
// 批 B 规则已加载且都是只提示的级别：没有 block。
func TestBatchBWarnRules(t *testing.T) {
	engine := loadGateEngine(t)
	for _, id := range []string{
		"LEGAL-DISC-001", "LEGAL-LIMIT-001", "LEGAL-JURIS-001", "LEGAL-ABS-001",
		"CON-COMP-0103", "CON-COMP-0105", "EX-SRC-001", "H-CLA-002", "IPC-GEN-INV-002", "EX-INV-007",
	} {
		rule := engine.RuleByID(id)
		if rule == nil {
			t.Fatalf("批 B 规则 %s 未加载", id)
		}
		if rule.Action == ActionBlock {
			t.Fatalf("批 B 规则 %s 不应是 block 级", id)
		}
	}
}

// 批 B 的 warn 规则不阻断交付调用（渐进式门禁：先提示，后收紧）。
func TestBatchBDoesNotBlock(t *testing.T) {
	engine := loadGateEngine(t)
	gate := NewStructuralGate(engine, []StructuralEntry{{
		Tool:     "render_patent_document",
		TextArgs: []string{"draft"},
		RuleIDs:  []string{"LEGAL-DISC-001"},
	}})
	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{{
			Name:      "render_patent_document",
			Arguments: `{"draft":"本意见具有法律效力。"}`,
		}},
	}
	if err := gate.BeforeToolExecution(context.Background(), nil, tec); err != nil {
		t.Fatalf("warn 级规则不应阻断：%v", err)
	}
}

// 同一条文本命中过多 warn 规则时，标注只列前 5 条并说明剩余数量。
func TestOutputGateCapsWarnLines(t *testing.T) {
	var rules []Rule
	for i, word := range []string{"甲词", "乙词", "丙词", "丁词", "戊词", "己词", "庚词"} {
		rules = append(rules, Rule{
			RuleID: "T-WARN-" + string(rune('A'+i)),
			Action: ActionWarn,
			Check: Check{
				Type:  "keyword_blocklist",
				Extra: map[string]any{"keywords": []any{word}},
			},
		})
	}
	set := &RuleSet{Rules: rules, ruleIndex: make(map[string]*Rule, len(rules))}
	for i := range rules {
		set.ruleIndex[rules[i].RuleID] = &rules[i]
	}
	gate := NewOutputGate(NewEngine(set))
	tec := &agentcore.ToolExecutionContext{
		Results: []agentcore.ToolResult{{ToolName: "t", Result: "甲词 乙词 丙词 丁词 戊词 己词 庚词"}},
	}
	gate.AfterToolExecution(context.Background(), nil, tec)
	output := tec.Results[0].ForLLM
	if !strings.Contains(output, "另有 2 条提示未列出") {
		t.Fatalf("应截断并说明剩余数量：%q", output)
	}
	if strings.Count(output, "T-WARN-") != maxWarnRules {
		t.Fatalf("应只列 %d 条，实际 %d 条：%q", maxWarnRules, strings.Count(output, "T-WARN-"), output)
	}
}

// 批 B 的误报抽检：锁定「不该命中」的典型文本，并如实记录已知会命中的泛称。
//
// 2026-10-09 的教训是：warn 级禁词表可能被完成摘要这类旁路文本触发。这里把
// 交付件正文与摘要两类文本都过一遍，命中情况作为可复核记录。
func TestWarnRulesFalsePositiveSamples(t *testing.T) {
	engine := loadGateEngine(t)

	shouldPass := []struct{ id, text, why string }{
		{"CON-COMP-0103", "根据《专利法》第22条第3款，本申请具备创造性。", "精确引用不应触发模糊引用提示"},
		{"LEGAL-DISC-001", "本意见不构成正式法律意见，需经代理人确认。", "否定语境不应触发免责提示"},
		{"LEGAL-JURIS-001", "「无管辖权」是被告的抗辩主张，法院尚未认定。", "引文豁免"},
	}
	for _, tc := range shouldPass {
		rule := engine.RuleByID(tc.id)
		if rule == nil {
			t.Fatalf("规则 %s 未加载", tc.id)
		}
		result, err := rule.Check.Evaluate(tc.text)
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if !result.Passed {
			t.Errorf("%s 误报：%s（%q）", tc.id, tc.why, tc.text)
		}
	}

	// 已知命中（warn 级可接受，但需要在部署后观察是否刷屏）。
	knownHits := []struct{ id, text string }{
		{"CON-COMP-0103", "根据法律规定，申请人应当提交请求书。"},
		{"IPC-GEN-INV-002", "判断是否存在事后诸葛亮的问题，应回到申请日。"},
	}
	for _, tc := range knownHits {
		rule := engine.RuleByID(tc.id)
		if rule == nil {
			t.Fatalf("规则 %s 未加载", tc.id)
		}
		result, _ := rule.Check.Evaluate(tc.text)
		t.Logf("已知命中 %s：%q -> passed=%v", tc.id, tc.text, result.Passed)
	}
}

func TestStructuralPatternRegex(t *testing.T) {
	engine := loadGateEngine(t)
	rule := engine.RuleByID("CON-103")
	if rule == nil {
		t.Fatal("缺少 CON-103")
	}
	cases := []struct {
		text string
		want bool
	}{
		{"本申请具备新颖性，依据专利法第二十二条。", true},
		{"本申请具备新颖性，依据专利法第22条。", true},
		{"本申请具备新颖性。", false},
	}
	for _, tc := range cases {
		result, err := rule.Check.Evaluate(tc.text)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if result.Passed != tc.want {
			t.Fatalf("%q passed=%v, want %v（%v）", tc.text, result.Passed, tc.want, result.Details)
		}
	}
}

// 关键词支持 "a|b" 交替组（与 dsh 的规则资产一致）。
func TestKeywordBlocklistAlternation(t *testing.T) {
	rule := Rule{
		RuleID: "T-ALT",
		Action: ActionWarn,
		Check: Check{
			Type:  "keyword_blocklist",
			Extra: map[string]any{"keywords": []any{"绝对可以|百分百"}},
		},
	}
	for _, text := range []string{"本方案绝对可以实施。", "百分百成功。"} {
		result, err := rule.Check.Evaluate(text)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if result.Passed {
			t.Fatalf("%q 应命中交替组", text)
		}
	}
	if result, _ := rule.Check.Evaluate("本方案通常可行。"); !result.Passed {
		t.Fatal("无命中时应通过")
	}
}

func TestKeywordBlocklist(t *testing.T) {
	engine := loadGateEngine(t)
	risk := engine.RuleByID("PAT-RISK-001")
	if risk == nil {
		t.Fatal("缺少 PAT-RISK-001")
	}
	cases := []struct {
		name string
		text string
		want bool // true = 通过
	}{
		{"直接命中", "综上，被控产品构成侵权。", false},
		{"否定语境不报", "本方案不构成侵权，可以继续实施。", true},
		{"避免侵权不报", "为避免侵权风险，建议规避设计。", true},
		{"引文豁免", "审查意见认为『请求宣告无效』的理由不成立，我方不予认同。", true},
		{"未闭合引号不豁免", "审查意见认为「请求宣告无效的理由不成立。", false},
		{"无关键词", "权利要求 1 包括壳体与转子。", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := risk.Check.Evaluate(tc.text)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if result.Passed != tc.want {
				t.Fatalf("passed = %v, want %v（details=%v）", result.Passed, tc.want, result.Details)
			}
		})
	}
}

func TestStructuralAnalysis(t *testing.T) {
	engine := loadGateEngine(t)
	rule := engine.RuleByID("CON-COMP-0106")
	if rule == nil {
		t.Fatal("缺少 CON-COMP-0106")
	}
	complete := "综上，对比文件 1 未公开该特征，根据专利法第22条第3款，本申请具备创造性。"
	result, err := rule.Check.Evaluate(complete)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Passed || result.Score != 1 {
		t.Fatalf("齐全文本应通过：passed=%v score=%v details=%v", result.Passed, result.Score, result.Details)
	}

	missingLaw := "综上，对比文件 1 未公开该特征，本申请具备创造性。"
	result, err = rule.Check.Evaluate(missingLaw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Passed {
		t.Fatalf("缺法律依据不应通过：%v", result.Details)
	}
}

func TestOutputGateAnnotatesWarn(t *testing.T) {
	engine := loadGateEngine(t)
	gate := NewOutputGate(engine)
	tec := &agentcore.ToolExecutionContext{
		Results: []agentcore.ToolResult{
			{ToolName: "analyze_patent_novelty", Result: "综上，被控产品构成侵权。"},
		},
	}
	gate.AfterToolExecution(context.Background(), nil, tec)
	if !strings.Contains(tec.Results[0].ForLLM, gateMarker) {
		t.Fatalf("应追加门禁标注：%q", tec.Results[0].ForLLM)
	}
	if !strings.Contains(tec.Results[0].ForLLM, "PAT-APPROVAL-001") && !strings.Contains(tec.Results[0].ForLLM, "PAT-RISK-001") {
		t.Fatalf("标注应包含命中的规则：%q", tec.Results[0].ForLLM)
	}

	// 幂等：再次执行不重复追加。
	before := tec.Results[0].ForLLM
	gate.AfterToolExecution(context.Background(), nil, tec)
	if tec.Results[0].ForLLM != before {
		t.Fatal("门禁标注不应重复追加")
	}
}

func TestOutputGateLeavesCleanResultUntouched(t *testing.T) {
	engine := loadGateEngine(t)
	gate := NewOutputGate(engine)
	tec := &agentcore.ToolExecutionContext{
		Results: []agentcore.ToolResult{
			{ToolName: "ipc_classify", Result: "IPC 分类：B65G 47/90"},
		},
	}
	gate.AfterToolExecution(context.Background(), nil, tec)
	if tec.Results[0].ForLLM != "" {
		t.Fatalf("无违规时不应改写：%q", tec.Results[0].ForLLM)
	}
}

// engineWithRule 构造只含一条规则的引擎（含索引），用于验证门禁按 action 判定。
func engineWithRule(rule Rule) *Engine {
	set := &RuleSet{
		Rules:     []Rule{rule},
		ruleIndex: map[string]*Rule{rule.RuleID: &rule},
	}
	return NewEngine(set)
}

func riskRule(action Action) Rule {
	return Rule{
		RuleID: "T-RISK",
		Name:   "测试风险规则",
		Action: action,
		Check: Check{
			Type: "keyword_blocklist",
			Extra: map[string]any{
				"keywords":        []any{"构成侵权"},
				"negationContext": true,
			},
		},
	}
}

func blockGate(rules ...Rule) (*StructuralGate, *agentcore.ToolExecutionContext) {
	set := &RuleSet{Rules: rules, ruleIndex: make(map[string]*Rule, len(rules))}
	for i := range rules {
		set.ruleIndex[rules[i].RuleID] = &rules[i]
	}
	gate := NewStructuralGate(NewEngine(set), []StructuralEntry{{
		Tool:     "render_patent_document",
		TextArgs: []string{"draft"},
		RuleIDs:  []string{"T-RISK"},
	}})
	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{{
			Name:      "render_patent_document",
			Arguments: `{"draft":"综上，被控产品构成侵权。"}`,
		}},
	}
	return gate, tec
}

// warn 不阻断、block 阻断——证明门禁按 action 判定，不是恒拒或恒放。
func TestStructuralGateHonorsAction(t *testing.T) {
	warnGate, warnCalls := blockGate(riskRule(ActionWarn))
	if err := warnGate.BeforeToolExecution(context.Background(), nil, warnCalls); err != nil {
		t.Fatalf("warn 级规则不应阻断：%v", err)
	}

	blockGateInstance, blockCalls := blockGate(riskRule(ActionBlock))
	err := blockGateInstance.BeforeToolExecution(context.Background(), nil, blockCalls)
	if err == nil {
		t.Fatal("block 级规则必须阻断")
	}
	if !strings.Contains(err.Error(), "T-RISK") {
		t.Fatalf("错误应包含规则 id：%v", err)
	}

	// 合规文本在 block 规则下也应放行（反向验证：不是无条件拒绝）。
	cleanGate, cleanCalls := blockGate(riskRule(ActionBlock))
	cleanCalls.ToolCalls[0].Arguments = `{"draft":"本方案不构成侵权。"}`
	if err := cleanGate.BeforeToolExecution(context.Background(), nil, cleanCalls); err != nil {
		t.Fatalf("合规文本不应被阻断：%v", err)
	}
}

func TestStructuralGateIgnoresOtherTools(t *testing.T) {
	engine := loadGateEngine(t)
	gate := NewStructuralGate(engine, []StructuralEntry{{
		Tool:     "render_patent_document",
		TextArgs: []string{"draft"},
		RuleIDs:  []string{"PAT-RISK-001"},
	}})
	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{{
			Name:      "ipc_classify",
			Arguments: `{"draft":"构成侵权"}`,
		}},
	}
	if err := gate.BeforeToolExecution(context.Background(), nil, tec); err != nil {
		t.Fatalf("未声明的工具不应被门禁：%v", err)
	}
}

func TestStructuralGateWhenArgs(t *testing.T) {
	engine := loadGateEngine(t)
	gate := NewStructuralGate(engine, []StructuralEntry{{
		Tool:     "render_doc_template",
		TextArgs: []string{"body"},
		RuleIDs:  []string{"PAT-ABS-001"},
		WhenArgs: map[string]string{"template": "claims-spec"},
	}})
	args := `{"template":"oa-response","body":"本方案绝对可以实施。"}`
	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{{Name: "render_doc_template", Arguments: args}},
	}
	if err := gate.BeforeToolExecution(context.Background(), nil, tec); err != nil {
		t.Fatalf("whenArgs 不匹配时不应生效：%v", err)
	}
}

func TestValidateGateRulesRejectsUnknownType(t *testing.T) {
	engine := engineWithRule(Rule{
		RuleID: "X-1",
		Action: ActionBlock,
		Check:  Check{Type: "patent_novelty"},
	})
	err := ValidateGateRules(engine, []string{"X-1"})
	if err == nil || !strings.Contains(err.Error(), "不支持确定性门禁") {
		t.Fatalf("应拒绝非确定性类型：%v", err)
	}
	if err := ValidateGateRules(engine, []string{"MISSING"}); err == nil {
		t.Fatal("应拒绝不存在的规则 id")
	}
}
