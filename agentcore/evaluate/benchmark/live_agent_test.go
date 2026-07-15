package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evaluate"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/tools"
)

// This file implements "product capability" evaluation: unlike
// live_deepseek_test.go which measures a bare LLM (Provider.Complete with no
// tools, no Agent runtime), the tests here drive a full agentcore.Agent so the
// score reflects Mady's actual product value (tool use, multi-turn reasoning).
//
// Three tiers share the same P2A cases and the same fixed sampling seed so
// their pass rates are directly comparable:
//
//   1. TestLiveAgentBaselineEval    — Agent + five-step prompt, NO tools.
//     Sanity check: should match the bare-LLM baseline (TestLiveDeepSeekEval),
//     proving the Agent framework itself introduces no degradation.
//   2. TestLiveAgentWithWorkflowEval — Agent + run_five_step_workflow tool.
//     Measures the gain from structured five-step reasoning.
//   3. TestLiveAgentWithPatentToolsEval — Agent + patent/scholar retrieval tools.
//     Measures the gain from external prior-art retrieval on novelty/inventive
//     questions (requires the nuo-patent CLI and network access).
//
// P2B (invalidation decisions) is intentionally excluded — it is frozen due to
// empty-shell inputs and a degenerate conclusion distribution. See the P2B
// FROZEN note on TestLiveDeepSeekInvalidationEval and suite.ValidCases.

// evalAgentCaseCount controls how many P2A cases each tier evaluates. Start
// small (3) to validate the pipeline cheaply; bump to 10 once the chain is
// confirmed working. Override at runtime via MADY_EVAL_CASES.
func evalAgentCaseCount(t *testing.T) int {
	t.Helper()
	if v := os.Getenv("MADY_EVAL_CASES"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 && n <= 31 {
			return n
		}
	}
	return 3
}

// caseTypeFromExamID infers the reasoning CaseType from a P2A exam case ID.
// P2A IDs follow the pattern patent_exam_YYYY_aXX_NN where aXX encodes the
// primary patent-law article under examination.
//
// IMPORTANT LESSON (2026-07-15, learned empirically): the mapping must reflect
// what the EXAM tests, not what the article's real-world workflow is. P2A exam
// questions are analysis/judgment tasks ("analyze unity", "judge amendment
// legality"), NOT full procedural tasks ("draft a complete claim set",
// "file an invalidation request"). Mapping A31→drafting caused the 5-step
// drafting manifest to make the agent write a full claim set instead of
// analyzing unity — scores dropped sharply (e.g. 2019_a31_03: 0.93→0.40).
//
// Therefore ALL P2A articles map to patentability (whose template does
// evidence-based analysis: parse → search → compare → multi-hypothesis
// evaluate → conclude). The drafting/invalidation manifests remain available
// for real-case scenarios (a user genuinely drafting claims or filing
// invalidation) but are NOT used for exam questions.
func caseTypeFromExamID(caseID string) reasoning.CaseType {
	switch {
	case containsArt(caseID, "a22"):
		// Article 22 — novelty / inventive step. patentability's template adds
		// a multi_hypothesis inventive-step stage, matching the A22 exam focus.
		return reasoning.CasePatentability
	case containsArt(caseID, "a2"):
		// Article 2 — patent-eligible subject matter (an analysis question).
		return reasoning.CasePatentability
	case containsArt(caseID, "a26"):
		// Article 26 — sufficiency of disclosure / support / clarity.
		return reasoning.CasePatentability
	case containsArt(caseID, "a31"), containsArt(caseID, "r42"):
		// Article 31 / Rule 42 — unity / divisional. Exam tests unity ANALYSIS
		// (do claims share a specific technical feature?), NOT full claim
		// drafting. patentability's evidence-based analysis fits better than
		// drafting's full-claim-writing flow.
		return reasoning.CasePatentability
	case containsArt(caseID, "a33"):
		// Article 33 — amendment scope. Exam tests whether an amendment exceeds
		// the original scope (an analysis question), NOT the full invalidation
		// procedure. patentability fits; invalidation is for real-case filing.
		return reasoning.CasePatentability
	default:
		return reasoning.CasePatentability
	}
}

// containsArt checks whether caseID contains the given article tag as a
// distinct segment (e.g. "a2" in "patent_exam_2018_a2_01"). It matches the tag
// surrounded by underscores or at the ID end to avoid "a2" matching "a22".
func containsArt(caseID, art string) bool {
	return strings.Contains(caseID, "_"+art+"_") || strings.HasSuffix(caseID, "_"+art)
}

