package reasoning

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// FiveStepRunner orchestrates the five-step reasoning workflow.
//
// Phase 3 (current): Full Stage ① → ② → ③ → ④ → ⑤ with configurable
// WorkflowManifest and enhanced syllogism checking.
//
// The runner implements agentcore.Step, so it can be injected into an
// agent config as the primary execution step.
type FiveStepRunner struct {
	planner      *Planner
	compiler     *PlanCompiler
	retriever    *MultiSourceRetriever
	checker      *EnhancedSyllogismChecker
	manifest     *WorkflowManifest
	ruleManifest *RuleRetrievalManifest
	bb           *FactBlackboard
	// Stage ① config.
	stage1Collectors []FactCollectorFunc
	// requireConfirmation gates Stage ② → ③ with a human-confirmation interrupt.
	// When true, runStage2 returns an InterruptError after retrieval so the
	// caller can save a checkpoint and let the user confirm/edit the rule set
	// before Plan/Execute/Check consume it.
	requireConfirmation bool
}

// FactCollectorFunc is a function that implements Stage ① fact collection.
// It receives the input and blackboard, and returns a CollectResult.
// This avoids circular imports (the concrete collectors live in the collector/ package).
type FactCollectorFunc func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error)

// FiveStepRunnerConfig configures the runner.
type FiveStepRunnerConfig struct {
	Planner      *Planner
	NodeBuilder  NodeBuilder               // nil = noop builder (testing)
	Retriever    *MultiSourceRetriever     // nil = skip Stage ②
	Checker      *EnhancedSyllogismChecker // nil = pass-through check (Phase 1 mode)
	Manifest     *WorkflowManifest         // nil = use defaults per CaseType
	Collectors   []FactCollectorFunc       // nil = skip Stage ① (use seedBlackboard)
	RuleManifest *RuleRetrievalManifest    // nil = use default based on CaseType
	// RequireRuleConfirmation, when true, pauses after Stage ② rule retrieval
	// for human confirmation before proceeding to Plan (Stage ③). The pause is
	// implemented as an InterruptError carrying the retrieved rule count, so the
	// tool layer can save a checkpoint and let the user confirm.
	RequireRuleConfirmation bool
	CaseID                  string
	CaseType                CaseType
	TechField               string
}

// NewFiveStepRunner creates a runner for the five-step workflow.
func NewFiveStepRunner(cfg FiveStepRunnerConfig) *FiveStepRunner {
	if cfg.Planner == nil {
		cfg.Planner = NewPlanner(nil)
	}
	r := &FiveStepRunner{
		planner:             cfg.Planner,
		compiler:            NewPlanCompiler(cfg.NodeBuilder),
		retriever:           cfg.Retriever,
		checker:             cfg.Checker,
		manifest:            cfg.Manifest,
		ruleManifest:        cfg.RuleManifest,
		bb:                  NewFactBlackboard(cfg.CaseID, cfg.CaseType, cfg.TechField),
		stage1Collectors:    cfg.Collectors,
		requireConfirmation: cfg.RequireRuleConfirmation,
	}
	// Register manifest's Plan steps as Planner template if provided.
	if cfg.Manifest != nil {
		plan := cfg.Manifest.Stage4.ToPlan(cfg.CaseType)
		cfg.Planner.RegisterTemplate(cfg.CaseType, PlanIntentChain, *plan)
	}
	return r
}

// SetRequireRuleConfirmation toggles the Stage ② confirmation gate. When
// enabled, runStage2 interrupts after retrieval so the caller can save a
// checkpoint and let the user confirm the rule set before Plan/Execute/Check.
func (r *FiveStepRunner) SetRequireRuleConfirmation(enabled bool) {
	r.requireConfirmation = enabled
}

// Run executes the full five-step reasoning workflow from Stage ①.
func (r *FiveStepRunner) Run(ctx context.Context, input string) (string, error) {
	return r.runFrom(ctx, input, 1)
}

// runStage1 executes fact collection. If no collectors are configured,
// it falls back to the legacy seedBlackboard behavior.
func (r *FiveStepRunner) runStage1(ctx context.Context, input string) error {
	if len(r.stage1Collectors) == 0 {
		r.seedBlackboard(input)
		return nil
	}

	for _, c := range r.stage1Collectors {
		result, err := c(ctx, input, r.bb)
		if err != nil {
			return fmt.Errorf("collector: %w", err)
		}
		_ = result
	}

	// Store Stage ① output on blackboard.
	r.bb.SetStageOutput("stage1", map[string]int{
		"total_facts": len(r.bb.ActiveFacts()),
	})
	return nil
}

