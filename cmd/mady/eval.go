package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore/evaluate"
	"github.com/xujian519/mady/agentcore/evaluate/cli"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/pkg/util"
)

// citationVerifierAdapter 将线上引用核验 Gate（guardrails.VerifyCitations）
// 适配为 evaluate.CitationVerifier，作为 `mady eval` 的核验源注入。
//
// 注入点选在 cmd/mady 顶层入口而非 agentcore/evaluate/cli，原因是：
// evaluate 包不得反向引用 guardrails（架构约束，详见 AGENTS.md 分层规范），
// 由顶层装配层注入最干净。
func citationVerifierAdapter(text string) evaluate.CitationValidityReport {
	r := guardrails.VerifyCitations(text)
	return evaluate.CitationValidityReport{
		Total:        r.Total,
		Valid:        r.Valid,
		Unknown:      r.Unknown,
		Unverifiable: r.Unverifiable,
		Suspect:      r.Suspect,
		Invalid:      r.Invalid,
	}
}

// runEvalCLI 运行评估 CLI 子命令。
//
// 用法:
//
//	mady eval [flags]
//
// 标志:
//
//	--suite      测试集名称: p1, p2a, p2b, all (默认 all)
//	--domain     领域过滤: patent, legal, general (默认 不限制)
//	--case       指定用例 ID (可重复, 如 --case id1 --case id2)
//	--format     输出格式: table, json, markdown (默认 markdown)
//	--output     输出文件路径 (默认 stdout)
//	--mode       执行模式: static, live (默认 static)
//	--model      (live 模式) LLM 模型名 (如 deepseek-chat)
//	--workers    (live 模式) 并发数 (默认 4)
//	--timeout    (live 模式) 单题超时秒数 (默认 900)
func runEval(ctx context.Context, args []string) error {
	// 子命令分发：mady eval baseline [flags]
	if len(args) > 0 && args[0] == "baseline" {
		return runEvalBaseline(ctx, args[1:])
	}

	// 注入线上引用核验 Gate 作为 citation_validity 核验源。
	evaluate.SetCitationVerifier(citationVerifierAdapter)

	fs := flag.NewFlagSet("eval", flag.ContinueOnError)

	suite := fs.String("suite", "all", "测试集: p1, p2a, p2b, all")
	domain := fs.String("domain", "", "领域过滤: patent, legal, general")
	var caseIDs multiStringFlag
	fs.Var(&caseIDs, "case", "指定用例 ID (可重复)")
	format := fs.String("format", "markdown", "输出格式: table, json, markdown, enhanced")
	output := fs.String("output", "", "输出文件路径 (默认 stdout)")
	baseline := fs.String("baseline", "", "(enhanced 格式) 前次评估的 JSON 结果文件，用于趋势对比")
	mode := fs.String("mode", "static", "执行模式: static, live")
	model := fs.String("model", "", "(live 模式) LLM 模型名")
	workers := fs.Int("workers", 4, "(live 模式) 并发数")
	timeoutSec := fs.Int("timeout", 900, "(live 模式) 单题超时秒数")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// 验证参数
	var runMode cli.RunMode
	switch *mode {
	case "static":
		runMode = cli.ModeStatic
	case "live":
		runMode = cli.ModeLive
	default:
		return fmt.Errorf("不支持的执行模式 %q, 可用值: static, live", *mode)
	}

	var outputFormat cli.OutputFormat
	switch *format {
	case "table":
		outputFormat = cli.FormatTable
	case "json":
		outputFormat = cli.FormatJSON
	case "markdown":
		outputFormat = cli.FormatMarkdown
	case "enhanced":
		outputFormat = cli.FormatEnhanced
	default:
		return fmt.Errorf("不支持的输出格式 %q, 可用值: table, json, markdown, enhanced", *format)
	}

	if runMode == cli.ModeLive && *model == "" {
		return fmt.Errorf("live 模式需要 --model 参数")
	}

	evalCLI := &cli.EvalCLI{
		Suite:        *suite,
		Domain:       *domain,
		CaseIDs:      caseIDs,
		Format:       outputFormat,
		Output:       *output,
		BaselineFile: *baseline,
		Mode:         runMode,
		LLMModel:     *model,
		Workers:      *workers,
		TimeoutSec:   *timeoutSec,
	}

	result, err := cli.RunCLI(ctx, evalCLI)
	if err != nil {
		return fmt.Errorf("评估失败: %w", err)
	}

	if err := cli.OutputResult(result); err != nil {
		return fmt.Errorf("输出结果失败: %w", err)
	}

	// 非零退出码表示未全部通过
	if result.Report.PassRate < 1.0 {
		os.Exit(1)
	}
	return nil
}

