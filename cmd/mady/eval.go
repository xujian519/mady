package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/xujian519/mady/agentcore/evaluate/cli"
)

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

// multiStringFlag 支持重复的 --case 标志。
type multiStringFlag []string

func (f *multiStringFlag) String() string {
	return strings.Join(*f, ", ")
}

func (f *multiStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}