// toolFactory builds per-case tools. Tiers whose tools depend on the case
// (e.g. the workflow tier, which selects a CaseType per question) supply a
// factory; tiers with fixed tools pass nil and use the pre-assembled set.
type toolFactory func(caseID string) []*agentcore.Tool

// agentRunFunc builds a RunFunc that drives a fresh agentcore.Agent per call.
// A new agent is constructed for every case to avoid cross-case state leakage
// (memory/context compaction from one case must not influence the next).
//
// tools may be nil (baseline tier) or pre-assembled (workflow/patent tiers).
// MaxTurns is raised to 20 since P2A inputs average ~983 chars and five-step
// reasoning may span several tool-call turns.
func agentRunFunc(env *deepSeekTestEnv, agentTools []*agentcore.Tool, sysPrompt string) evaluate.RunFunc {
	return func(ctx context.Context, input string) (string, error) {
		cfg := agentcore.Config{
			ModelConfig: agentcore.ModelConfig{
				Name:      "eval-agent",
				Model:     env.Model,
				Provider:  env.Provider,
				Streaming: true,
			},
			SystemPrompt: sysPrompt,
			Tools:        agentTools,
			ExecutionConfig: agentcore.ExecutionConfig{
				MaxTurns:          20,
				ExecutionMode:     agentcore.ModeSerial,
				ValidateArguments: true,
			},
			CompactionConfig: agentcore.CompactionConfig{
				ContextWindow:    128000,
				ReserveTokens:    32000,
				KeepRecentTokens: 4000,
			},
		}
		agent := agentcore.New(cfg)
		defer agent.Close()
		return agent.Run(ctx, input)
	}
}

// toolCallCounter wraps a set of tools so that each tool's invocations are
// counted via an atomic counter. It returns the wrapped tools plus a snapshot
// function for logging. This gives product-capability evaluation visibility
// into whether the agent actually used the tools it was given.
type toolCallCounter struct {
	counts map[string]*atomic.Int64
}

func newToolCallCounter(agentTools []*agentcore.Tool) (*toolCallCounter, []*agentcore.Tool) {
	if len(agentTools) == 0 {
		return nil, agentTools
	}
	c := &toolCallCounter{counts: make(map[string]*atomic.Int64, len(agentTools))}
	wrapped := make([]*agentcore.Tool, 0, len(agentTools))
	for _, tl := range agentTools {
		if tl == nil {
			continue
		}
		name := tl.Name
		counter := &atomic.Int64{}
		c.counts[name] = counter
		wrapped = append(wrapped, wrapToolWithCounter(tl, counter))
	}
	return c, wrapped
}

// wrapToolWithCounter returns a shallow copy of tl whose Func increments the
// counter before delegating to the original Func. All other fields (Name,
// Parameters, ReadOnly, ...) are preserved so the tool schema is unchanged.
func wrapToolWithCounter(tl *agentcore.Tool, counter *atomic.Int64) *agentcore.Tool {
	dup := *tl
	orig := tl.Func
	dup.Func = func(ctx context.Context, args json.RawMessage) (any, error) {
		counter.Add(1)
		return orig(ctx, args)
	}
	return &dup
}

// snapshot returns "tool=count" pairs for logging.
func (c *toolCallCounter) snapshot() string {
	if c == nil {
		return "(no tools)"
	}
	parts := make([]string, 0, len(c.counts))
	for name, ct := range c.counts {
		parts = append(parts, fmt.Sprintf("%s=%d", name, ct.Load()))
	}
	if len(parts) == 0 {
		return "(no calls)"
	}
	return joinCounts(parts)
}

// runAgentLiveEval mirrors runLiveEval's structure but drives an Agent-backed
// RunFunc instead of a bare-Provider cache lookup. Predictions are cached per
// (tier, case) so a partial run can be resumed and re-scoring does not re-bill
// the LLM. Tool-call counts are logged per case for observability.
func runAgentLiveEval(t *testing.T, env *deepSeekTestEnv, cases []evaluate.TestCase, cachePath, systemPrompt string, agentTools []*agentcore.Tool) {
	runAgentLiveEvalWithFactory(t, env, cases, cachePath, systemPrompt, agentTools, nil)
}

