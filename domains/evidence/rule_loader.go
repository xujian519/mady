package evidence

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

type rulesDoc struct {
	Rules []EvidenceRule `yaml:"rules"`
}

type RuleIndex struct {
	mu     sync.RWMutex
	byID   map[string]EvidenceRule
	byType map[EvidenceType][]EvidenceRule
	all    []EvidenceRule
}

func NewRuleIndex() *RuleIndex {
	return &RuleIndex{
		byID:   make(map[string]EvidenceRule),
		byType: make(map[EvidenceType][]EvidenceRule),
	}
}

func (idx *RuleIndex) LoadYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取证据规则文件失败 %s: %w", path, err)
	}
	return idx.LoadBytes(data)
}

func (idx *RuleIndex) LoadBytes(data []byte) error {
	var doc rulesDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("解析证据规则 YAML 失败: %w", err)
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 重置索引，防止重复加载时累积重复条目
	idx.byID = make(map[string]EvidenceRule)
	idx.byType = make(map[EvidenceType][]EvidenceRule)
	idx.all = nil

	for i := range doc.Rules {
		rule := &doc.Rules[i]
		if rule.RuleID == "" {
			return fmt.Errorf("规则 #%d: ruleId 为空", i+1)
		}
		if !rule.EvidenceType.Valid() && rule.EvidenceType != "" {
			return fmt.Errorf("规则 %s: 未知的 evidenceType %q", rule.RuleID, rule.EvidenceType)
		}
		idx.byID[rule.RuleID] = *rule
		idx.byType[rule.EvidenceType] = append(idx.byType[rule.EvidenceType], *rule)
		idx.all = append(idx.all, *rule)
	}

	sort.Slice(idx.all, func(i, j int) bool {
		return ruleSeverityOrder(idx.all[i].Severity) < ruleSeverityOrder(idx.all[j].Severity)
	})
	return nil
}

func (idx *RuleIndex) GetRule(ruleID string) (EvidenceRule, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	r, ok := idx.byID[ruleID]
	return r, ok
}

func (idx *RuleIndex) GetRulesByType(evType EvidenceType) []EvidenceRule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	rules := append([]EvidenceRule(nil), idx.byType[evType]...)
	if evType != EvTypeGeneral {
		rules = append(rules, idx.byType[EvTypeGeneral]...)
	}
	return rules
}

func (idx *RuleIndex) AllRules() []EvidenceRule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]EvidenceRule, len(idx.all))
	copy(out, idx.all)
	return out
}

func (idx *RuleIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.all)
}

func ruleSeverityOrder(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "major":
		return 1
	case "minor":
		return 2
	default:
		return 3
	}
}
