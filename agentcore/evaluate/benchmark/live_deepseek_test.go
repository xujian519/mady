package benchmark

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evaluate"
	"github.com/xujian519/mady/provider/chatcompat"
)

// deepSeekTestEnv 保存 DeepSeek live 测试的公共环境参数。
type deepSeekTestEnv struct {
	Provider agentcore.Provider
	Model    string
}

// newDeepSeekTestEnv 从环境变量读取 API key 与模型名称，构造 DeepSeek provider。
// 当 MADY_LIVE_EVAL 或 DEEPSEEK_API_KEY 未设置时返回 nil 表示跳过。
func newDeepSeekTestEnv(t *testing.T) *deepSeekTestEnv {
	t.Helper()
	if os.Getenv("MADY_LIVE_EVAL") != "1" {
		t.Skip("set MADY_LIVE_EVAL=1 to run live evaluation against DeepSeek")
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set")
	}
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}
	return &deepSeekTestEnv{
		Provider: chatcompat.New(chatcompat.Config{
			APIKey:  apiKey,
			BaseURL: "https://api.deepseek.com/v1",
		}),
		Model: model,
	}
}

// randomCases 从 all 中随机选取 n 个 case，固定或可变种子便于复现。
func randomCases(t *testing.T, all []evaluate.TestCase, n int, seed int64) []evaluate.TestCase {
	t.Helper()
	if len(all) < n {
		t.Fatalf("not enough cases: got %d, need %d", len(all), n)
	}
	r := rand.New(rand.NewSource(seed))
	selected := r.Perm(len(all))[:n]
	cases := make([]evaluate.TestCase, 0, n)
	for _, idx := range selected {
		cases = append(cases, all[idx])
	}
	return cases
}

// runLiveEval 针对给定 case 集合调用 DeepSeek，使用缓存并在完成后输出评估报告。
func runLiveEval(t *testing.T, env *deepSeekTestEnv, cases []evaluate.TestCase, cachePath, systemPrompt string) {
	t.Helper()
	if len(cases) == 0 {
		t.Fatal("no cases to evaluate")
	}

	cache := loadCache(cachePath)
	for i, c := range cases {
		if pred, ok := cache[c.ID]; ok && pred != "" {
			t.Logf("(%d/%d) %s loaded from cache (len=%d)", i+1, len(cases), c.ID, len(pred))
			continue
		}
		t.Logf("(%d/%d) calling DeepSeek for %s...", i+1, len(cases), c.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		req := &agentcore.ProviderRequest{
			Model: env.Model,
			Messages: []agentcore.Message{
				{Role: agentcore.RoleSystem, Content: systemPrompt},
				{Role: agentcore.RoleUser, Content: c.Input},
			},
			Temperature: 0.2,
		}
		resp, err := env.Provider.Complete(ctx, req)
		cancel()
		if err != nil {
			t.Fatalf("case %s: %v", c.ID, err)
		}
		cache[c.ID] = resp.Content
		saveCache(cachePath, cache)
		t.Logf("(%d/%d) %s response length: %d", i+1, len(cases), c.ID, len(resp.Content))
	}

	// Build input -> prediction map for efficient RunFunc lookup.
	inputToPred := make(map[string]string, len(cases))
	for _, c := range cases {
		inputToPred[c.Input] = cache[c.ID]
	}

	report, err := LiveEvaluator(env.Provider, env.Model).EvaluateBatch(context.Background(), cases, func(ctx context.Context, input string) (string, error) {
		return inputToPred[input], nil
	})
	if err != nil {
		t.Fatalf("EvaluateBatch: %v", err)
	}

	t.Logf("Total cases: %d", report.TotalCases)
	t.Logf("Passed: %d", report.PassedCases)
	t.Logf("Pass rate: %.2f", report.PassRate)
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

func loadCache(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]string{}
	}
	return out
}

