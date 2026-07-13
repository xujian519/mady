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

// TestLiveDeepSeekEval 使用 DeepSeek API 对随机 3 道专利代理人考试真题进行真实评分。
// 运行条件：环境变量 DEEPSEEK_API_KEY 已设置。
// 中间结果会缓存到 /tmp/mady_deepseek_eval.json，中断后可重新运行继续。
func TestLiveDeepSeekEval(t *testing.T) {
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

	provider := chatcompat.New(chatcompat.Config{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com/v1",
	})

	cachePath := filepath.Join(os.TempDir(), "mady_deepseek_eval.json")
	cache := loadCache(cachePath)

	allCases := PatentExamRealCases()
	if len(allCases) < 3 {
		t.Fatalf("not enough cases: got %d", len(allCases))
	}
	seed := time.Now().UnixNano()
	r := rand.New(rand.NewSource(seed))
	selected := r.Perm(len(allCases))[:3]
	cases := make([]evaluate.TestCase, 0, 3)
	for _, idx := range selected {
		cases = append(cases, allCases[idx])
	}
	t.Logf("Random seed: %d", seed)
	for i, c := range cases {
		t.Logf("selected %d: %s", i+1, c.ID)
	}

	systemPrompt := `你是一名资深的专利代理人和专利审查专家。

请严格按照以下五步工作法回答专利代理人考试实务题：
① 收集事实：列出题目中的关键事实、技术方案、时间线、当事人、权利要求、证据文件等。
② 检索规则：识别并引用相关的中国专利法、专利法实施细则、专利审查指南条文。
③ 制定计划：根据事实和规则，确定分析步骤和需要回答的子问题。
④ 执行推理：按步骤进行法律推理和技术对比，给出每一步的结论和依据。
⑤ 校验结论：检查结论是否与事实、规则一致，是否存在遗漏或矛盾，并给出最终结论。

最后输出完整、条理清晰的正式答案。`

	for i, c := range cases {
		if pred, ok := cache[c.ID]; ok && pred != "" {
			t.Logf("(%d/%d) %s loaded from cache (len=%d)", i+1, len(cases), c.ID, len(pred))
			continue
		}
		t.Logf("(%d/%d) calling DeepSeek for %s...", i+1, len(cases), c.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		req := &agentcore.ProviderRequest{
			Model: model,
			Messages: []agentcore.Message{
				{Role: agentcore.RoleSystem, Content: systemPrompt},
				{Role: agentcore.RoleUser, Content: c.Input},
			},
			Temperature: 0.2,
		}
		resp, err := provider.Complete(ctx, req)
		cancel()
		if err != nil {
			t.Fatalf("case %s: %v", c.ID, err)
		}
		cache[c.ID] = resp.Content
		saveCache(cachePath, cache)
		t.Logf("(%d/%d) %s response length: %d", i+1, len(cases), c.ID, len(resp.Content))
	}

	report, err := LiveEvaluator(provider, model).EvaluateBatch(context.Background(), cases, func(ctx context.Context, input string) (string, error) {
		for _, c := range cases {
			if c.Input == input {
				return cache[c.ID], nil
			}
		}
		return "", nil
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
