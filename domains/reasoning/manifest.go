package reasoning

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// WorkflowManifest defines the complete five-step workflow for a case type.
// It is the YAML-driven "how" — the executable configuration that drives
// Stage ① through Stage ⑤.
//
// A new professional scenario should only require a new YAML file — the
// five-step skeleton code does not change.
type WorkflowManifest struct {
	ID       string       `yaml:"id" json:"id"`
	Name     string       `yaml:"name" json:"name"`
	CaseType CaseType     `yaml:"case_type" json:"case_type"`
	Stage1   Stage1Config `yaml:"stage1" json:"stage1"`
	Stage2   Stage2Config `yaml:"stage2" json:"stage2"`
	Stage3   Stage3Config `yaml:"stage3" json:"stage3"`
	Stage4   Stage4Config `yaml:"stage4" json:"stage4"`
	Stage5   Stage5Config `yaml:"stage5" json:"stage5"`
}

// Stage1Config configures fact collection.
type Stage1Config struct {
	Collectors []CollectorCfg `yaml:"collectors" json:"collectors"`
}

// CollectorCfg configures a single fact collector.
type CollectorCfg struct {
	Type    string         `yaml:"type" json:"type"` // "user_input" | "documents" | "knowledge" | "derived"
	Enabled bool           `yaml:"enabled" json:"enabled"`
	Config  map[string]any `yaml:"config" json:"config"`
}

// Stage2Config configures rule retrieval.
type Stage2Config struct {
	ManifestID  string          `yaml:"manifest_id" json:"manifest_id"` // references RuleRetrievalManifest
	Sources     []RuleSourceCfg `yaml:"sources" json:"sources"`
	Aggregation string          `yaml:"aggregation" json:"aggregation"`
	MaxRules    int             `yaml:"max_rules" json:"max_rules"`
}

// Stage3Config configures plan generation.
type Stage3Config struct {
	PlanTemplates []PlanTemplateCfg `yaml:"plan_templates" json:"plan_templates"`
	LLMConfig     map[string]any    `yaml:"llm_config,omitempty" json:"llm_config,omitempty"`
}

// PlanTemplateCfg maps an intent to a Plan template reference.
type PlanTemplateCfg struct {
	Intent      PlanIntent `yaml:"intent" json:"intent"`
	TemplateRef string     `yaml:"template_ref" json:"template_ref"` // key in the built-in template map
}

// Stage4Config configures plan execution.
type Stage4Config struct {
	DefaultStrategy StrategyType `yaml:"default_strategy" json:"default_strategy"`
	MaxSteps        int          `yaml:"max_steps" json:"max_steps"` // Pregel max steps
	Steps           []PlanStep   `yaml:"steps" json:"steps"`
}

// Stage5Config configures syllogism checking.
type Stage5Config struct {
	SyllogismLevel      int  `yaml:"syllogism_level" json:"syllogism_level"` // 1-3
	LLMValidate         bool `yaml:"llm_validate" json:"llm_validate"`
	RequireAllFactsUsed bool `yaml:"require_all_facts_used" json:"require_all_facts_used"`
	RequireAllRulesUsed bool `yaml:"require_all_rules_used" json:"require_all_rules_used"`
	MaxRetries          int  `yaml:"max_retries" json:"max_retries"`
}

// ToRuleRetrievalManifest converts Stage2Config to RuleRetrievalManifest.
func (c Stage2Config) ToRuleRetrievalManifest() RuleRetrievalManifest {
	return RuleRetrievalManifest{
		ManifestID:  c.ManifestID,
		Name:        c.ManifestID,
		Sources:     c.Sources,
		Aggregation: c.Aggregation,
		MaxRules:    c.MaxRules,
	}
}

// ToPlan converts Stage4Config steps to a Plan skeleton.
func (c Stage4Config) ToPlan(caseType CaseType) *Plan {
	return &Plan{
		PlanID:   "plan_from_manifest",
		Intent:   PlanIntentChain,
		CaseType: caseType,
		Steps:    c.Steps,
	}
}

// manifestFile is the YAML envelope for a WorkflowManifest.
type manifestFile struct {
	WorkflowManifest WorkflowManifest `yaml:"workflow_manifest"`
}

// WorkflowManifestStore loads and queries WorkflowManifests from a directory.
type WorkflowManifestStore struct {
	mu        sync.RWMutex
	manifests map[string]*WorkflowManifest   // key: manifest ID
	byCase    map[CaseType]*WorkflowManifest // key: CaseType (last-loaded wins)
}

// NewWorkflowManifestStore creates an empty store.
func NewWorkflowManifestStore() *WorkflowManifestStore {
	return &WorkflowManifestStore{
		manifests: make(map[string]*WorkflowManifest),
		byCase:    make(map[CaseType]*WorkflowManifest),
	}
}

