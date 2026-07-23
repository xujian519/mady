package rules

import (
	"os"
	"path/filepath"
	"testing"
)

const testRulesYAML = `# 测试规则
rules:
  - ruleId: TEST-001
    name: 测试规则一
    description: 新颖性判断测试规则
    legalBasis: 专利法第22条第2款
    domain: patent_novelty
    severity: critical
    action: block
    check:
      type: feature_comparison
      method: separate_comparison
      principles:
        - "单独对比原则"
        - "四相同标准"
      assessment:
        hasDistinctiveFeature: pass
        allFeaturesMatched: fail
  - ruleId: TEST-002
    name: 测试规则二
    description: 创造性判断测试规则
    legalBasis: 专利法第22条第3款
    domain: patent_inventiveness
    severity: major
    action: review
    check:
      type: obviousness
      criteria:
        - priority: 1
          name: 技术领域相同
      assessment:
        noTeaching: pass
        hasTeaching: fail
`

const testArticleYAML = `articleId: "A22.2"
name: "新颖性判断"
lawRef: "专利法第22条第2款"
guidelineRef: "审查指南第二部分第三章"
steps:
  - id: "A22.2_step_1"
    order: 1
    name: "确定对比文件中公开的技术特征"
    ruleRef: "审查指南第二部分第三章 3.1"
    inputHint: "对比文件全文"
    outputSchema:
      disclosedFeatures: "string[] — 公开的技术特征"
conclusionSchema:
  novel: "boolean — 是否具备新颖性"
  confidence: "'high' | 'medium' | 'low'"
applicableTo:
  - patentability
  - invalidity
`

const testOrchestrationYAML = `id: "invalidation"
name: "无效宣告事务"
caseType: "invalidation"
description: "对目标专利提出无效宣告请求"
discoveryStages:
  - name: "目标专利深度分析"
    goal: "提取技术方案"
    suggestions: ["CNIPA获取专利全文"]
availableArticles:
  - articleId: "A22.2"
    priority: 1
    description: "新颖性"
executionTemplate:
  artifactType: "无效宣告请求书"
  sections:
    - "无效理由"
    - "证据清单"
`

const testReflectionYAML = `common:
  description: "通用错误表述"
  phrases:
    - "抱歉"
    - "我错了"
patent:
  description: "专利不确定表述"
  phrases:
    - "待核实"
    - "需人工确认"
`

const testRulesSubdirYAML = `rules:
  - ruleId: SUB-001
    name: 子目录规则测试
    description: 来自 rules/ 子目录的规则
    legalBasis: 专利法第33条
    domain: patent_amendment
    severity: major
    action: review
    check:
      type: text_pattern
      method: llm_judge
      principles:
        - "子目录规则应正常加载"
`

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test-rules.yaml"), []byte(testRulesYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reflection-indicators.yaml"), []byte(testReflectionYAML), 0644); err != nil {
		t.Fatal(err)
	}
	// rules/ 子目录
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "sub-rules.yaml"), []byte(testRulesSubdirYAML), 0644); err != nil {
		t.Fatal(err)
	}
	artDir := filepath.Join(dir, "articles")
	if err := os.MkdirAll(artDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "A22.2-novelty.yaml"), []byte(testArticleYAML), 0644); err != nil {
		t.Fatal(err)
	}
	orchDir := filepath.Join(dir, "orchestrations")
	if err := os.MkdirAll(orchDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orchDir, "invalidation.yaml"), []byte(testOrchestrationYAML), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadFromDir(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	// rules 来自：顶层 test-rules.yaml(2) + rules/ sub-rules.yaml(1) = 3
	if len(rs.Rules) != 3 {
		t.Fatalf("expected 3 rules (2 top-level + 1 rules/ subdir), got %d", len(rs.Rules))
	}
	if rs.Rules[0].RuleID != "TEST-001" {
		t.Errorf("expected TEST-001, got %s", rs.Rules[0].RuleID)
	}
	if rs.Rules[0].Check.Type != "feature_comparison" {
		t.Errorf("expected check type feature_comparison, got %s", rs.Rules[0].Check.Type)
	}
	if rs.Rules[0].Check.Method != "separate_comparison" {
		t.Errorf("expected method separate_comparison, got %s", rs.Rules[0].Check.Method)
	}
	if len(rs.Rules[0].Check.Principles) != 2 {
		t.Errorf("expected 2 principles, got %d", len(rs.Rules[0].Check.Principles))
	}
	if rs.Rules[0].Check.Assessment["hasDistinctiveFeature"] != "pass" {
		t.Errorf("expected assessment pass, got %s", rs.Rules[0].Check.Assessment["hasDistinctiveFeature"])
	}
}

func TestLoadFromDir_RulesSubdir(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	// 验证 rules/ 子目录中的规则被正确加载
	sub := rs.ruleIndex["SUB-001"]
	if sub == nil {
		t.Fatal("SUB-001 from rules/ subdirectory not found")
	}
	if sub.Domain != "patent_amendment" {
		t.Errorf("expected domain 'patent_amendment', got %s", sub.Domain)
	}
	if sub.Severity != SeverityMajor {
		t.Errorf("expected severity major, got %s", sub.Severity)
	}
	if len(sub.Check.Principles) != 1 {
		t.Errorf("expected 1 principle, got %d", len(sub.Check.Principles))
	}
	// 通过 rules/ 子目录加载的规则也能通过 Engine 查询
	e := NewEngine(rs)
	amendRules := e.RulesByDomain("patent_amendment")
	if len(amendRules) != 1 {
		t.Errorf("expected 1 patent_amendment rule, got %d", len(amendRules))
	}
	if amendRules[0].RuleID != "SUB-001" {
		t.Errorf("expected SUB-001 via Engine, got %s", amendRules[0].RuleID)
	}
}

