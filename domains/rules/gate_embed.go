package rules

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// 随包门禁规则：批 A 的确定性规则随二进制分发，不依赖 $MADY_HOME 的部署目录。
//
// 与用户规则集分开加载的原因：门禁是安全能力，不能因为部署目录缺失或其中某个
// 规则文件解析失败而静默失效；同时避免把整个规则目录的解析问题带进门禁启动路径。
//
//go:embed data/rules/gate-batch-a.yaml data/rules/gate
var bundledGateYAML embed.FS

// gateRuleFiles 是随包门禁规则文件：手工维护的补充规则 + 由 scripts/rulescan 生成的批 A。
var gateRuleFiles = []string{
	"data/rules/gate-batch-a.yaml",
	"data/rules/gate/gate-batch-a-block.yaml",
	"data/rules/gate/gate-batch-a-structural.yaml",
}

// LoadBundledGateRules 解析并合并随包门禁规则。
func LoadBundledGateRules() (*RuleSet, error) {
	set := &RuleSet{
		ruleIndex:       make(map[string]*Rule),
		rulesByDomain:   make(map[string][]Rule),
		rulesBySeverity: make(map[Severity][]Rule),
	}
	for _, name := range gateRuleFiles {
		data, err := bundledGateYAML.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("rules: 读取随包门禁规则 %s: %w", name, err)
		}
		var file struct {
			Rules []Rule `yaml:"rules"`
		}
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("rules: 解析随包门禁规则 %s: %w", name, err)
		}
		for _, rule := range file.Rules {
			if rule.RuleID == "" {
				return nil, fmt.Errorf("rules: %s 存在空 ruleId", name)
			}
			if _, dup := set.ruleIndex[rule.RuleID]; dup {
				return nil, fmt.Errorf("rules: 随包门禁规则 ruleId 重复：%s（%s）", rule.RuleID, name)
			}
			set.Rules = append(set.Rules, rule)
		}
	}
	if len(set.Rules) == 0 {
		return nil, fmt.Errorf("rules: 随包门禁规则为空")
	}
	for i := range set.Rules {
		rule := &set.Rules[i]
		set.ruleIndex[rule.RuleID] = rule
		set.rulesByDomain[rule.Domain] = append(set.rulesByDomain[rule.Domain], *rule)
		set.rulesBySeverity[rule.Severity] = append(set.rulesBySeverity[rule.Severity], *rule)
	}
	return set, nil
}

// NewBundledEngine 构造只含随包门禁规则的引擎。
func NewBundledEngine() (*Engine, error) {
	set, err := LoadBundledGateRules()
	if err != nil {
		return nil, err
	}
	return NewEngine(set), nil
}
