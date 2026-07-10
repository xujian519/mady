// Package workflow provides workflow primitives (Pipeline, Parallel, Router)
// and the Planner → Executor → Reflector → Reviewer (PER) general-purpose
// multi-agent workflow.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ──────────────────────────────────────────────────────────
// Config types
// ──────────────────────────────────────────────────────────

// AgentRoleConfig configures one Agent role in the PER pipeline.
type AgentRoleConfig struct {
	Name         string
	Provider     agentcore.Provider
	SystemPrompt string
	Tools        []*agentcore.Tool
	MaxTurns     int64
	Temperature  float64
}

// ToConfig converts the role config to an agentcore.Config.
func (arc AgentRoleConfig) ToConfig() agentcore.Config {
	opts := []agentcore.ConfigOption{
		agentcore.WithName(arc.Name),
		agentcore.WithProvider(arc.Provider),
		agentcore.WithSystemPrompt(arc.SystemPrompt),
		agentcore.WithTemperature(arc.Temperature),
		agentcore.WithTools(arc.Tools...),
	}
	if arc.MaxTurns > 0 {
		opts = append(opts, agentcore.WithMaxTurns(arc.MaxTurns))
	}
	return agentcore.NewConfig(opts...)
}

// ReflectorConfig configures the reflection loop.
type ReflectorConfig struct {
	Judge      AgentRoleConfig // judge agent that evaluates executor outputs
	MaxRetries int             // max retries per execution attempt (default 2)
}

// PERConfig configures the full Planner → Executor → Reflector → Reviewer
// four-stage workflow.
type PERConfig struct {
	Planner   AgentRoleConfig
	Executor  AgentRoleConfig
	Reflector ReflectorConfig
	Reviewer  AgentRoleConfig
}

// ──────────────────────────────────────────────────────────
// Default system prompts
// ──────────────────────────────────────────────────────────

const defaultPlannerPrompt = `你是一个任务规划专家。分析用户的需求，将其分解为有序的执行步骤。

输出 JSON 格式的执行计划（不要包含其他内容）：
{
  "steps": [
    {
      "order": 1,
      "description": "步骤的具体描述",
      "expected_tool": "预期使用的工具或方法（可忽略）",
      "expected_output": "预期输出或完成标准"
    }
  ]
}

要求：
- 每步必须具体、可执行、可验证
- 步骤数量适当（3-8 步），不要过度拆分
- 使用中文描述步骤`

const defaultExecutorPrompt = `你是一个任务执行专家。按照给定的计划逐步执行。

你会收到一个 JSON 格式的执行计划。你的任务是：
1. 理解每一步的要求
2. 按顺序执行每个步骤
3. 如果有可用工具，可以调用它们来辅助执行
4. 执行完成后，汇总汇报每步的执行结果

注意：
- 如果某步失败，说明原因并尝试继续后续步骤
- 确保输出内容完整、准确`

const defaultReflectorJudgePrompt = `你是一个任务评估专家。评估执行结果是否充分满足计划要求。

分析步骤：
1. 计划要求什么？
2. 实际执行了什么？
3. 执行结果与计划之间有什么差距？

以 JSON 格式输出评估结果（不要包含其他内容）：
{"pass": true 或 false, "critique": "改进建议（pass 为 true 时可为空）"}

要求：
- 仔细对比计划和执行结果
- 如果回答基本满足要求，即使不完美也应通过
- 只拒绝明显偏离计划或质量不达标的回答
- 拒绝时提供具体的、可操作的改进建议`

const defaultReviewerPrompt = `你是一个质量审查专家。对最终输出进行完整性、正确性和合规性审查。

审查要点：
1. 输出是否完整，是否有遗漏
2. 内容是否正确、合理
3. 是否符合用户原始需求
4. 是否有安全或合规问题

以 JSON 格式输出审查结果（不要包含其他内容）：
{"pass": true 或 false, "score": 0-100, "issues": ["问题..."], "suggestions": ["建议..."]}`

// ──────────────────────────────────────────────────────────
// PER-specific step types
// ──────────────────────────────────────────────────────────