// runAgentLiveEvalWithFactory is like runAgentLiveEval but allows per-case
// tool construction via factory. When factory is non-nil, it is called for
// each un-cached case to build the tool set (overriding agentTools); this is
// used by the workflow tier to select a CaseType-matched runner per question.
func runAgentLiveEvalWithFactory(t *testing.T, env *deepSeekTestEnv, cases []evaluate.TestCase, cachePath, systemPrompt string, agentTools []*agentcore.Tool, factory toolFactory) {
	t.Helper()
	if len(cases) == 0 {
		t.Fatal("no cases to evaluate")
	}

	// For the fixed-tools path, wrap once upfront. For the factory path,
	// wrapping happens per-case inside the loop.
	fixedCounter, fixedTools := newToolCallCounter(agentTools)
	cache := loadCache(cachePath)
	missing := false
	for i, c := range cases {
		if pred, ok := cache[c.ID]; ok && pred != "" {
			t.Logf("(%d/%d) %s loaded from cache (len=%d)", i+1, len(cases), c.ID, len(pred))
			continue
		}
		missing = true

		// Select tools for this case: factory (per-case) or fixed set.
		var runTools []*agentcore.Tool
		var counter *toolCallCounter
		if factory != nil {
			counter, runTools = newToolCallCounter(factory(c.ID))
		} else {
			counter, runTools = fixedCounter, fixedTools
		}
		t.Logf("(%d/%d) running Agent for %s ...", i+1, len(cases), c.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		run := agentRunFunc(env, runTools, systemPrompt)
		out, err := run(ctx, c.Input)
		cancel()
		if err != nil {
			t.Errorf("case %s: %v", c.ID, err)
			cache[c.ID] = ""
			continue
		}
		cache[c.ID] = out
		saveCache(cachePath, cache)
		t.Logf("(%d/%d) %s done (len=%d) tools[%s]", i+1, len(cases), c.ID, len(out), counter.snapshot())
	}
	if missing {
		saveCache(cachePath, cache)
	}

	// Score with the same LiveEvaluator used by the bare-LLM baseline so the
	// two tiers are directly comparable.
	report, err := LiveEvaluator(env.Provider, env.Model).EvaluateBatch(context.Background(), cases, func(ctx context.Context, input string) (string, error) {
		for _, c := range cases {
			if c.Input == input {
				return cache[c.ID], nil
			}
		}
		return "", fmt.Errorf("no prediction cached for input")
	})
	if err != nil {
		t.Fatalf("EvaluateBatch: %v", err)
	}

	t.Logf("Total cases: %d", report.TotalCases)
	t.Logf("Passed: %d", report.PassedCases)
	t.Logf("Pass rate: %.2f", report.PassRate)
	for name, score := range report.AggregateScores {
		t.Logf("  metric %s mean: %.3f", name, score)
	}
	for _, r := range report.Results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		t.Logf("[%s] %s avg=%.3f scores=%v", status, r.CaseID, r.Average, r.Scores)
	}
	if report.PassRate < 1.0 {
		t.Logf("Report markdown:\n%s", evaluate.FormatReport(report))
	}
}

func joinCounts(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// TestLiveAgentBaselineEval runs the Agent framework WITHOUT any tools against
// P2A cases. Its pass rate should closely track the bare-LLM baseline
// (TestLiveDeepSeekEval) — a significant drop would indicate the framework
// itself degrades output quality, which must be fixed before trusting the
// higher tiers.
func TestLiveAgentBaselineEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := randomCases(t, PatentExamRealCases(), evalAgentCaseCount(t), 20241201)
	t.Logf("Baseline tier: %d cases (seed 20241201)", len(cases))
	for i, c := range cases {
		t.Logf("  selected %d: %s", i+1, c.ID)
	}
	cachePath := filepath.Join(os.TempDir(), "mady_agent_baseline_eval.json")
	runAgentLiveEval(t, env, cases, cachePath, patentExamSystemPrompt, nil)
}

