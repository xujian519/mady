package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/workflows/patent"
)

// runPatentCLI dispatches `mady patent <subcommand> [args...]`.
//
// Subcommands:
//
//	mady patent novelty [<description> | -f <file>] [-o <file>]
//	mady patent oa [<oa_text> | -f <file>] [-o <file>]
func runPatentCLI(ctx context.Context, args []string) {
	if len(args) < 3 {
		printPatentUsage()
		os.Exit(2)
	}

	subcommand := args[2]
	subArgs := args[3:]
	switch subcommand {
	case "novelty":
		runPatentNovelty(ctx, subArgs)
	case "oa":
		runPatentOA(ctx, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown patent subcommand %q\n\n", subcommand)
		printPatentUsage()
		os.Exit(2)
	}
}

func printPatentUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  mady patent novelty [<description> | -f <file>] [-o <file>]
        新颖性/创造性分析：对发明进行技术特征提取、现有技术检索、
        规则引擎检查，生成结构化分析报告。
        -o <file>  将结果写入 Markdown 文件（可选）

  mady patent oa [<oa_text> | -f <file>] [-o <file>]
        审查意见（OA）答复起草：分析通知书文本，生成答复书骨架。
        -o <file>  将结果写入 Markdown 文件（可选）

Examples:
  mady patent novelty "一种基于深度学习的图像识别方法，包括卷积神经网络..."
  mady patent novelty -f invention.txt
  mady patent novelty -f invention.txt -o report.md
  mady patent oa "审查员认为权利要求1不具备新颖性..."
  mady patent oa -f office_action.txt
  mady patent oa -f office_action.txt -o response.md
`)
}

// parseCLIArgs 解析 CLI 参数，返回 (input, outputFile)。
// -f <file> 从文件读取输入；-o <file> 指定输出文件；其余视为直接输入文本。
func parseCLIArgs(args []string) (input, outputFile string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f":
			if i+1 < len(args) {
				data, err := os.ReadFile(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "读取文件失败: %v\n", err)
					os.Exit(1)
				}
				input = string(data)
				i++ // skip the next arg (filename)
			}
		case "-o":
			if i+1 < len(args) {
				outputFile = args[i+1]
				i++ // skip the next arg (filename)
			}
		default:
			// First non-flag argument is treated as direct input text.
			if input == "" {
				input = args[i]
			}
		}
	}
	return
}

func runPatentNovelty(ctx context.Context, args []string) {
	input, outputFile := parseCLIArgs(args)

	if input == "" {
		fmt.Fprintln(os.Stderr, "请提供发明描述、使用 -f <file> 从文件读取，或使用 -h 查看帮助")
		printPatentUsage()
		os.Exit(2)
	}

	opts := []patent.GraphOption{}
	if retriever := domains.GetPatentRetriever(); retriever != nil {
		opts = append(opts, patent.WithRetriever(retriever))
	}
	compiled, err := patent.BuildNoveltyGraphWithRulesWithOpts(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "分析引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	state, err := compiled.Run(ctx, graph.PregelState{
		patent.StateInput: input,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "分析执行失败: %v\n", err)
		os.Exit(1)
	}

	output := state.GetString(patent.StateOutput)
	if output == "" {
		fmt.Fprintln(os.Stderr, "分析完成但未能生成输出")
		os.Exit(1)
	}

	// 输出结果：写入文件或 stdout。
	if outputFile != "" {
		if err := patent.SaveNoveltyReport(output, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
}

func runPatentOA(ctx context.Context, args []string) {
	input, outputFile := parseCLIArgs(args)

	if input == "" {
		fmt.Fprintln(os.Stderr, "请提供 OA 通知书文本、使用 -f <file> 从文件读取，或使用 -h 查看帮助")
		printPatentUsage()
		os.Exit(2)
	}

	compiled, err := patent.BuildOAResponseGraph()
	if err != nil {
		fmt.Fprintf(os.Stderr, "OA 答复引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	state, err := compiled.Run(ctx, graph.PregelState{
		patent.OAStateInput: input,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "OA 答复生成失败: %v\n", err)
		os.Exit(1)
	}

	output := state.GetString(patent.OAStateOutput)
	if output == "" {
		fmt.Fprintln(os.Stderr, "OA 答复生成完成但未能生成输出")
		os.Exit(1)
	}

	// 输出结果：写入文件或 stdout。
	if outputFile != "" {
		if err := patent.SaveOAResponse(output, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "保存答复书失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "答复书已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
}