func TestLoadCheckExtra(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	r := rs.ruleIndex["TEST-002"]
	if r == nil {
		t.Fatal("TEST-002 not found")
	}
	if r.Check.Extra == nil {
		t.Fatal("expected Extra to be non-nil for TEST-002")
	}
	if _, ok := r.Check.Extra["criteria"]; !ok {
		t.Errorf("expected 'criteria' in Extra, got keys: %v", r.Check.Extra)
	}
}

func TestRulesByDomain(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	novelty := e.RulesByDomain("patent_novelty")
	if len(novelty) != 1 {
		t.Fatalf("expected 1 novelty rule, got %d", len(novelty))
	}
	if novelty[0].RuleID != "TEST-001" {
		t.Errorf("expected TEST-001, got %s", novelty[0].RuleID)
	}
}

func TestRulesBySeverity(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	critical := e.RulesBySeverity(SeverityCritical)
	if len(critical) != 1 {
		t.Fatalf("expected 1 critical rule, got %d", len(critical))
	}
	major := e.RulesBySeverity(SeverityMajor)
	// TEST-002 (顶级) + SUB-001 (rules/子目录) = 2
	if len(major) != 2 {
		t.Fatalf("expected 2 major rules, got %d", len(major))
	}
}

func TestRuleByID(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	r := e.RuleByID("TEST-001")
	if r == nil {
		t.Fatal("TEST-001 not found")
	}
	if r.Name != "测试规则一" {
		t.Errorf("expected '测试规则一', got %s", r.Name)
	}
	if e.RuleByID("NONEXIST") != nil {
		t.Error("expected nil for nonexistent rule")
	}
}

func TestSearchRules(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	results := e.SearchRules("新颖性")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for '新颖性', got %d", len(results))
	}
	if results[0].RuleID != "TEST-001" {
		t.Errorf("expected TEST-001, got %s", results[0].RuleID)
	}
	results = e.SearchRules("专利法")
	// TEST-001/TEST-002（"专利法第22条"）+ SUB-001（"专利法第33条"）= 3
	if len(results) != 3 {
		t.Fatalf("expected 3 results for '专利法', got %d", len(results))
	}
	results = e.SearchRules("不存在的关键词")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestArticle(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	af := e.Article("A22.2")
	if af == nil {
		t.Fatal("A22.2 not found")
	}
	if af.Name != "新颖性判断" {
		t.Errorf("expected '新颖性判断', got %s", af.Name)
	}
	if len(af.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(af.Steps))
	}
	if af.Steps[0].Order != 1 {
		t.Errorf("expected order 1, got %d", af.Steps[0].Order)
	}
	if len(af.ApplicableTo) != 2 {
		t.Errorf("expected 2 applicableTo, got %d", len(af.ApplicableTo))
	}
}

func TestOrchestration(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	orch := e.Orchestration("invalidation")
	if orch == nil {
		t.Fatal("invalidation not found")
	}
	if orch.Name != "无效宣告事务" {
		t.Errorf("expected '无效宣告事务', got %s", orch.Name)
	}
	if len(orch.DiscoveryStages) != 1 {
		t.Fatalf("expected 1 discovery stage, got %d", len(orch.DiscoveryStages))
	}
	if len(orch.AvailableArticles) != 1 {
		t.Fatalf("expected 1 available article, got %d", len(orch.AvailableArticles))
	}
	if orch.ExecutionTemplate.ArtifactType != "无效宣告请求书" {
		t.Errorf("expected '无效宣告请求书', got %s", orch.ExecutionTemplate.ArtifactType)
	}
}

func TestReflectionIndicators(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	common := e.ReflectionIndicators("common")
	if common == nil {
		t.Fatal("common reflection domain not found")
	}
	if len(common.Phrases) != 2 {
		t.Fatalf("expected 2 phrases, got %d", len(common.Phrases))
	}
	patent := e.ReflectionIndicators("patent")
	if patent == nil {
		t.Fatal("patent reflection domain not found")
	}
	if len(patent.Phrases) != 2 {
		t.Fatalf("expected 2 patent phrases, got %d", len(patent.Phrases))
	}
}

func TestToRuleConstraints(t *testing.T) {
	dir := setupTestDir(t)
	rs, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}
	e := NewEngine(rs)
	constraints := e.ToRuleConstraints("patent_novelty")
	if len(constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(constraints))
	}
	if constraints[0].ArticleID != "TEST-001" {
		t.Errorf("expected TEST-001, got %s", constraints[0].ArticleID)
	}
	if constraints[0].Requirement != "must" {
		t.Errorf("expected 'must' for critical, got %s", constraints[0].Requirement)
	}
	constraints = e.ToRuleConstraints("patent_inventiveness")
	if len(constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(constraints))
	}
	if constraints[0].Requirement != "should" {
		t.Errorf("expected 'should' for major, got %s", constraints[0].Requirement)
	}
}
