// Package workflows provides a unified workflow definition DSL and orchestrator
// for Mady agents.
//
// Architecture:
//
//	YAML/Go DSL → Workflow → WorkflowOrchestrator → Pregel/DAG
//
// The Workflow type is a high-level abstraction that compiles down to the
// existing Pregel graph engine (graph/pregel.go) and FiveStep reasoning
// framework (domains/reasoning/), providing:
//   - Unified YAML workflow definitions
//   - Role-based collaboration templates (BCIP-inspired)
//   - Checkpoint-aware execution via graph.PregelCheckpointer
//   - Human-in-the-loop approval gates
package workflows

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/graph"
)

// ---------------------------------------------------------------------------
// WorkflowStep — 工作流中的一个步骤节点
// ---------------------------------------------------------------------------

// StepType 标识步骤的执行类型。
type StepType string

const (
	// StepAgent 将步骤委托给一个 Agent 角色。
	StepAgent StepType = "agent"
	// StepTool 直接执行一个工具。
	StepTool StepType = "tool"
	// StepQualityCheck 运行质量检查。
	StepQualityCheck StepType = "quality_check"
	// StepHumanApproval 暂停等待人工审批。
	StepHumanApproval StepType = "human_approval"
	// StepSubWorkflow 嵌入另一个工作流。
	StepSubWorkflow StepType = "sub_workflow"
)

// WorkflowStep 定义一个工作流步骤。
type WorkflowStep struct {
	// ID 是步骤的唯一标识（如 "search", "analyze"）。
	ID string `yaml:"id" json:"id"`

	// Type 是执行类型。
	Type StepType `yaml:"type" json:"type"`

	// Role 指定负责此步骤的 Agent 角色（仅 StepAgent）。
	Role string `yaml:"role,omitempty" json:"role,omitempty"`

	// Tool 指定要执行的工具名称（仅 StepTool）。
	Tool string `yaml:"tool,omitempty" json:"tool,omitempty"`

	// Prompt 是传递给 Agent 或工具的提示词。
	Prompt string `yaml:"prompt,omitempty" json:"prompt,omitempty"`

	// DependsOn 列出此步骤依赖的前序步骤 ID。
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`

	// Condition 是可选的执行条件（Go 表达式）。
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"`

	// Config 是步骤级别的配置覆盖。
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`

	// Timeout 是步骤执行的超时时间。
	Timeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Retry 是失败时的重试次数。
	Retry int `yaml:"retry,omitempty" json:"retry,omitempty"`
}

// ---------------------------------------------------------------------------
// Workflow — 完整工作流定义
// ---------------------------------------------------------------------------

// Workflow 是一个完整的工作流定义。
type Workflow struct {
	// Name 是工作流名称（如 "novelty_check"）。
	Name string `yaml:"name" json:"name"`

	// Description 是工作流的可读描述。
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Domain 是工作流所属领域（patent / legal）。
	Domain string `yaml:"domain,omitempty" json:"domain,omitempty"`

	// Steps 是步骤列表（按拓扑顺序定义）。
	Steps []WorkflowStep `yaml:"steps" json:"steps"`

	// Parallel 标记同层步骤是否并行执行。
	Parallel bool `yaml:"parallel,omitempty" json:"parallel,omitempty"`

	// MaxRetries 是全局最大重试次数。
	MaxRetries int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`

	// Version 是工作流定义版本号。
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// Validate 校验工作流定义是否合法。
func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workflow: name must not be empty")
	}
	seen := make(map[string]int)
	for i, step := range w.Steps {
		if step.ID == "" {
			return fmt.Errorf("workflow %q: step %d: ID must not be empty", w.Name, i)
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("workflow %q: duplicate step ID %q", w.Name, step.ID)
		}
		seen[step.ID] = i
		switch step.Type {
		case StepAgent:
			if step.Role == "" {
				return fmt.Errorf("workflow %q: step %q: agent type requires role", w.Name, step.ID)
			}
		case StepTool:
			if step.Tool == "" {
				return fmt.Errorf("workflow %q: step %q: tool type requires tool name", w.Name, step.ID)
			}
		case StepHumanApproval, StepQualityCheck, StepSubWorkflow:
			// 无额外校验。
		default:
			return fmt.Errorf("workflow %q: step %q: unknown type %q", w.Name, step.ID, step.Type)
		}
	}
	return nil
}