// runEvalBaseline 读取 EvalStore 数据并输出评估基线统计。
//
// 用法:  mady eval baseline [--since YYYY-MM-DD] [--until YYYY-MM-DD]
//
// 输出格式兼容 docs/evaluation-baseline-v0.8.md 的基线文档格式。
func runEvalBaseline(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("eval baseline", flag.ContinueOnError)
	since := fs.String("since", "", "起始日期 (YYYY-MM-DD, 默认 7 天前)")
	until := fs.String("until", "", "截止日期 (YYYY-MM-DD, 默认今天)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// 解析日期，默认 7 天窗口。
	sinceTime := time.Now().AddDate(0, 0, -7)
	untilTime := time.Now()
	if *since != "" {
		t, err := time.Parse("2006-01-02", *since)
		if err != nil {
			return fmt.Errorf("--since 日期格式无效 %q: %w (应为 YYYY-MM-DD)", *since, err)
		}
		sinceTime = t
	}
	if *until != "" {
		t, err := time.Parse("2006-01-02", *until)
		if err != nil {
			return fmt.Errorf("--until 日期格式无效 %q: %w (应为 YYYY-MM-DD)", *until, err)
		}
		untilTime = t.Add(24*time.Hour - time.Second) // inclusive end of day
	}

	// 打开 eval.db。
	madyHome, err := util.MadyHome()
	if err != nil {
		return fmt.Errorf("获取 MADY_HOME 失败: %w", err)
	}
	evalDB := filepath.Join(madyHome, "eval.db")
	if _, err := os.Stat(evalDB); err != nil {
		return fmt.Errorf("eval.db 不存在（%s），请先运行 mady serve 生成评估数据", evalDB)
	}

	store, err := knowledge.NewEvalStore(knowledge.EvalStoreConfig{DSN: evalDB})
	if err != nil {
		return fmt.Errorf("打开 eval.db 失败: %w", err)
	}
	defer store.Close()

	stats, err := store.QueryStats(ctx, sinceTime, untilTime)
	if err != nil {
		return fmt.Errorf("查询统计失败: %w", err)
	}

	// 输出基线报告。
	fmt.Printf("\n📊 Eval Baseline (%s)\n", stats.TimeRange)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("总评估数:          %d\n", stats.TotalEvaluations)
	if stats.TotalEvaluations > 0 {
		fmt.Printf("平均忠实度:        %.2f\n", stats.AvgFaithfulness)
		fmt.Printf("平均回答相关度:     %.2f\n", stats.AvgAnswerRelevancy)
		fmt.Printf("平均上下文精度:     %.2f\n", stats.AvgContextPrecision)
		fmt.Printf("低忠实度 (<0.7):   %d (%.1f%%)\n", stats.LowFaithfulness, stats.LowFaithfulnessRate*100)
	} else {
		fmt.Println("（无评估数据）")
	}
	fmt.Println()

	// 查询低忠实度详情。
	if stats.LowFaithfulness > 0 {
		lowResults, err := store.QueryByThreshold(ctx, 0.7, 10)
		if err == nil && len(lowResults) > 0 {
			fmt.Println("低忠实度示例（前 10 条）：")
			fmt.Println(strings.Repeat("-", 50))
			for i, r := range lowResults {
				if i >= 5 {
					fmt.Printf("  ... 还有 %d 条\n", len(lowResults)-5)
					break
				}
				fmt.Printf("  Turn %d | 忠实度 %.2f | %s\n", r.Turn, r.Faithfulness, truncate(r.Question, 60))
			}
			fmt.Println()
		}
	}

	return nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// multiStringFlag 支持重复的 --case 标志。
type multiStringFlag []string

func (f *multiStringFlag) String() string {
	return strings.Join(*f, ", ")
}

func (f *multiStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}