// TestLiveAgentWithWorkflowEval equips the Agent with the
// run_five_step_workflow tool (fact blackboard → rule retrieval → plan →
// reasoning → conclusion). The retriever and LlmClient are wired to the
// DeepSeek provider; with no knowledge graph the Stage ② rule retrieval
// degrades gracefully (no-op), which is the intended behavior.
//
// Unlike the initial version which fixed caseType to novelty_search for all
// questions (causing framework mismatch on non-novelty questions), this test
// now infers the CaseType from each case's exam ID so the five-step tool uses
// the closest matching reasoning template (e.g. patentability for A22
// inventive-step questions).
func TestLiveAgentWithWorkflowEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := randomCases(t, PatentExamRealCases(), evalAgentCaseCount(t), 20241201)
	t.Logf("Workflow tier: %d cases (seed 20241201)", len(cases))

	llm := reasoning.NewLlmClientFromProvider(env.Provider, env.Model)

	// Build a per-case workflow tool whose runner uses the CaseType inferred
	// from the exam question's primary patent-law article.
	factory := func(caseID string) []*agentcore.Tool {
		ct := caseTypeFromExamID(caseID)
		runner := reasoning.NewWorkflowRunner(
			caseID, // caseID: use the exam case ID for traceability
			ct,     // caseType: inferred from article (a2/a22/a26/a31/a33/r42)
			"",     // techField
			nil,    // retriever: nil → skip Stage ② (no KG available)
			llm,    // llm for planner/checker
		)
		return []*agentcore.Tool{reasoning.AsWorkflowTool(runner)}
	}

	// Log the CaseType mapping once for visibility.
	for _, c := range cases {
		t.Logf("  %s → CaseType=%s", c.ID, caseTypeFromExamID(c.ID))
	}

	cachePath := filepath.Join(os.TempDir(), "mady_agent_workflow_eval.json")
	runAgentLiveEvalWithFactory(t, env, cases, cachePath, patentExamSystemPrompt, nil, factory)
}

// TestLiveAgentP2BBaselineEval runs the Agent framework (no tools) against
// P2B real invalidation-decision cases. Unlike P2A exam questions, P2B cases
// contain full case facts (claim 1 + evidence + invalidation grounds), making
// this a real-case evaluation of the Agent's analytical capability.
func TestLiveAgentP2BBaselineEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := randomCases(t, InvalidationDecisionCases, evalAgentCaseCount(t), 20241201)
	t.Logf("P2B baseline tier: %d cases (seed 20241201)", len(cases))
	cachePath := filepath.Join(os.TempDir(), "mady_p2b_agent_baseline_eval.json")
	runAgentLiveEval(t, env, cases, cachePath, invalidationSystemPrompt, nil)
}

// TestLiveAgentP2BWorkflowEval equips the Agent with the run_five_step_workflow
// tool (CaseInvalidation manifest) for P2B real invalidation cases. This is
// the key test for validating the invalidation manifest's value — P2A exam
// questions could not exercise it (they map to patentability), but P2B real
// invalidation decisions are exactly the invalidation manifest's target scenario.
func TestLiveAgentP2BWorkflowEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := randomCases(t, InvalidationDecisionCases, evalAgentCaseCount(t), 20241201)
	t.Logf("P2B workflow tier: %d cases (seed 20241201)", len(cases))

	llm := reasoning.NewLlmClientFromProvider(env.Provider, env.Model)

	// All P2B cases are invalidation decisions → use CaseInvalidation manifest.
	factory := func(caseID string) []*agentcore.Tool {
		runner := reasoning.NewWorkflowRunner(
			caseID,                     // caseID
			reasoning.CaseInvalidation, // caseType: invalidation manifest (5 steps)
			"",                         // techField
			nil,                        // retriever: nil → skip Stage ②
			llm,                        // llm for planner/checker
		)
		return []*agentcore.Tool{reasoning.AsWorkflowTool(runner)}
	}

	cachePath := filepath.Join(os.TempDir(), "mady_p2b_agent_workflow_eval.json")
	runAgentLiveEvalWithFactory(t, env, cases, cachePath, invalidationSystemPrompt, nil, factory)
}