// verdict is the parsed result from the Reflector Judge.
type verdict struct {
	Pass     bool   `json:"pass"`
	Critique string `json:"critique"`
}

// reflectiveStep implements the executor + reflection retry loop.
// It runs the executor, evaluates the output with a judge, and retries
// with critique feedback if needed.
type reflectiveStep struct {
	executorConfig  AgentRoleConfig
	reflectorConfig ReflectorConfig
}

// NewReflectiveStep creates a Step that wraps an executor with a
// reflection/retry loop. The executor runs, then a judge evaluates the
// result. If the judge says pass=false, the critique is injected and
// the executor retries, up to cfg.MaxRetries times.
//
// Defaults are applied when config fields are zero-valued:
//   - executor.Name → "executor"
//   - judge.Name → "reflector-judge"
//   - MaxRetries → 2
//   - executor.MaxTurns → 20
//   - judge.MaxTurns → 5
//   - prompts → built-in defaults
func NewReflectiveStep(executorCfg AgentRoleConfig, reflectorCfg ReflectorConfig) Step {
	// Apply defaults.
	if executorCfg.Name == "" {
		executorCfg.Name = "executor"
	}
	if reflectorCfg.Judge.Name == "" {
		reflectorCfg.Judge.Name = "reflector-judge"
	}
	if reflectorCfg.MaxRetries <= 0 {
		reflectorCfg.MaxRetries = 2
	}
	if executorCfg.MaxTurns <= 0 {
		executorCfg.MaxTurns = 20
	}
	if reflectorCfg.Judge.MaxTurns <= 0 {
		reflectorCfg.Judge.MaxTurns = 5
	}
	if executorCfg.SystemPrompt == "" {
		executorCfg.SystemPrompt = defaultExecutorPrompt
	}
	if reflectorCfg.Judge.SystemPrompt == "" {
		reflectorCfg.Judge.SystemPrompt = defaultReflectorJudgePrompt
	}

	return &reflectiveStep{
		executorConfig:  executorCfg,
		reflectorConfig: reflectorCfg,
	}
}

func (s *reflectiveStep) Run(ctx context.Context, input string) (string, error) {
	executorCfg := s.executorConfig
	maxRetries := s.reflectorConfig.MaxRetries
	judgeCfg := s.reflectorConfig.Judge

	currentInput := input
	var lastOutput string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 1. Execute — run the executor agent with the current input (which
		//    may include critique feedback from a previous attempt).
		execAgent := agentcore.New(executorCfg.ToConfig())
		output, err := execAgent.Run(ctx, currentInput)
		if err != nil {
			lastOutput = output
			continue
		}
		lastOutput = output

		// 2. Judge — evaluate the execution result.
		judgeInput := buildJudgeInput(currentInput, output)
		judgeAgent := agentcore.New(judgeCfg.ToConfig())
		verdictStr, judgeErr := judgeAgent.Run(ctx, judgeInput)

		// Fail-open: if the judge itself fails, accept the current output.
		if judgeErr != nil {
			return output, nil
		}

		result, parseErr := parseVerdict(verdictStr)
		if parseErr != nil {
			return output, nil
		}

		// 3. Pass — output satisfies the plan.
		if result.Pass {
			return output, nil
		}

		// 4. Retry — inject the critique as feedback for the next attempt.
		currentInput = injectCritique(currentInput, result.Critique)
	}

	// Exhausted retries — return the last output (fail-open).
	return lastOutput, nil
}

// reviewerStep runs the final QA review agent.
type reviewerStep struct {
	config AgentRoleConfig
}

func (s *reviewerStep) Run(ctx context.Context, input string) (string, error) {
	agent := agentcore.New(s.config.ToConfig())
	return agent.Run(ctx, input)
}

// ──────────────────────────────────────────────────────────
// NewPER — top-level factory
// ──────────────────────────────────────────────────────────