// runStage2 executes rule retrieval via MultiSourceRetriever.
// If no retriever is configured, it is a no-op.
func (r *FiveStepRunner) runStage2(ctx context.Context) error {
	if r.retriever == nil {
		return nil
	}

	rm := defaultRuleManifest(r.bb.CaseType)
	if r.ruleManifest != nil {
		rm = *r.ruleManifest
	}
	rules, err := r.retriever.Retrieve(ctx, rm, r.bb.ActiveFacts(), r.bb.TechnicalField)
	if err != nil {
		return err
	}

	// Write retrieved rules as RuleConstraints on the blackboard.
	for _, rule := range rules {
		r.bb.AddRuleConstraint(rule.Rule)
	}

	r.bb.SetStageOutput("stage2", map[string]int{
		"total_rules": len(rules),
	})

	// Confirmation gate: pause for human review of the retrieved rule set
	// before Plan/Execute/Check consume it. The interrupt carries the rule
	// count so the tool layer can surface a confirmation prompt and save a
	// checkpoint for later resumption at Stage ③.
	if r.requireConfirmation && len(rules) > 0 {
		return agentcore.NewInterruptErrorWithData(
			fmt.Sprintf("规则检索完成（%d 条），请人工确认规则集后继续", len(rules)),
			map[string]any{
				"gate":        "rule_confirmation",
				"stage":       2,
				"total_rules": len(rules),
				"case_id":     r.bb.CaseID,
			},
		)
	}
	return nil
}

// defaultRuleManifest returns a minimal manifest for the given case type.
func defaultRuleManifest(ct CaseType) RuleRetrievalManifest {
	return RuleRetrievalManifest{
		ManifestID: string(ct) + "_default",
		CaseType:   ct,
		Name:       string(ct) + " 默认规则源",
		Sources: []RuleSourceCfg{
			{Source: RuleSourceKG, MaxPerSource: 10, Weight: 1.0},
			{Source: RuleSourceVector, MaxPerSource: 5, Weight: 0.8},
		},
		Aggregation: "merge",
		MaxRules:    15,
	}
}

// seedBlackboard adds a user-input fact from the input string.
func (r *FiveStepRunner) seedBlackboard(input string) {
	r.bb.AddFact(FactEntry{
		ID:          fmt.Sprintf("fact_%s_input", r.bb.CaseID),
		Source:      string(CollectorUserInput),
		Content:     input,
		Confidence:  1.0,
		ExtractedAt: nowISO(),
		CollectorID: CollectorUserInput,
		Category:    FactCategoryTechnical,
	})
}

// passThroughCheck creates a minimal pass-through CheckReport.
func (r *FiveStepRunner) passThroughCheck(plan *Plan) CheckReport {
	return CheckReport{
		PlanID:     plan.PlanID,
		Passed:     true,
		UsedFacts:  plan.UsedFacts,
		UsedRules:  plan.UsedRules,
		Confidence: 0.7,
	}
}

// formatResult produces a human-readable workflow result.
func (r *FiveStepRunner) formatResult(plan *Plan, report CheckReport) (string, error) {
	var sb strings.Builder
	sb.WriteString("## 五步工作法执行结果\n\n")
	fmt.Fprintf(&sb, "**案件ID**: %s\n", r.bb.CaseID)
	fmt.Fprintf(&sb, "**案件类型**: %s\n", r.bb.CaseType)
	fmt.Fprintf(&sb, "**校验通过**: %v\n", report.Passed)
	fmt.Fprintf(&sb, "**置信度**: %.0f%%\n\n", report.Confidence*100)

	// Output actual analysis content (from LLMNodeBuilder) if available.
	// This is the substance that formatResult previously lacked — without it,
	// the tool only output step names and JSON metadata, providing no value.
	analysisAny, ok := r.bb.StageOutput("analysis")
	analysis, _ := analysisAny.(string)
	if ok && analysis != "" {
		sb.WriteString("### 分析过程\n")
		sb.WriteString(analysis)
		sb.WriteString("\n\n")
	} else {
		// Fallback: list step descriptions (noop builder or no LLM).
		sb.WriteString("### 执行步骤\n\n")
		for _, step := range plan.Steps {
			fmt.Fprintf(&sb, "- **步骤 %d** (%s): %s\n", step.Order, step.Strategy, step.Description)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### 校验报告\n")
	fmt.Fprintf(&sb, "- 校验通过: %v\n", report.Passed)
	fmt.Fprintf(&sb, "- 置信度: %.0f%%\n", report.Confidence*100)

	return sb.String(), nil
}

// detectPlanIntent picks a PlanIntent based on the blackboard state.
func detectPlanIntent(bb *FactBlackboard) PlanIntent {
	switch bb.CaseType {
	case CasePatentability, CaseInvalidation:
		// Creative step / invalidation often requires multi-hypothesis debate.
		return PlanIntentChain // Phase 1: use chain; Phase 4: promote to multi_hypothesis
	case CaseNoveltySearch, CaseFTO, CaseValidity:
		return PlanIntentChain // Deterministic comparison.
	case CaseRejection, CaseReexamination:
		return PlanIntentChain // Structured response to office actions.
	default:
		return PlanIntentSimple
	}
}