// TestLiveAgentP2BPromptAugmentedEval uses the invalidation manifest's specific
// step descriptions as a structured system prompt (NOT as an external tool).
//
// Empirical finding: L2 (tool-call orchestration, 0.334) < L1 (generic prompt,
// 0.513). This test checks whether manifest-guided prompt augmentation (L4)
// outperforms L1's generic five-step prompt. The hypothesis: for LLM Agents,
// precise prompt guidance > external step orchestration > generic prompt.
func TestLiveAgentP2BPromptAugmentedEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := randomCases(t, InvalidationDecisionCases, evalAgentCaseCount(t), 20241201)
	t.Logf("P2B prompt-augmented tier: %d cases (seed 20241201)", len(cases))

	// Generate a structured prompt from the invalidation manifest's steps.
	var manifest *reasoning.WorkflowManifest
	for _, m := range reasoning.DefaultManifests() {
		if m.CaseType == reasoning.CaseInvalidation {
			manifest = m
			break
		}
	}
	if manifest == nil {
		t.Fatal("invalidation manifest not found")
	}
	augmentedPrompt := reasoning.ManifestToSystemPrompt(manifest)
	t.Logf("augmented prompt:\n%s", augmentedPrompt)

	cachePath := filepath.Join(os.TempDir(), "mady_p2b_prompt_augmented_eval.json")
	runAgentLiveEval(t, env, cases, cachePath, augmentedPrompt, nil)
}

// mockHumanRevision simulates a human expert revising the Agent's draft. It is
// NOT a direct copy of the reference (that would be cheating) — instead it
// models how a patent attorney would correct the draft: fix wrong conclusions,
// add missing legal citations, strengthen weak reasoning. The revision prompt
// shows the expert the reference answer (as a "model answer" they know) and
// asks them to revise the draft toward it while preserving the draft's
// structure where it's already correct.
//
// This models the L5 HITL tier: Agent自主推理产出初稿 → 人工审阅修订 → 终稿。
// The score gap between L1 (draft) and L5 (revised) estimates the theoretical
// ceiling of human-in-the-loop value.
func mockHumanRevision(ctx context.Context, provider agentcore.Provider, model, draft, reference string) (string, error) {
	prompt := fmt.Sprintf(`你是一位资深专利代理人，正在审阅一份 AI 生成的无效宣告分析初稿。
请像人类专家一样修订这份初稿：纠正错误的结论、补充遗漏的法条引用、增强薄弱的推理。
保留初稿中正确的部分，不要完全重写。

参考答案（你作为专家知道的正确结论）：
%s

AI 初稿：
%s

请输出修订后的完整答案：`, truncForPrompt(reference, 2000), truncForPrompt(draft, 4000))

	resp, err := provider.Complete(ctx, &agentcore.ProviderRequest{
		Model: model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: "你是资深专利代理人，负责审阅和修订 AI 产出的法律分析。修订时保持专业准确，纠正错误但不改变正确结论。"},
			{Role: agentcore.RoleUser, Content: prompt},
		},
		Temperature: 0.01,
		MaxTokens:   2048,
	})
	if err != nil {
		return draft, err // fallback to draft on error
	}
	if resp == nil || resp.Content == "" {
		return draft, nil
	}
	return resp.Content, nil
}