// NewPER builds a Planner → Executor → Reflector → Reviewer pipeline.
//
// Defaults are applied when PERConfig fields are zero-valued:
//   - Planner.Name → "planner"
//   - Executor.Name → "executor"
//   - Judge.Name → "reflector-judge"
//   - Reviewer.Name → "reviewer"
//   - Reflector.MaxRetries → 2
//   - MaxTurns → 20 for all roles
//   - prompts → built-in defaults
func NewPER(cfg PERConfig) *Pipeline {
	// --- apply defaults ---

	if cfg.Planner.Name == "" {
		cfg.Planner.Name = "planner"
	}
	if cfg.Executor.Name == "" {
		cfg.Executor.Name = "executor"
	}
	if cfg.Reflector.Judge.Name == "" {
		cfg.Reflector.Judge.Name = "reflector-judge"
	}
	if cfg.Reviewer.Name == "" {
		cfg.Reviewer.Name = "reviewer"
	}
	if cfg.Reflector.MaxRetries <= 0 {
		cfg.Reflector.MaxRetries = 2
	}

	setRoleDefaults := func(rc *AgentRoleConfig) {
		if rc.MaxTurns <= 0 {
			rc.MaxTurns = 20
		}
	}
	setRoleDefaults(&cfg.Planner)
	setRoleDefaults(&cfg.Executor)
	setRoleDefaults(&cfg.Reflector.Judge)
	setRoleDefaults(&cfg.Reviewer)

	if cfg.Planner.SystemPrompt == "" {
		cfg.Planner.SystemPrompt = defaultPlannerPrompt
	}
	if cfg.Executor.SystemPrompt == "" {
		cfg.Executor.SystemPrompt = defaultExecutorPrompt
	}
	if cfg.Reflector.Judge.SystemPrompt == "" {
		cfg.Reflector.Judge.SystemPrompt = defaultReflectorJudgePrompt
	}
	if cfg.Reviewer.SystemPrompt == "" {
		cfg.Reviewer.SystemPrompt = defaultReviewerPrompt
	}

	// --- build pipeline ---

	return &Pipeline{
		Steps: []Step{
			// Step 1: Planner — decomposes task into a structured plan.
			NewAgentStep(cfg.Planner.ToConfig()),
			// Step 2: Executor + reflection loop.
			NewReflectiveStep(cfg.Executor, cfg.Reflector),
			// Step 3: Reviewer — final QA gate.
			&reviewerStep{config: cfg.Reviewer},
		},
	}
}

// ──────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────

// buildJudgeInput pairs the original input with the execution output
// for the judge to evaluate.
func buildJudgeInput(originalInput, output string) string {
	var b strings.Builder
	b.WriteString("== 原始任务/计划 ==\n")
	b.WriteString(originalInput)
	b.WriteString("\n\n== 执行结果 ==\n")
	b.WriteString(output)
	b.WriteString("\n\n请评估执行结果是否满足计划要求。")
	return b.String()
}

// injectCritique wraps the input with critique feedback for retry.
func injectCritique(input, critique string) string {
	var b strings.Builder
	b.WriteString(input)
	b.WriteString("\n\n== 评估反馈 ==\n以下是对上次执行结果的评估和改进建议。请根据反馈重新执行：\n")
	b.WriteString(critique)
	return b.String()
}

// parseVerdict extracts the pass/critique verdict from a judge response.
// It searches for the first JSON object in the response, accepting
// JSON embedded in free text (e.g., inside a code block).
func parseVerdict(content string) (*verdict, error) {
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON object found in judge response")
	}

	var v verdict
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return nil, fmt.Errorf("parse verdict JSON: %w", err)
	}
	return &v, nil
}

// extractJSON finds the first top-level JSON object ({...}) in a string.
// It handles nested braces and ignores leading/trailing text.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Try direct parse first.
	if json.Valid([]byte(s)) {
		return s
	}

	// Search for first '{' and match its closing brace, accounting for nesting.
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := s[start : i+1]
				if json.Valid([]byte(candidate)) {
					return candidate
				}
				// Found a matching brace but invalid JSON — look for another
				// top-level object.
				depth = 1 // restart from this opening brace
				start = i
			}
		}
	}

	return ""
}
