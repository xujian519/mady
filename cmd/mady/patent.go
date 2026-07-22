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
	case "invalidation":
		runPatentInvalidation(ctx, subArgs)
	case "infringement":
		runPatentInfringement(ctx, subArgs)
	case "reexamination":
		runPatentReexamination(ctx, subArgs)
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

  mady patent invalidation [<claims_text> | -f <file>] [-o <file>]
        专利无效宣告分析：输入目标专利权利要求，识别无效理由，
        逐项生成无效论证骨架并经规则引擎校验。
        -o <file>  将结果写入 Markdown 文件（可选）

  mady patent infringement <patent_claims> <accused_product> [-o <file>]
        专利侵权比对分析：输入专利权利要求和被控侵权方案，
        进行全面覆盖（字面侵权）和等同侵权分析。
        -o <file>  将结果写入 Markdown 文件（可选）

  mady patent reexamination [<decision_text> | -f <file>] [-o <file>]
        驳回复审请求书起草：解析驳回决定书，生成复审请求书骨架。
        -o <file>  将结果写入 Markdown 文件（可选）

Examples:
  mady patent novelty "一种基于深度学习的图像识别方法，包括卷积神经网络..."
  mady patent novelty -f invention.txt
  mady patent novelty -f invention.txt -o report.md
  mady patent oa "审查员认为权利要求1不具备新颖性..."
  mady patent oa -f office_action.txt
  mady patent oa -f office_action.txt -o response.md
  mady patent invalidation -f claims.txt
  mady patent invalidation "权利要求1..." -o invalidation.md
  mady patent infringement "权利要求文本" "被控产品描述"
  mady patent reexamination -f rejection.txt
  mady patent reexamination "驳回决定..." -o reexam.md
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

func runPatentInvalidation(ctx context.Context, args []string) {
	input, outputFile := parseCLIArgs(args)

	if input == "" {
		fmt.Fprintln(os.Stderr, "请提供权利要求文本、使用 -f <file> 从文件读取，或使用 -h 查看帮助")
		printPatentUsage()
		os.Exit(2)
	}

	opts := []patent.InvGraphOption{}
	if retriever := domains.GetPatentRetriever(); retriever != nil {
		opts = append(opts, patent.WithInvRetriever(retriever))
	}
	compiled, err := patent.BuildInvalidationGraphWithOpts(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效宣告分析引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	state, err := compiled.Run(ctx, graph.PregelState{
		patent.InvStateInput: input,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "分析执行失败: %v\n", err)
		os.Exit(1)
	}

	output := state.GetString(patent.InvStateOutput)
	if output == "" {
		fmt.Fprintln(os.Stderr, "分析完成但未能生成输出")
		os.Exit(1)
	}

	if outputFile != "" {
		if err := patent.SaveNoveltyReport(output, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "无效宣告分析报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
}

func runPatentInfringement(ctx context.Context, args []string) {
	// infringement requires two arguments: patent claims and accused product
	outputFile := ""
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 < len(args) {
				outputFile = args[i+1]
				i++
			}
		case "-f":
			if i+1 < len(args) {
				data, err := os.ReadFile(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "读取文件失败: %v\n", err)
					os.Exit(1)
				}
				positional = append(positional, string(data))
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "侵权比对分析需要两个参数：专利权利要求文本 和 被控侵权方案描述")
		fmt.Fprintln(os.Stderr, "用法: mady patent infringement <patent_claims> <accused_product> [-o <file>]")
		fmt.Fprintln(os.Stderr, "      mady patent infringement -f claims.txt -f product.txt")
		os.Exit(2)
	}

	claimsText := positional[0]
	productText := positional[1]

	compiled, err := patent.BuildInfringementGraph()
	if err != nil {
		fmt.Fprintf(os.Stderr, "侵权分析引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	state, err := compiled.Run(ctx, graph.PregelState{
		patent.InfStatePatentClaims:   claimsText,
		patent.InfStateAccusedProduct: productText,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "侵权分析执行失败: %v\n", err)
		os.Exit(1)
	}

	output := state.GetString(patent.InfStateOutput)
	if output == "" {
		fmt.Fprintln(os.Stderr, "分析完成但未能生成输出")
		os.Exit(1)
	}

	if outputFile != "" {
		if err := patent.SaveNoveltyReport(output, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "侵权分析报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
}

func runPatentReexamination(ctx context.Context, args []string) {
	input, outputFile := parseCLIArgs(args)

	if input == "" {
		fmt.Fprintln(os.Stderr, "请提供驳回决定书文本、使用 -f <file> 从文件读取，或使用 -h 查看帮助")
		printPatentUsage()
		os.Exit(2)
	}

	compiled, err := patent.BuildReexaminationGraph()
	if err != nil {
		fmt.Fprintf(os.Stderr, "复审请求书引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	state, err := compiled.Run(ctx, graph.PregelState{
		patent.ReexamStateInput: input,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "复审请求书起草失败: %v\n", err)
		os.Exit(1)
	}

	output := state.GetString(patent.ReexamStateOutput)
	if output == "" {
		fmt.Fprintln(os.Stderr, "起草完成但未能生成输出")
		os.Exit(1)
	}

	if outputFile != "" {
		if err := patent.SaveNoveltyReport(output, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "复审请求书已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
}
