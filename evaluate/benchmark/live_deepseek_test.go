package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/evaluate"
	"github.com/xujian519/mady/provider/chatcompat"
)

// deepSeekTestEnv 保存 DeepSeek live 测试的公共环境参数。
type deepSeekTestEnv struct {
	Provider agentcore.Provider
	Model    string
}

// newDeepSeekTestEnv 从环境变量读取 API key 与模型名称，构造 live 评估 provider。
// 当 MADY_LIVE_EVAL 未设为 "1" 或密钥缺失时跳过（t.Skip）。
//
// 端点选择（DeepSeek 官方端点优先，支持任意 OpenAI 兼容端点回退）：
//   - DEEPSEEK_API_KEY（可选 DEEPSEEK_MODEL，默认 deepseek-chat）
//     → https://api.deepseek.com/v1；
//   - 否则 MADY_EVAL_API_KEY + MADY_EVAL_BASE_URL + MADY_EVAL_MODEL
//     → 任意 OpenAI 兼容端点（如本地 MLX 服务 http://127.0.0.1:8000/v1），
//     用于零成本/最小成本冒烟验证 live 链路。
func newDeepSeekTestEnv(t *testing.T) *deepSeekTestEnv {
	t.Helper()
	if os.Getenv("MADY_LIVE_EVAL") != "1" {
		t.Skip("set MADY_LIVE_EVAL=1 to run live evaluation against DeepSeek")
	}
	if apiKey := os.Getenv("DEEPSEEK_API_KEY"); apiKey != "" {
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
	// 回退：通用 OpenAI 兼容端点（本地或第三方），三者缺一不可。
	apiKey := os.Getenv("MADY_EVAL_API_KEY")
	baseURL := os.Getenv("MADY_EVAL_BASE_URL")
	model := os.Getenv("MADY_EVAL_MODEL")
	if apiKey == "" || baseURL == "" || model == "" {
		t.Skip("DEEPSEEK_API_KEY not set (or provide MADY_EVAL_API_KEY + MADY_EVAL_BASE_URL + MADY_EVAL_MODEL)")
	}
	return &deepSeekTestEnv{
		Provider: chatcompat.New(chatcompat.Config{
			APIKey:  apiKey,
			BaseURL: baseURL,
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
	// 生成阶段并发跑批：本地 MLX 端点支持连续批处理，liveEvalWorkers 路并发
	// 显著缩短墙钟时间；cache 写入与 firstErr 收集由 mu 保护，错误在 Wait 后
	// 统一 Fatal（t.Fatalf 不能从子 goroutine 调用）。
	// 先同步挑出未生成的题（主 goroutine 读 cache），再并发生成，
	// 否则主循环读 cache 与 worker 写 cache 形成数据竞争。
	var pending []int
	for i, c := range cases {
		if pred, ok := cache[c.ID]; ok && pred != "" {
			t.Logf("(%d/%d) %s loaded from cache (len=%d)", i+1, len(cases), c.ID, len(pred))
			continue
		}
		pending = append(pending, i)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, liveEvalWorkers)
	for _, i := range pending {
		c := cases[i]
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			t.Logf("(%d/%d) calling DeepSeek for %s...", i+1, len(cases), c.ID)
			ctx, cancel := context.WithTimeout(context.Background(), liveCaseTimeout)
			defer cancel()
			req := &agentcore.ProviderRequest{
				Model: env.Model,
				Messages: []agentcore.Message{
					{Role: agentcore.RoleSystem, Content: systemPrompt},
					{Role: agentcore.RoleUser, Content: c.Input},
				},
				Temperature: 0.2,
				MaxTokens:   liveMaxResponseTokens,
			}
			resp, err := env.Provider.Complete(ctx, req)
			mu.Lock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("case %s: %w", c.ID, err)
				}
			} else {
				cache[c.ID] = resp.Content
				saveCache(cachePath, cache)
			}
			mu.Unlock()
			if err == nil {
				t.Logf("(%d/%d) %s response length: %d", i+1, len(cases), c.ID, len(resp.Content))
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatalf("%v", firstErr)
	}

	// Build input -> prediction map for efficient RunFunc lookup.
	inputToPred := make(map[string]string, len(cases))
	for _, c := range cases {
		inputToPred[c.Input] = cache[c.ID]
	}

	// 评判阶段同样按题缓存（<cachePath>.judge），长跑批被打断可无损续跑。
	report := evaluateBatchWithJudgeCache(t, LiveEvaluator(env.Provider, env.Model), cases, func(ctx context.Context, input string) (string, error) {
		return inputToPred[input], nil
	}, cachePath+".judge")

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

// TestLiveDeepSeekInvalidationEval 使用 DeepSeek API 对全部无效决定书案例进行真实评分。
// 中间结果缓存到 /tmp/mady_deepseek_invalidation_eval.json。
//
// P2B REBUILT (2026-07-15): 此数据集已从 宝宸知识库_Raw（31562 件真实无效决定书 MD）
// 重新提取，含完整字段（权利要求1/证据/无效理由）和均衡结论分布。
// 之前的冻结版本（40 条空壳）已被替换。重建脚本见 scripts/extract_invalidation_cases.py。
func TestLiveDeepSeekInvalidationEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)
	cases := InvalidationDecisionCases
	// Allow limiting the number of P2B cases for faster iteration.
	if n := evalAgentCaseCount(t); n < len(cases) {
		cases = randomCases(t, cases, n, 20241201)
	}
	cachePath := filepath.Join(os.TempDir(), "mady_deepseek_invalidation_eval.json")
	runLiveEval(t, env, cases, cachePath, invalidationSystemPrompt)
}

// TestLiveDeepSeekEval 使用 DeepSeek API 对专利代理人考试真题（P2A）进行真实评分。
// 默认随机抽 3 题（固定种子保证可复现）；MADY_EVAL_CASES 控制题量，
// MADY_EVAL_SUITE=p2a 跑全量 31 题。中间结果缓存到 /tmp/mady_deepseek_eval.json。
func TestLiveDeepSeekEval(t *testing.T) {
	env := newDeepSeekTestEnv(t)

	cases := p2aEvalCases(t)
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
