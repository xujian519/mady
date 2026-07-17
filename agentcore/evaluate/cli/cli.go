// Package cli provides the CLI evaluation engine for running benchmarks
// from the command line via `mady eval`.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore/evaluate"
	"github.com/xujian519/mady/agentcore/evaluate/benchmark"
)

// OutputFormat 控制 CLI 报告的输出格式。
type OutputFormat string

const (
	FormatTable    OutputFormat = "table"
	FormatJSON     OutputFormat = "json"
	FormatMarkdown OutputFormat = "markdown"
)

// RunMode 控制评估的执行模式。
type RunMode string

const (
	ModeStatic RunMode = "static"
	ModeLive   RunMode = "live"
)

// EvalCLI 是评估 CLI 的配置和引擎。
type EvalCLI struct {
	// Suite 指定测试集名称：p1 / p2a / p2b / all（默认 all）
	Suite string

	// Domain 按领域过滤：patent / legal / general（空=不限制）
	Domain string

	// CaseIDs 指定一个或多个用例 ID（可选），非空时覆盖 Suite 和 Domain
	CaseIDs []string

	// Format 输出格式：table / json / markdown，默认 markdown
	Format OutputFormat

	// Output 输出文件路径（空=stdout）
	Output string

	// Mode 执行模式：static / live
	Mode RunMode

	// LLMModel 仅在 live 模式使用，设 LLM 模型名（如 "deepseek-chat"）
	LLMModel string

	// LLMProvider 仅在 live 模式使用，设 LLM Provider
	LLMProvider ProviderFactory

	// Workers 并发数，默认 4
	Workers int

	// TimeoutSec 单题超时秒数，默认 900
	TimeoutSec int

	// Predictions 仅在 static 模式使用，预存预测 map[CaseID]prediction
	Predictions map[string]string
}

// ProviderFactory 是创建 LLM Provider 的工厂函数，供 live 模式使用。
type ProviderFactory func(model string) (interface {
	Complete(ctx context.Context, req interface{}) (interface{}, error)
}, error)

// RunResult 包含 CLI 评估的完整结果。
type RunResult struct {
	Report    *evaluate.BatchReport
	Duration  time.Duration
	CLIConfig *EvalCLI
}

// RunCLI 根据 CLI 配置运行评估，返回结果。
func RunCLI(ctx context.Context, cli *EvalCLI) (*RunResult, error) {
	start := time.Now()

	cases := loadCases(cli)
	if len(cases) == 0 {
		return nil, fmt.Errorf("cli: 没有匹配的测试用例 (suite=%q domain=%q case_ids=%v)",
			cli.Suite, cli.Domain, cli.CaseIDs)
	}

	var report *evaluate.BatchReport
	var err error

	switch cli.Mode {
	case ModeStatic:
		report = runStatic(cases, cli.Predictions)
	case ModeLive:
		report, err = runLive(ctx, cases, cli)
	default:
		return nil, fmt.Errorf("cli: 不支持的执行模式 %q", cli.Mode)
	}

	if err != nil {
		return nil, fmt.Errorf("cli: 评估失败: %w", err)
	}

	return &RunResult{
		Report:    report,
		Duration:  time.Since(start),
		CLIConfig: cli,
	}, nil
}

// loadCases 根据 CLI 配置加载和过滤测试用例。
func loadCases(cli *EvalCLI) []evaluate.TestCase {
	if len(cli.CaseIDs) > 0 {
		allCases := benchmark.AllCases()
		idSet := make(map[string]bool, len(cli.CaseIDs))
		for _, id := range cli.CaseIDs {
			idSet[strings.TrimSpace(id)] = true
		}
		var filtered []evaluate.TestCase
		for _, c := range allCases {
			if idSet[c.ID] {
				filtered = append(filtered, c)
			}
		}
		return filtered
	}

	var selected []evaluate.TestCase
	switch cli.Suite {
	case "p1":
		all := benchmark.AllCases()
		var p1Cases []evaluate.TestCase
		for _, c := range all {
			if !strings.Contains(c.ID, "real") && !strings.Contains(c.ID, "invalidation") {
				p1Cases = append(p1Cases, c)
			}
		}
		if len(p1Cases) > 0 {
			selected = p1Cases
		}
	case "p2a":
		all := benchmark.AllCases()
		for _, c := range all {
			if strings.Contains(c.ID, "real") && !strings.Contains(c.ID, "invalidation") {
				selected = append(selected, c)
			}
		}
	case "p2b":
		selected = benchmark.InvalidationDecisionCases
	case "", "all":
		selected = benchmark.AllCases()
	default:
		if cli.Domain != "" {
			selected = benchmark.CasesByDomain(cli.Domain)
		} else {
			selected = benchmark.AllCases()
		}
	}

	if cli.Domain != "" && len(selected) > 0 {
		var filtered []evaluate.TestCase
		for _, c := range selected {
			if c.Domain == cli.Domain {
				filtered = append(filtered, c)
			}
		}
		return filtered
	}

	return selected
}

// runStatic 以静态模式运行评估。
func runStatic(cases []evaluate.TestCase, predictions map[string]string) *evaluate.BatchReport {
	if predictions == nil {
		predictions = make(map[string]string)
	}
	return benchmark.DefaultEvaluator().EvaluateStatic(cases, predictions)
}