// TestLiveAgentP2BHitlEval measures the L5 human-in-the-loop tier: the Agent
// produces a draft (same as L1), then a mock human expert revises it, and the
// revised version is scored. The L1→L5 gap estimates the theoretical ceiling
// of HITL value — how much better could the output be with expert revision.
//
// This is the key test for answering "should the five-step workflow be combined
// with human collaboration?" If L5 >> L1, HITL has clear value and the
// architecture should invest in real HITL (connecting RecordDecision, etc).
func TestLiveAgentP2BHitlEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := randomCases(t, InvalidationDecisionCases, evalAgentCaseCount(t), 20241201)
	t.Logf("P2B HITL tier: %d cases (seed 20241201)", len(cases))

	// Step 1: Generate Agent drafts (same as L1, cached separately).
	draftCachePath := filepath.Join(os.TempDir(), "mady_p2b_hitl_draft_eval.json")
	draftCache := loadCache(draftCachePath)
	draftMissing := false
	for i, c := range cases {
		if pred, ok := draftCache[c.ID]; ok && pred != "" {
			t.Logf("(%d/%d) %s draft loaded from cache (len=%d)", i+1, len(cases), c.ID, len(pred))
			continue
		}
		draftMissing = true
		t.Logf("(%d/%d) generating draft for %s ...", i+1, len(cases), c.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		run := agentRunFunc(env, nil, invalidationSystemPrompt)
		out, err := run(ctx, c.Input)
		cancel()
		if err != nil {
			t.Errorf("draft %s: %v", c.ID, err)
			draftCache[c.ID] = ""
			continue
		}
		draftCache[c.ID] = out
		saveCache(draftCachePath, draftCache)
	}
	if draftMissing {
		saveCache(draftCachePath, draftCache)
	}

	// Step 2: Mock human revision for each case.
	revisedCachePath := filepath.Join(os.TempDir(), "mady_p2b_hitl_revised_eval.json")
	revisedCache := loadCache(revisedCachePath)
	revisedMissing := false
	for i, c := range cases {
		if rev, ok := revisedCache[c.ID]; ok && rev != "" {
			t.Logf("(%d/%d) %s revision loaded from cache (len=%d)", i+1, len(cases), c.ID, len(rev))
			continue
		}
		revisedMissing = true
		draft := draftCache[c.ID]
		if draft == "" {
			continue
		}
		t.Logf("(%d/%d) revising %s ...", i+1, len(cases), c.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		revised, err := mockHumanRevision(ctx, env.Provider, env.Model, draft, c.Expected)
		cancel()
		if err != nil {
			t.Errorf("revision %s: %v", c.ID, err)
			revisedCache[c.ID] = draft // fallback to draft
			continue
		}
		revisedCache[c.ID] = revised
		saveCache(revisedCachePath, revisedCache)
		t.Logf("(%d/%d) %s revised (len=%d → %d)", i+1, len(cases), c.ID, len(draft), len(revised))
	}
	if revisedMissing {
		saveCache(revisedCachePath, revisedCache)
	}

	// Step 3: Score the revised versions.
	report, err := LiveEvaluator(env.Provider, env.Model).EvaluateBatch(context.Background(), cases, func(ctx context.Context, input string) (string, error) {
		for _, c := range cases {
			if c.Input == input {
				return revisedCache[c.ID], nil
			}
		}
		return "", fmt.Errorf("no revision cached for input")
	})
	if err != nil {
		t.Fatalf("EvaluateBatch: %v", err)
	}

	t.Logf("Total cases: %d", report.TotalCases)
	t.Logf("Passed: %d", report.PassedCases)
	t.Logf("Pass rate: %.2f", report.PassRate)
	for name, score := range report.AggregateScores {
		t.Logf("  metric %s mean: %.3f", name, score)
	}
	for _, r := range report.Results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		t.Logf("[%s] %s avg=%.3f scores=%v", status, r.CaseID, r.Average, r.Scores)
	}
	if report.PassRate < 1.0 {
		t.Logf("Report markdown:\n%s", evaluate.FormatReport(report))
	}
}

// TestLiveAgentWithPatentToolsEval equips the Agent with prior-art retrieval
// tools (patent_lookup, patent_legal, scholar_search). This measures the gain
// from external retrieval on novelty/inventive-step questions. It requires the
// nuo-patent CLI (set NUO_PATENT_PATH, e.g. "node /path/to/nuo-patent/dist/cli.js")
// and network access to Semantic Scholar; missing dependencies cause the tools
// to return errors which the agent is expected to handle gracefully (its score
// then reflects degraded retrieval).
func TestLiveAgentWithPatentToolsEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	if os.Getenv("MADY_EVAL_PATENT_TOOLS") != "1" {
		t.Skip("set MADY_EVAL_PATENT_TOOLS=1 to run the patent-tools tier (requires NUO_PATENT_PATH + network)")
	}
	cases := randomCases(t, PatentExamRealCases(), evalAgentCaseCount(t), 20241201)
	t.Logf("Patent-tools tier: %d cases (seed 20241201)", len(cases))

	// PatentToolConfigDefaults reads NUO_PATENT_PATH (falling back to the bare
	// "nuo-patent" name); an empty config would not resolve the local build.
	patentTools := tools.BuildTools(tools.ExtensionConfig{
		ScholarSearch: &tools.ScholarSearchConfig{},
		PatentTool:    tools.PatentToolConfigDefaults(),
	})
	var agentTools []*agentcore.Tool
	for _, tl := range patentTools {
		if tl == nil {
			continue
		}
		// Keep read-only retrieval tools only.
		if tl.ReadOnly {
			agentTools = append(agentTools, tl)
		}
	}
	if len(agentTools) == 0 {
		t.Skip("no patent/scholar tools available — check tools.BuildTools")
	}
	t.Logf("patent tools armed: %d", len(agentTools))

	cachePath := filepath.Join(os.TempDir(), "mady_agent_patent_eval.json")
	runAgentLiveEval(t, env, cases, cachePath, patentExamSystemPrompt, agentTools)
}