func saveCache(path string, cache map[string]string) {
	data, _ := json.MarshalIndent(cache, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
}

// invalidationSystemPrompt 是专利复审无效决定书分析的专家提示。
const invalidationSystemPrompt = `你是一名资深的专利复审和无效宣告审查专家。

请严格按照以下五步工作法分析专利无效宣告请求审查决定案例：
① 收集事实：列出涉案专利信息、权利要求、请求人提交的证据、请求理由。
② 检索规则：识别并引用相关的中国专利法、专利法实施细则、专利审查指南条文。
③ 制定计划：确定分析步骤，针对每项无效理由逐一分析。
④ 执行推理：按步骤进行法律推理和技术对比，判断各权利要求是否具备新颖性、创造性或是否符合其他法条规定。
⑤ 校验结论：给出最终审查决定结论（维持有效/全部无效/部分无效），并说明核心理由和依据的法条。

最后输出完整、条理清晰的正式答案。`

// patentExamSystemPrompt 是专利代理人考试实务题的专家提示。
const patentExamSystemPrompt = `你是一名资深的专利代理人和专利审查专家。

请严格按照以下五步工作法回答专利代理人考试实务题：
① 收集事实：列出题目中的关键事实、技术方案、时间线、当事人、权利要求、证据文件等。
② 检索规则：识别并引用相关的中国专利法、专利法实施细则、专利审查指南条文。
③ 制定计划：根据事实和规则，确定分析步骤和需要回答的子问题。
④ 执行推理：按步骤进行法律推理和技术对比，给出每一步的结论和依据。
⑤ 校验结论：检查结论是否与事实、规则一致，是否存在遗漏或矛盾，并给出最终结论。

最后输出完整、条理清晰的正式答案。`

// TestLiveDeepSeekInvalidationEval 使用 DeepSeek API 对全部 40 道无效决定书案例进行真实评分。
// 中间结果缓存到 /tmp/mady_deepseek_invalidation_eval.json。
//
// P2B FROZEN (2026-07-15): 此数据集已冻结，仅保留测试代码与缓存以备数据重建后复用。
// 冻结原因（经代码验证）：
//  1. 空壳输入 — 40/40 条的「独立权利要求1」「主要证据」「请求理由」三个字段全部为空，
//     模型仅凭专利号与类型瞎猜结论，无法进行有效推理。
//  2. 退化分布 — 实际结论分布为 全部无效 34 / 部分无效 5 / 维持有效 1（文档曾误记为
//     16/14/10），模型对每题都回答"全部无效"即可获得可观分数，数据集无法区分模型好坏。
//  3. 评估口径 — runLiveEval 直接调裸 Provider.Complete，不走 Agent runtime，32.5% 的
//     通过率测的是 DeepSeek 裸读空壳题目的能力，与 Mady 产品能力无关。
//
// 重建方向：回到本地 2009 件原始 docx 重新提取完整字段（权利要求/对比文件/请求理由/决定要点）
// 并平衡结论分布后再解冻。详见 docs/evaluation-baseline-v0.6.md。
func TestLiveDeepSeekInvalidationEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cachePath := filepath.Join(os.TempDir(), "mady_deepseek_invalidation_eval.json")
	runLiveEval(t, env, InvalidationDecisionCases, cachePath, invalidationSystemPrompt)
}

// TestLiveDeepSeekEval 使用 DeepSeek API 对随机 3 道专利代理人考试真题进行真实评分。
// 固定随机种子保证结果可复现；中间结果缓存到 /tmp/mady_deepseek_eval.json。
func TestLiveDeepSeekEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)

	allCases := PatentExamRealCases()
	seed := int64(20241201)
	cases := randomCases(t, allCases, 3, seed)
	t.Logf("Random seed: %d", seed)
	for i, c := range cases {
		t.Logf("selected %d: %s", i+1, c.ID)
	}

	cachePath := filepath.Join(os.TempDir(), "mady_deepseek_eval.json")
	runLiveEval(t, env, cases, cachePath, patentExamSystemPrompt)
}

// PatentExamRealCases 聚合全部 31 道真实专利考试真题 case。
func PatentExamRealCases() []evaluate.TestCase {
	var cases []evaluate.TestCase
	cases = append(cases, PatentExamRealA2Cases...)
	cases = append(cases, PatentExamRealA22Cases...)
	cases = append(cases, PatentExamRealA26Cases...)
	cases = append(cases, PatentExamRealA31Cases...)
	cases = append(cases, PatentExamRealA33Cases...)
	cases = append(cases, PatentExamRealR42Cases...)
	return cases
}
