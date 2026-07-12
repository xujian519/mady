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
// Built-in default manifests (fallback when no YAML is available).
// =============================================================================

// DefaultManifests returns the built-in WorkflowManifests for all supported
// case types. These serve as fallbacks when no YAML configuration exists.
func DefaultManifests() []*WorkflowManifest {
	return []*WorkflowManifest{
		defaultNoveltySearchManifest(),
		defaultPatentabilityManifest(),
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
