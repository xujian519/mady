package evidence

import (
	"testing"
)

func TestNewRuleIndex(t *testing.T) {
	idx := NewRuleIndex()
	if idx == nil {
		t.Fatal("NewRuleIndex() 返回 nil")
	}
	if idx.Count() != 0 {
		t.Errorf("新索引规则数 = %d, 期望 %d", idx.Count(), 0)
	}
}

func TestRuleIndex_LoadBytes(t *testing.T) {
	idx := NewRuleIndex()

	yamlData := []byte(`
rules:
  - ruleId: EVI-001
    name: 规则一
    description: 测试用
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
  - ruleId: EVI-002
    name: 规则二
    description: 测试用
    legalBasis: 专利法
    domain: patent
    severity: critical
    action: apply
    evidenceType: electronic
`)

	if err := idx.LoadBytes(yamlData); err != nil {
		t.Fatalf("LoadBytes() 返回错误: %v", err)
	}

	if idx.Count() != 2 {
		t.Errorf("规则数 = %d, 期望 %d", idx.Count(), 2)
	}
}

func TestRuleIndex_GetRule(t *testing.T) {
	idx := NewRuleIndex()

	yamlData := []byte(`
rules:
  - ruleId: EVI-TEST
    name: 测试规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: minor
    action: apply
    evidenceType: general
`)

	if err := idx.LoadBytes(yamlData); err != nil {
		t.Fatalf("LoadBytes() 返回错误: %v", err)
	}

	rule, ok := idx.GetRule("EVI-TEST")
	if !ok {
		t.Fatal("GetRule(EVI-TEST) 未找到")
	}
	if rule.RuleID != "EVI-TEST" {
		t.Errorf("RuleID = %q, 期望 %q", rule.RuleID, "EVI-TEST")
	}
	if rule.Severity != "minor" {
		t.Errorf("Severity = %q, 期望 %q", rule.Severity, "minor")
	}

	_, ok = idx.GetRule("NONEXISTENT")
	if ok {
		t.Error("GetRule(NONEXISTENT) 应返回 false")
	}
}

func TestRuleIndex_GetRulesByType(t *testing.T) {
	idx := NewRuleIndex()

	yamlData := []byte(`
rules:
  - ruleId: EVI-GEN-1
    name: 通用规则1
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
  - ruleId: EVI-ELEC-1
    name: 电子规则1
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: electronic
  - ruleId: EVI-GEN-2
    name: 通用规则2
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: minor
    action: apply
    evidenceType: general
`)

	if err := idx.LoadBytes(yamlData); err != nil {
		t.Fatalf("LoadBytes() 返回错误: %v", err)
	}

	elecRules := idx.GetRulesByType(EvTypeElectronic)
	if len(elecRules) != 3 {
		t.Errorf("电子类型规则数 = %d, 期望 %d（含通用规则回退）", len(elecRules), 3)
	}

	genRules := idx.GetRulesByType(EvTypeGeneral)
	if len(genRules) != 2 {
		t.Errorf("通用类型规则数 = %d, 期望 %d", len(genRules), 2)
	}
}

func TestRuleIndex_AllRules_SortedBySeverity(t *testing.T) {
	idx := NewRuleIndex()

	yamlData := []byte(`
rules:
  - ruleId: EVI-001
    name: 次要规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: minor
    action: apply
    evidenceType: general
  - ruleId: EVI-002
    name: 严重规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: critical
    action: apply
    evidenceType: general
  - ruleId: EVI-003
    name: 中等规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
`)

	if err := idx.LoadBytes(yamlData); err != nil {
		t.Fatalf("LoadBytes() 返回错误: %v", err)
	}

	all := idx.AllRules()
	if len(all) != 3 {
		t.Fatalf("规则数 = %d, 期望 %d", len(all), 3)
	}

	expectedOrder := []string{"critical", "major", "minor"}
	for i, rule := range all {
		if rule.Severity != expectedOrder[i] {
			t.Errorf("排序错误 [%d]: Severity = %q, 期望 %q", i, rule.Severity, expectedOrder[i])
		}
	}
}

func TestRuleIndex_LoadBytes_ResetsOnReload(t *testing.T) {
	idx := NewRuleIndex()

	first := []byte(`
rules:
  - ruleId: EVI-001
    name: 规则一
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
`)

	second := []byte(`
rules:
  - ruleId: EVI-002
    name: 规则二
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: minor
    action: apply
    evidenceType: general
`)

	if err := idx.LoadBytes(first); err != nil {
		t.Fatalf("第一次 LoadBytes() 失败: %v", err)
	}
	if idx.Count() != 1 {
		t.Errorf("第一次加载后规则数 = %d, 期望 %d", idx.Count(), 1)
	}

	if err := idx.LoadBytes(second); err != nil {
		t.Fatalf("第二次 LoadBytes() 失败: %v", err)
	}
	if idx.Count() != 1 {
		t.Errorf("第二次加载后规则数 = %d, 期望 %d（应为重置后新加载的数）", idx.Count(), 1)
	}

	if _, ok := idx.GetRule("EVI-001"); ok {
		t.Error("第二次加载后不应找到 EVI-001")
	}
	if _, ok := idx.GetRule("EVI-002"); !ok {
		t.Error("第二次加载后应能找到 EVI-002")
	}
}

func TestRuleIndex_LoadBytes_ErrorEmptyRuleID(t *testing.T) {
	idx := NewRuleIndex()

	yamlData := []byte(`
rules:
  - ruleId: ""
    name: 空ID规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
`)

	err := idx.LoadBytes(yamlData)
	if err == nil {
		t.Fatal("应返回 ruleId 为空的错误")
	}
}

func TestRuleIndex_LoadBytes_ErrorInvalidType(t *testing.T) {
	idx := NewRuleIndex()

	yamlData := []byte(`
rules:
  - ruleId: EVI-INVALID
    name: 无效类型规则
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: invalid_type_xyz
`)

	err := idx.LoadBytes(yamlData)
	if err == nil {
		t.Fatal("应返回未知 evidenceType 的错误")
	}
}

func TestRuleIndex_Count(t *testing.T) {
	idx := NewRuleIndex()
	if idx.Count() != 0 {
		t.Errorf("空索引 Count = %d, 期望 %d", idx.Count(), 0)
	}

	yamlData := []byte(`
rules:
  - ruleId: EVI-001
    name: 规则A
    description: 测试
    legalBasis: 专利法
    domain: patent
    severity: major
    action: apply
    evidenceType: general
`)

	if err := idx.LoadBytes(yamlData); err != nil {
		t.Fatal(err)
	}
	if idx.Count() != 1 {
		t.Errorf("Count = %d, 期望 %d", idx.Count(), 1)
	}
}