// LoadDir loads all YAML files from a directory into the store.
// Each file should contain a top-level "workflow_manifest" key.
func (s *WorkflowManifestStore) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("manifest store: read dir %s: %w", dir, err)
	}

	var loaded int
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		mf, err := s.loadFile(path)
		if err != nil {
			return fmt.Errorf("manifest store: load %s: %w", path, err)
		}
		s.mu.Lock()
		s.manifests[mf.ID] = mf
		s.byCase[mf.CaseType] = mf
		s.mu.Unlock()
		loaded++
	}

	if loaded == 0 {
		return fmt.Errorf("manifest store: no YAML files found in %s", dir)
	}
	return nil
}

func (s *WorkflowManifestStore) loadFile(path string) (*WorkflowManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var mf manifestFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	if mf.WorkflowManifest.ID == "" {
		return nil, fmt.Errorf("workflow_manifest.id is required")
	}
	if mf.WorkflowManifest.CaseType == "" {
		return nil, fmt.Errorf("workflow_manifest.case_type is required")
	}

	return &mf.WorkflowManifest, nil
}

// Get returns the manifest for a given ID.
func (s *WorkflowManifestStore) Get(id string) (*WorkflowManifest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.manifests[id]
	return m, ok
}

// GetByCaseType returns the manifest registered for a CaseType.
func (s *WorkflowManifestStore) GetByCaseType(ct CaseType) (*WorkflowManifest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byCase[ct]
	return m, ok
}

// Register adds a programmatically constructed manifest.
func (s *WorkflowManifestStore) Register(m *WorkflowManifest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifests[m.ID] = m
	s.byCase[m.CaseType] = m
}

// List returns all loaded manifest IDs.
func (s *WorkflowManifestStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.manifests))
	for id := range s.manifests {
		ids = append(ids, id)
	}
	return ids
}

// =============================================================================
// Global workflow manifest store
// =============================================================================

// globalWorkflowStore 是全局 WorkflowManifestStore 单例，
// 由 framework.go 启动时调用 LoadYAMLWorkflowManifests 填充。
// NewWorkflowRunner 优先从此读取 YAML 配置，缺失时回退到 DefaultManifests()。
var globalWorkflowStore = NewWorkflowManifestStore()

// GlobalWorkflowStore 返回全局 workflow manifest 存储实例。
// 调用方（framework.go / NewWorkflowRunner）通过此函数访问。
func GlobalWorkflowStore() *WorkflowManifestStore {
	return globalWorkflowStore
}

// =============================================================================
// Built-in default manifests (fallback when no YAML is available).
// =============================================================================

// DefaultManifests returns the built-in WorkflowManifests for all supported
// case types. These serve as fallbacks when no YAML configuration exists.
func DefaultManifests() []*WorkflowManifest {
	return []*WorkflowManifest{
		defaultNoveltySearchManifest(),
		defaultPatentabilityManifest(),
		defaultDraftingManifest(),
		defaultInvalidationManifest(),
	}
}

func defaultNoveltySearchManifest() *WorkflowManifest {
	return &WorkflowManifest{
		ID:       "patent_novelty_default",
		Name:     "专利新颖性分析（默认配置）",
		CaseType: CaseNoveltySearch,
		Stage1: Stage1Config{
			Collectors: []CollectorCfg{
				{Type: "user_input", Enabled: true, Config: map[string]any{"max_facts": 10}},
				{Type: "documents", Enabled: true, Config: map[string]any{"max_facts": 20}},
				{Type: "knowledge", Enabled: true, Config: map[string]any{"top_k": 5}},
			},
		},
		Stage2: Stage2Config{
			ManifestID: "patent_novelty_rules",
			Sources: []RuleSourceCfg{
				{Source: RuleSourceRules, MaxPerSource: 10, Weight: 1.2},
				{Source: RuleSourceKG, MaxPerSource: 10, Weight: 1.0},
				{Source: RuleSourceVector, MaxPerSource: 5, Weight: 0.8},
			},
			Aggregation: "priority",
			MaxRules:    10,
		},
		Stage3: Stage3Config{},
		Stage4: Stage4Config{
			DefaultStrategy: StrategyChain,
			MaxSteps:        30,
			Steps: []PlanStep{
				{Order: 1, Description: "解析技术交底书，提取技术特征", Strategy: StrategyChain},
				{Order: 2, Description: "检索现有技术文献", Strategy: StrategyReact},
				{Order: 3, Description: "逐项对比技术特征与现有技术", Strategy: StrategyChain},
				{Order: 4, Description: "生成新颖性分析结论", Strategy: StrategyChain},
			},
		},
		Stage5: Stage5Config{
			SyllogismLevel: 1,
			LLMValidate:    false,
			MaxRetries:     2,
		},
	}
}

