package benchmark

// 本文件为 live 评估提供「按题评判缓存」：与生成缓存（/tmp/mady_*_eval.json）
// 配套，把 EvaluateBatch 的逐题评判结果增量落盘（<生成缓存路径>.judge）。
// 动机：本地/免费端点跑批耗时长，常被外部超时（如 300s 命令窗口）打断；
// 评判阶段此前不落盘，打断即全废。缓存后生成与评判两阶段均可无损续跑。
// 仅测试辅助代码，不影响生产路径。

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/evaluate"
)

// liveEvalWorkers 是 live 评估生成/评判两阶段的并发度。
// 本地 MLX 端点（连续批处理）下 4 路并发聚合吞吐约 2.4×，且单流速率不稀释；
// 过高并发会拉长单题墙钟时间，在受限命令窗口内反而无法完题。
const liveEvalWorkers = 4

// liveCaseTimeout 是单题生成/运行的超时。本地 12B 端点生成万字长答案
// 需要 10 分钟以上（远慢于云端 API），5 分钟会在长题上稳定超时。
const liveCaseTimeout = 15 * time.Minute

// liveMaxResponseTokens 是单题响应上限。本地端点逐流速率有限（~55 字符/秒），
// 不设上限时长尾题（>16K 字符）在任何受限命令窗口内都无法完成；
// 8192 tokens（约 1.2 万中文字符）只截断病态超长答案，常规答案不受影响。
const liveMaxResponseTokens int64 = 8192

// evaluateBatchWithJudgeCache 与 Evaluator.EvaluateBatch 语义一致，但逐题缓存
// 评判结果：已缓存的题直接命中，未缓存的题并发评判并立即落盘。
func evaluateBatchWithJudgeCache(t *testing.T, ev *evaluate.Evaluator, cases []evaluate.TestCase, run evaluate.RunFunc, judgeCachePath string) *evaluate.BatchReport {
	t.Helper()

	cache := loadJudgeCache(judgeCachePath)
	results := make([]evaluate.CaseResult, len(cases))

	// 先同步挑出未评判的题（主 goroutine 读 cache），再并发评判；
	// 否则主循环读 cache 与 worker 写 cache 形成数据竞争。
	var pending []int
	for i, c := range cases {
		if r, ok := cache[c.ID]; ok && r.Scores != nil {
			t.Logf("(%d/%d) %s judge loaded from cache", i+1, len(cases), c.ID)
			results[i] = r
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
			t.Logf("(%d/%d) judging %s ...", i+1, len(cases), c.ID)
			single, err := ev.EvaluateBatch(context.Background(), []evaluate.TestCase{c}, run)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if len(single.Results) != 1 {
				if firstErr == nil {
					firstErr = &judgeResultCountError{caseID: c.ID, got: len(single.Results)}
				}
				return
			}
			r := single.Results[0]
			cache[c.ID] = r
			saveJudgeCache(judgeCachePath, cache)
			results[i] = r
		}()
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatalf("judge: %v", firstErr)
	}

	report := &evaluate.BatchReport{
		Results:         results,
		TotalCases:      len(results),
		AggregateScores: map[string]float64{},
	}
	metricSums := map[string]float64{}
	metricCounts := map[string]int{}
	for _, r := range report.Results {
		if r.Passed {
			report.PassedCases++
		}
		for name, score := range r.Scores {
			metricSums[name] += score
			metricCounts[name]++
		}
	}
	if report.TotalCases > 0 {
		report.PassRate = float64(report.PassedCases) / float64(report.TotalCases)
	}
	for name, sum := range metricSums {
		report.AggregateScores[name] = sum / float64(metricCounts[name])
	}
	return report
}

// judgeResultCountError 表示单题评判返回的结果数量异常（恒应为 1）。
type judgeResultCountError struct {
	caseID string
	got    int
}

func (e *judgeResultCountError) Error() string {
	return "EvaluateBatch case " + e.caseID + ": unexpected result count"
}

// loadJudgeCache 读取评判缓存；文件缺失或损坏时返回空缓存（容忍中断现场）。
func loadJudgeCache(path string) map[string]evaluate.CaseResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]evaluate.CaseResult{}
	}
	var out map[string]evaluate.CaseResult
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]evaluate.CaseResult{}
	}
	return out
}

// saveJudgeCache 原子性要求不高（测试缓存），直接覆盖写。
func saveJudgeCache(path string, cache map[string]evaluate.CaseResult) {
	data, _ := json.MarshalIndent(cache, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
}