// stubProvider is a minimal agentcore.Provider that returns a fixed response
// without any network call. It is used by the offline wiring smoke test below
// to validate that the three evaluation tiers can be assembled and driven
// end-to-end inside CI (no API key required).
type stubProvider struct{ reply string }

func (s *stubProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{Content: s.reply, FinishReason: "stop"}, nil
}

func (s *stubProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Content: s.reply, Done: true}
	close(ch)
	return ch, nil
}

// TestAgentWiringSmoke is an OFFLINE test (no MADY_LIVE_EVAL gate) that
// verifies the assembly of all three evaluation tiers works end-to-end with a
// stub provider. It guards against future refactors silently breaking the
// Config construction, tool injection, or toolCallCounter wiring that the live
// tiers depend on. It must NOT make any network call.
func TestAgentWiringSmoke(t *testing.T) {
	env := &deepSeekTestEnv{
		Provider: &stubProvider{reply: "smoke: 抽象答案"},
		Model:    "stub-model",
	}
	// A single P2A case is enough to exercise the wiring.
	cases := randomCases(t, PatentExamRealCases(), 1, 20241201)

	t.Run("baseline_no_tools", func(t *testing.T) {
		run := agentRunFunc(env, nil, patentExamSystemPrompt)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := run(ctx, cases[0].Input)
		if err != nil {
			t.Fatalf("baseline agentRunFunc: %v", err)
		}
		if out == "" {
			t.Fatal("baseline produced empty output")
		}
		t.Logf("baseline output len: %d", len(out))
	})

	t.Run("workflow_tool_assembly", func(t *testing.T) {
		// Workflow runner requires an LlmClient; the stub provider must be
		// adaptable. Retrieval is nil (graceful Stage ② skip).
		llm := reasoning.NewLlmClientFromProvider(env.Provider, env.Model)
		runner := reasoning.NewWorkflowRunner(
			"smoke-case", reasoning.CaseNoveltySearch, "", nil, llm,
		)
		wfTool := reasoning.AsWorkflowTool(runner)
		if wfTool == nil || wfTool.Name == "" {
			t.Fatal("AsWorkflowTool returned an empty tool")
		}
		// Wrap with counter and assert counting works on direct invocation.
		counter, wrapped := newToolCallCounter([]*agentcore.Tool{wfTool})
		if counter == nil {
			t.Fatal("newToolCallCounter returned nil counter for non-empty tools")
		}
		if len(wrapped) != 1 {
			t.Fatalf("expected 1 wrapped tool, got %d", len(wrapped))
		}
		// Driving agentRunFunc should construct an agent without error. We do
		// not assert the agent calls the tool (that depends on the stub model
		// emitting tool_calls, which it does not) — only that assembly works.
		run := agentRunFunc(env, wrapped, patentExamSystemPrompt)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := run(ctx, cases[0].Input); err != nil {
			t.Fatalf("workflow agentRunFunc: %v", err)
		}
	})

	t.Run("patent_tools_assembly", func(t *testing.T) {
		patentTools := tools.BuildTools(tools.ExtensionConfig{
			ScholarSearch: &tools.ScholarSearchConfig{},
			PatentTool:    &tools.PatentToolConfig{},
		})
		var readOnly []*agentcore.Tool
		for _, tl := range patentTools {
			if tl != nil && tl.ReadOnly {
				readOnly = append(readOnly, tl)
			}
		}
		if len(readOnly) == 0 {
			t.Fatal("expected at least one read-only patent/scholar tool")
		}
		counter, wrapped := newToolCallCounter(readOnly)
		if counter == nil {
			t.Fatal("newToolCallCounter returned nil counter")
		}
		// Counting wrapper must invoke the underlying Func and increment.
		before := counter.counts[readOnly[0].Name].Load()
		// Tools may error (no network) — that's fine; we only assert the
		// counter incremented when Func was invoked.
		_, _ = wrapped[0].Func(context.Background(), json.RawMessage(`{}`))
		after := counter.counts[readOnly[0].Name].Load()
		if after != before+1 {
			t.Fatalf("counter did not increment: before=%d after=%d", before, after)
		}
	})
}

// truncForPrompt truncates a string for inclusion in an LLM prompt, keeping
// the head and adding an ellipsis marker if truncated.
func truncForPrompt(s string, maxRunes int) string {
	if len([]rune(s)) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "\n...（略）"
}