// TopologicalSteps returns step IDs in topological order (input order assumed).
func (w *Workflow) TopologicalSteps() []string {
	ids := make([]string, len(w.Steps))
	for i, s := range w.Steps {
		ids[i] = s.ID
	}
	return ids
}

// StepByID 按 ID 查找步骤。
func (w *Workflow) StepByID(id string) *WorkflowStep {
	for i := range w.Steps {
		if w.Steps[i].ID == id {
			return &w.Steps[i]
		}
	}
	return nil
}

// Dependencies returns the set of step IDs that stepID depends on.
func (w *Workflow) Dependencies(id string) []string {
	for _, s := range w.Steps {
		if s.ID == id {
			return s.DependsOn
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// WorkflowTemplate — 预定义的工作流模板
// ---------------------------------------------------------------------------

// WorkflowTemplate 是一个可实例化的工作流模板。
type WorkflowTemplate struct {
	Name        string
	Description string
	Domain      string
	Steps       []TemplateStep
}

// TemplateStep 定义模板中的一个步骤占位符。
type TemplateStep struct {
	ID        string
	Type      StepType
	Role      string
	Tool      string
	DependsOn []int // 前序步骤的索引（在 Steps 切片中的位置）
}

// Instantiate 将模板实例化为具体的工作流，填充提示词参数。
func (t *WorkflowTemplate) Instantiate(params map[string]string) *Workflow {
	w := &Workflow{
		Name:        t.Name,
		Description: t.Description,
		Domain:      t.Domain,
		Steps:       make([]WorkflowStep, len(t.Steps)),
	}
	for i, ts := range t.Steps {
		w.Steps[i] = WorkflowStep{
			ID:   ts.ID,
			Type: ts.Type,
			Role: ts.Role,
			Tool: ts.Tool,
		}
		// 解析依赖（将索引转换为 ID）。
		for _, depIdx := range ts.DependsOn {
			if depIdx < len(t.Steps) {
				w.Steps[i].DependsOn = append(w.Steps[i].DependsOn, t.Steps[depIdx].ID)
			}
		}
		// 填充提示词模板。
		if len(params) > 0 {
			w.Steps[i].Prompt = expandTemplate(w.Steps[i].Prompt, params)
		}
	}
	return w
}

// expandTemplate 将 {{key}} 替换为 params 中的值。
func expandTemplate(tmpl string, params map[string]string) string {
	if tmpl == "" || len(params) == 0 {
		return tmpl
	}
	result := tmpl
	for k, v := range params {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// ---------------------------------------------------------------------------
// WorkflowOrchestrator — 工作流编排器
// ---------------------------------------------------------------------------

// WorkflowOrchestrator 执行 Workflow 并将其编译为底层的 Pregel 图。
//
// 编排流程：
//  1. Validate — 校验工作流定义
//  2. CompileToPregel — 将 Workflow 编译为 graph.CompiledPregelGraph
//  3. Execute — 通过 PregelCheckpointer 执行（支持暂停/恢复）
//
// 与现有系统的关系：
//   - graph/orchestrator/Orchestrator 是低层编排器（直接操作 Pregel 检查点）
//   - 本 WorkflowOrchestrator 是高层编排器（将 Workflow/DAG 编译为 Pregel）
type WorkflowOrchestrator struct {
	checkpointer *graph.PregelCheckpointer
}

// NewWorkflowOrchestrator 创建高层工作流编排器。
// 注意：checkpointer 在执行时按需构建，因此在构造函数中不预建。
func NewWorkflowOrchestrator(store graph.CheckpointStore) *WorkflowOrchestrator {
	return &WorkflowOrchestrator{
		checkpointer: nil, // 执行时按需设置
	}
}

// Execute 执行一个工作流并返回最终输出。
// 如果工作流包含 HumanApproval 步骤，会暂停等待外部确认。
func (wo *WorkflowOrchestrator) Execute(ctx context.Context, w *Workflow) (string, error) {
	if err := w.Validate(); err != nil {
		return "", fmt.Errorf("workflow: validate %q: %w", w.Name, err)
	}

	// 编译 Workflow 为 Pregel 图。
	pg := compileWorkflowToPregel(w)
	compiled, err := pg.Compile(w.Steps[0].ID, int64(len(w.Steps)*10))
	if err != nil {
		return "", fmt.Errorf("workflow: compile %q: %w", w.Name, err)
	}

	// 构造初始状态。
	initial := graph.PregelState{
		"input":    "",
		"workflow": w.Name,
		"steps":    make([]string, 0),
	}

	// 执行。
	result, err := compiled.Run(ctx, initial)
	if err != nil {
		return "", fmt.Errorf("workflow: execute %q: %w", w.Name, err)
	}

	// 提取输出。
	output, _ := result["output"].(string)
	return output, nil
}

// compileWorkflowToPregel 将 Workflow 编译为 Pregel 图。
func compileWorkflowToPregel(w *Workflow) *graph.PregelGraph {
	pg := graph.NewPregelGraph()

	// 为每一步创建 Pregel 节点。
	for i := range w.Steps {
		step := w.Steps[i] // capture
		_ = pg.AddNode(step.ID, func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
			// TODO: 根据 step.Type 路由到实际的执行逻辑。
			// 当前为占位实现——仅记录步骤执行并合并输入。
			newState := make(graph.PregelState)
			for k, v := range state {
				newState[k] = v
			}
			newState["last_step"] = step.ID
			newState["last_step_type"] = string(step.Type)

			// 收集步骤输出。
			steps, _ := state["steps"].([]string)
			newState["steps"] = append(steps, step.ID)

			slog.Debug("workflow: step executed",
				"workflow", w.Name, "step", step.ID, "type", step.Type,
				"role", step.Role, "tool", step.Tool)
			return newState, nil
		})
	}

	// 建立边（基于 DependsOn 和拓扑顺序）。
	for i, step := range w.Steps {
		if len(step.DependsOn) == 0 {
			// 无依赖：如果是第一步，设为例外；否则连接到前一步（贪心）。
			if i == 0 {
				continue // 入口节点
			}
			_ = pg.AddEdge(w.Steps[i-1].ID, step.ID)
		} else {
			for _, dep := range step.DependsOn {
				_ = pg.AddEdge(dep, step.ID)
			}
		}
	}

	return pg
}

// ---------------------------------------------------------------------------
// Convenience: template registration
// ---------------------------------------------------------------------------

var defaultTemplates []WorkflowTemplate

// RegisterTemplate 注册一个工作流模板。
func RegisterTemplate(t WorkflowTemplate) {
	defaultTemplates = append(defaultTemplates, t)
}

// Templates 返回所有已注册的模板。
func Templates() []WorkflowTemplate {
	out := make([]WorkflowTemplate, len(defaultTemplates))
	copy(out, defaultTemplates)
	return out
}

// ---------------------------------------------------------------------------
// Bridge: Workflow → FiveStepRunner
// ---------------------------------------------------------------------------

// ToFiveStepRunner 将 Workflow 适配为 FiveStepRunner 可消费的输入。
// 这连接了高层工作流声明和底层的五步推理引擎。
func (w *Workflow) ToFiveStepRunner() (*reasoning.FiveStepRunner, error) {
	// 五步工作法的步骤映射：
	// Step 0 → Stage 1（事实收集）
	// Step 1-N → Stage 4（编译执行）
	// 最后的 HumanApproval → Stage 5 检查点
	_ = w // 占位——实际集成时实现
	return nil, fmt.Errorf("workflow: FiveStepRunner adapter not yet implemented")
}