// runLive 以 live 模式运行评估。
func runLive(ctx context.Context, cases []evaluate.TestCase, cli *EvalCLI) (*evaluate.BatchReport, error) {
	workers := cli.Workers
	if workers <= 0 {
		workers = 4
	}
	timeout := cli.TimeoutSec
	if timeout <= 0 {
		timeout = 900
	}

	type caseTask struct {
		evaluate.TestCase
		index int
	}
	type caseResult struct {
		index      int
		prediction string
		err        error
	}

	tasks := make([]caseTask, len(cases))
	for i, c := range cases {
		tasks[i] = caseTask{TestCase: c, index: i}
	}

	results := make([]caseResult, len(cases))
	var mu sync.Mutex
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(t caseTask, idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			if cli.LLMProvider == nil {
				mu.Lock()
				results[idx] = caseResult{index: idx, prediction: "", err: fmt.Errorf("无 LLM Provider")}
				mu.Unlock()
				return
			}

			evalCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			provider, err := cli.LLMProvider(cli.LLMModel)
			if err != nil {
				mu.Lock()
				results[idx] = caseResult{index: idx, prediction: "", err: err}
				mu.Unlock()
				return
			}

			prediction, err := callProviderSimple(evalCtx, provider, t.Input)
			mu.Lock()
			results[idx] = caseResult{index: idx, prediction: prediction, err: err}
			mu.Unlock()
		}(task, i)
	}
	wg.Wait()

	predictions := make(map[string]string, len(results))
	for _, r := range results {
		if r.err == nil {
			predictions[cases[r.index].ID] = r.prediction
		}
	}

	return benchmark.DefaultEvaluator().EvaluateStatic(cases, predictions), nil
}

func callProviderSimple(ctx context.Context, provider interface{}, input string) (string, error) {
	type Completer interface {
		Complete(ctx context.Context, req interface{}) (interface{}, error)
	}
	p, ok := provider.(Completer)
	if !ok {
		return "", fmt.Errorf("provider 不实现 Complete 接口")
	}
	resp, err := p.Complete(ctx, map[string]interface{}{
		"messages": []map[string]interface{}{
			{"role": "user", "content": input},
		},
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", resp), nil
}

// FormatResult 根据 cli 配置输出格式，将运行结果转为字符串。
func FormatResult(result *RunResult) string {
	if result == nil || result.Report == nil {
		return "# 评估报告\n\n无数据。\n"
	}
	switch result.CLIConfig.Format {
	case FormatTable:
		return formatTableReport(result.Report)
	case FormatJSON:
		return formatJSONReport(result.Report, result.Duration)
	default:
		return formatMarkdownReport(result.Report, result.Duration)
	}
}

func formatMarkdownReport(report *evaluate.BatchReport, duration time.Duration) string {
	base := evaluate.FormatReport(report)
	return base + fmt.Sprintf("\n## 执行信息\n\n- 耗时: %s\n", duration.Round(time.Millisecond))
}

func formatTableReport(report *evaluate.BatchReport) string {
	var b strings.Builder
	b.WriteString("# 评估报告\n\n")

	fmt.Fprintf(&b, "用例数: %d | 通过: %d | 通过率: %.1f%%\n\n",
		report.TotalCases, report.PassedCases, report.PassRate*100)

	if len(report.AggregateScores) > 0 {
		b.WriteString("| 指标 | 平均分 |\n|------|--------|\n")
		names := make([]string, 0, len(report.AggregateScores))
		for n := range report.AggregateScores {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "| %s | %.3f |\n", name, report.AggregateScores[name])
		}
	}

	if len(report.Results) > 0 {
		b.WriteString("\n| 用例 | 平均分 | 状态 |\n|------|--------|------|\n")
		for _, r := range report.Results {
			status := "✅"
			if !r.Passed {
				status = "❌"
			}
			fmt.Fprintf(&b, "| %s | %.3f | %s |\n", r.CaseID, r.Average, status)
		}
	}
	return b.String()
}

func formatJSONReport(report *evaluate.BatchReport, duration time.Duration) string {
	type jsonResult struct {
		CaseID  string             `json:"case_id"`
		Passed  bool               `json:"passed"`
		Average float64            `json:"average"`
		Scores  map[string]float64 `json:"scores,omitempty"`
	}
	type jsonReport struct {
		TotalCases      int                `json:"total_cases"`
		PassedCases     int                `json:"passed_cases"`
		PassRate        float64            `json:"pass_rate"`
		AggregateScores map[string]float64 `json:"aggregate_scores,omitempty"`
		DurationMs      int64              `json:"duration_ms"`
		Results         []jsonResult       `json:"results,omitempty"`
	}

	jr := jsonReport{
		TotalCases:      report.TotalCases,
		PassedCases:     report.PassedCases,
		PassRate:        report.PassRate,
		AggregateScores: report.AggregateScores,
		DurationMs:      duration.Milliseconds(),
		Results:         make([]jsonResult, 0, len(report.Results)),
	}
	for _, r := range report.Results {
		jr.Results = append(jr.Results, jsonResult{
			CaseID:  r.CaseID,
			Passed:  r.Passed,
			Average: r.Average,
			Scores:  r.Scores,
		})
	}

	data, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// OutputResult 将运行结果写入指定的输出目标。
func OutputResult(result *RunResult) error {
	output := result.CLIConfig.Output
	content := FormatResult(result)

	if output == "" || output == "-" {
		_, err := os.Stdout.WriteString(content)
		return err
	}
	return os.WriteFile(output, []byte(content), 0o600)
}