func defaultPatentabilityManifest() *WorkflowManifest {
	return &WorkflowManifest{
		ID:       "patent_patentability_default",
		Name:     "专利可专利性分析（默认配置）",
		CaseType: CasePatentability,
		Stage1: Stage1Config{
			Collectors: []CollectorCfg{
				{Type: "user_input", Enabled: true, Config: map[string]any{"max_facts": 10}},
				{Type: "documents", Enabled: true, Config: map[string]any{"max_facts": 20}},
				{Type: "knowledge", Enabled: true, Config: map[string]any{"top_k": 5}},
				{Type: "derived", Enabled: true, Config: map[string]any{"max_facts": 10}},
			},
		},
		Stage2: Stage2Config{
			ManifestID: "patent_patentability_rules",
			Sources: []RuleSourceCfg{
				{Source: RuleSourceRules, MaxPerSource: 15, Weight: 1.2},
				{Source: RuleSourceKG, MaxPerSource: 15, Weight: 1.0},
				{Source: RuleSourceVector, MaxPerSource: 5, Weight: 0.8},
				{Source: RuleSourceSkill, MaxPerSource: 5, Weight: 0.6},
			},
			Aggregation: "priority",
			MaxRules:    15,
		},
		Stage3: Stage3Config{},
		Stage4: Stage4Config{
			DefaultStrategy: StrategyChain,
			MaxSteps:        50,
			Steps: []PlanStep{
				{Order: 1, Description: "解析技术交底书", Strategy: StrategyChain},
				{Order: 2, Description: "检索现有技术", Strategy: StrategyReact},
				{Order: 3, Description: "新颖性比对", Strategy: StrategyChain},
				{Order: 4, Description: "创造性分析（显而易见性判断）", Strategy: StrategyMultiHypothesis},
				{Order: 5, Description: "生成可专利性综合报告", Strategy: StrategyChain},
			},
		},
		Stage5: Stage5Config{
			SyllogismLevel: 2,
			LLMValidate:    true,
			MaxRetries:     2,
		},
	}
}

// defaultDraftingManifest defines the five-step workflow for patent claim
// drafting (权利要求撰写), covering A31 single-unity/divisional and R42
// divisional-application exam questions.
//
// Step design adapted from Athena "task_1_4_write_claims.md" (the most
// complete claim-drafting flow) and XiaoNuo Agent's patent-orchestrator
// five-step drafting SOP:
//  1. Parse disclosure → extract technical features
//  2. Determine protection-scope strategy → draft independent claim
//     (preamble + characterizing portion "其特征在于")
//  3. Draft dependent claims (additional-feature / parameter /
//     structural-refinement / functional-limitation types)
//  4. Analyze unity → judge unity/divisional feasibility
//  5. Generate claim-feature comparison table + quality check
//
// Reference: Athena保护范围 A/B/C 策略 (激进/平衡/保守), 从属权利要求四类型,
// 权利要求特征对照表 (权项 vs D1/D1+D2 区别 + 授权概率标注).
func defaultDraftingManifest() *WorkflowManifest {
	return &WorkflowManifest{
		ID:       "patent_drafting_default",
		Name:     "专利权利要求撰写（默认配置）",
		CaseType: CaseDrafting,
		Stage1: Stage1Config{
			Collectors: []CollectorCfg{
				{Type: "user_input", Enabled: true, Config: map[string]any{"max_facts": 15}},
				{Type: "documents", Enabled: true, Config: map[string]any{"max_facts": 20}},
				{Type: "derived", Enabled: true, Config: map[string]any{"max_facts": 10}},
			},
		},
		Stage2: Stage2Config{
			ManifestID: "patent_drafting_rules",
			Sources: []RuleSourceCfg{
				{Source: RuleSourceRules, MaxPerSource: 10, Weight: 1.2},
				{Source: RuleSourceKG, MaxPerSource: 10, Weight: 1.0},
				{Source: RuleSourceVector, MaxPerSource: 5, Weight: 0.8},
			},
			Aggregation: "priority",
			MaxRules:    10,
		},
		Stage3: Stage3Config{},
		Stage4: Stage4Config{
			DefaultStrategy: StrategyChain,
			MaxSteps:        40,
			Steps: []PlanStep{
				{
					Order:       1,
					Description: "解析技术交底书，提取技术方案与必要技术特征（区分必要技术特征与附加技术特征）",
					Strategy:    StrategyChain,
				},
				{
					Order:       2,
					Description: "确定保护范围策略（激进/平衡/保守），撰写独立权利要求：前序部分（技术主题+共有特征）+ 特征部分（其特征在于+区别特征）",
					Strategy:    StrategyChain,
				},
				{
					Order:       3,
					Description: "撰写从属权利要求：附加技术特征型、具体参数型、结构细化型、功能限定型，建立合理的引用关系",
					Strategy:    StrategyChain,
				},
				{
					Order:       4,
					Description: "分析单一性：判断各独立权利要求是否含相同或相应的特定技术特征，评估合案/分案申请可行性",
					Strategy:    StrategyMultiHypothesis,
				},
				{
					Order:       5,
					Description: "生成权利要求特征对照表（各权项 vs 对比文件区别 + 授权概率标注），完成撰写质量检查",
					Strategy:    StrategyChain,
				},
			},
		},
		Stage5: Stage5Config{
			SyllogismLevel: 1,
			LLMValidate:    false,
			MaxRetries:     2,
		},
	}
}

