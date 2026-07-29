// Package worker provides a Worker contract system for patent domain agents.
//
// Workers are specialized sub-agents with explicit input/output contracts,
// tier classification, and quality gates. The Worker system enables:
//   - Declarative Worker definitions (YAML/JSON)
//   - Input/Output contract validation at registration time
//   - Tier-based routing (Work → Provision → Reasoning → Domain → Checker)
//   - Automatic verification of Worker registration completeness
//
// Usage:
//
//	registry := worker.NewRegistry()
//	w := worker.Definition{
//	    Name:        "patent-technical-analyzer",
//	    Tier:        worker.TierWork,
//	    Description: "分析技术交底书，提取技术要素",
//	    Inputs:      []worker.Input{{Path: "data/cases/{caseId}/disclosure/*.md"}},
//	    Outputs:     []worker.Output{{Path: "data/cases/{caseId}/outputs/technical-analysis.md"}},
//	}
//	registry.Register(w)
package worker

// WorkerTier classifies Workers into five capability layers.
type WorkerTier string

const (
	TierWork      WorkerTier = "work"      // 工序 Worker：直接执行专利工序
	TierProvision WorkerTier = "provision" // 条款 Worker：法条适配与判断
	TierReasoning WorkerTier = "reasoning" // 推理 Worker：逻辑推理与分析
	TierDomain    WorkerTier = "domain"    // 领域 Worker：领域专业任务
	TierChecker   WorkerTier = "checker"   // 复核 Worker：交叉复核产出质量
)

// ContractLevel defines the strictness of an output contract.
type ContractLevel string

const (
	ContractHard       ContractLevel = "hard"       // 必须满足，不可妥协
	ContractSoft       ContractLevel = "soft"       // 推荐满足，可协商
	ContractStructured ContractLevel = "structured" // 结构化格式要求
)

// Input defines an input artifact contract for a Worker.
type Input struct {
	// Path is the expected input file path. May contain {caseId} placeholder.
	Path string `yaml:"path" json:"path"`

	// ContentSchema lists fields/content patterns that must appear in the input.
	ContentSchema []string `yaml:"contentSchema,omitempty" json:"content_schema,omitempty"`

	// QualityGate references a quality check ID that must pass on this input.
	QualityGate string `yaml:"qualityGate,omitempty" json:"quality_gate,omitempty"`

	// Optional marks whether this input is optional (default false).
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`

	// Description describes what this input is used for.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Output defines an output artifact contract for a Worker.
type Output struct {
	// Path is the expected output file path. May contain {caseId} placeholder.
	Path string `yaml:"path" json:"path"`

	// Format specifies the expected output format.
	Format string `yaml:"format,omitempty" json:"format,omitempty"` // "markdown" | "json"

	// StandardKey references a quality standard definition.
	StandardKey string `yaml:"standardKey,omitempty" json:"standard_key,omitempty"`

	// ContractLevel specifies the strictness of this contract.
	ContractLevel ContractLevel `yaml:"contractLevel,omitempty" json:"contract_level,omitempty"`
}

// Definition defines a single Worker's identity, contracts, and constraints.
type Definition struct {
	// Name is the unique identifier for this Worker.
	Name string `yaml:"name" json:"name"`

	// Tier classifies this Worker.
	Tier WorkerTier `yaml:"tier" json:"tier"`

	// Description explains what this Worker does.
	Description string `yaml:"description" json:"description"`

	// AllowedTools lists the tools this Worker may call (empty = no restriction).
	AllowedTools []string `yaml:"allowedTools,omitempty" json:"allowed_tools,omitempty"`

	// Inputs defines the expected input artifacts.
	Inputs []Input `yaml:"inputs,omitempty" json:"inputs,omitempty"`

	// Outputs defines the expected output artifacts.
	Outputs []Output `yaml:"outputs,omitempty" json:"outputs,omitempty"`

	// ForbiddenActions lists actions this Worker must never perform.
	ForbiddenActions []string `yaml:"forbiddenActions,omitempty" json:"forbidden_actions,omitempty"`

	// CanInvoke lists other Workers this Worker may delegate to.
	CanInvoke []string `yaml:"canInvoke,omitempty" json:"can_invoke,omitempty"`

	// TriggersHITL marks whether this Worker's output requires human approval.
	TriggersHITL bool `yaml:"triggersHITL,omitempty" json:"triggers_hitl,omitempty"`

	// PreRegister marks whether this Worker is registered at startup (true)
	// or lazily on first use (false). Defaults to true.
	PreRegister *bool `yaml:"preRegister,omitempty" json:"pre_register,omitempty"`
}

// IsPreRegistered returns true if this Worker should be pre-registered.
func (d *Definition) IsPreRegistered() bool {
	if d.PreRegister == nil {
		return true
	}
	return *d.PreRegister
}