// defaultInvalidationManifest defines the five-step workflow for patent
// invalidation analysis (无效宣告分析), covering A33 amendment-scope exam
// questions and future P2B invalidation-decision scenarios.
//
// Step design adapted from XiaoNuo Agent "invalidity_checker.yaml" (the most
// authoritative invalidation SOP) and Athena "task07_invalid_strategy.md" /
// "cap07_invalid.md" (4-step flow + evidence-combination strategies):
//  1. Parse claims + requester's invalidation grounds
//  2. Analyze each invalidation ground independently (novelty A22.2 /
//     inventiveness A22.3 / disclosure A26.3 / amendment A33)
//  3. Organize prior-art evidence chains, evaluate combination strategies
//  4. Assess whether each ground is established (must argue independently,
//     NEVER substitute "综合来看" for per-claim analysis)
//  5. Conclude (maintain / invalidate-all / invalidate-partial) + legal basis
//
// Hard constraints ported from XiaoNuo invalidity_checker:
//   - Each invalidation ground MUST be argued independently per claim
//   - Multi-document combinations MUST justify combination motivation
//   - Prior-art publication dates MUST be verified against priority date
//
// Evidence combination strategies (from both projects, identical):
//
//	方案1: single D1 defeats novelty | 方案2: D1+D2 defeats inventiveness
//	方案3: D1+common knowledge | 方案4: comprehensive multi-ground
func defaultInvalidationManifest() *WorkflowManifest {
	return &WorkflowManifest{
		ID:       "patent_invalidation_default",
		Name:     "专利无效宣告分析（默认配置）",
		CaseType: CaseInvalidation,
		Stage1: Stage1Config{
			Collectors: []CollectorCfg{
				{Type: "user_input", Enabled: true, Config: map[string]any{"max_facts": 15}},
				{Type: "documents", Enabled: true, Config: map[string]any{"max_facts": 25}},
				{Type: "knowledge", Enabled: true, Config: map[string]any{"top_k": 5}},
				{Type: "derived", Enabled: true, Config: map[string]any{"max_facts": 10}},
			},
		},
		Stage2: Stage2Config{
			ManifestID: "patent_invalidation_rules",
			Sources: []RuleSourceCfg{
				{Source: RuleSourceRules, MaxPerSource: 15, Weight: 1.2},
				{Source: RuleSourceKG, MaxPerSource: 15, Weight: 1.0},
				{Source: RuleSourceVector, MaxPerSource: 5, Weight: 0.8},
				{Source: RuleSourceSkill, MaxPerSource: 5, Weight: 0.6},
			},
			Aggregation: "priority",
			MaxRules:    15,
		},
		Stage3: Stage3Config{},
		Stage4: Stage4Config{
			DefaultStrategy: StrategyChain,
			MaxSteps:        50,
			Steps: []PlanStep{
				{
					Order:       1,
					Description: "解析涉案专利权利要求（独立/从属，前序/特征部分）与请求人提交的无效理由",
					Strategy:    StrategyChain,
				},
				{
					Order:       2,
					Description: "逐项分析无效理由：新颖性（第22条第2款，单独对比）、创造性（第22条第3款，三步法）、公开充分（第26条第3款）、修改超范围（第33条）",
					Strategy:    StrategyChain,
				},
				{
					Order:       3,
					Description: "组织对比文件证据链，评估证据组合策略（单篇新颖性/多篇创造性/公知常识组合/综合方案），核实对比文件公开日是否早于优先权日",
					Strategy:    StrategyReact,
				},
				{
					Order:       4,
					Description: "逐项评估各无效理由是否成立：每项理由须独立论证，不得以'综合来看'代替逐条分析；多篇组合须论证组合动机",
					Strategy:    StrategyMultiHypothesis,
				},
				{
					Order:       5,
					Description: "给出审查结论（维持有效/全部无效/部分无效），说明核心理由与依据法条",
					Strategy:    StrategyChain,
				},
			},
		},
		Stage5: Stage5Config{
			SyllogismLevel:      2,
			LLMValidate:         true,
			RequireAllRulesUsed: true,
			MaxRetries:          2,
		},
	}
}
