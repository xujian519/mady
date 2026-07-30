package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/workflows/patent"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/util"
)

// runPatentCLI dispatches `mady patent <subcommand> [args...]`.
//
// Subcommands:
//
//	mady patent novelty [<description> | -f <file>] [-o <file>]
//	mady patent oa [<oa_text> | -f <file>] [-o <file>]
func runPatentCLI(ctx context.Context, args []string) error {
	if len(args) < 3 {
		printPatentUsage()
		return fmt.Errorf("patent: missing subcommand (novelty/oa/invalidation/infringement/reexamination)")
	}

	subcommand := args[2]
	subArgs := args[3:]
	switch subcommand {
	case "novelty":
		return runPatentNovelty(ctx, subArgs)
	case "oa":
		return runPatentOA(ctx, subArgs)
	case "invalidation":
		return runPatentInvalidation(ctx, subArgs)
	case "infringement":
		return runPatentInfringement(ctx, subArgs)
	case "reexamination":
		return runPatentReexamination(ctx, subArgs)
	default:
		printPatentUsage()
		return fmt.Errorf("patent: unknown subcommand %q", subcommand)
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
func parseCLIArgs(args []string) (input, outputFile string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("patent: -f 需要文件名参数")
			}
			data, rerr := util.ReadFile(args[i+1]) // CLI arg from user
			if rerr != nil {
				return "", "", fmt.Errorf("读取文件失败: %w", rerr)
			}
			input = string(data)
			i++ // skip the next arg (filename)
		case "-o":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("patent: -o 需要文件名参数")
			}
			outputFile = args[i+1]
			i++ // skip the next arg (filename)
		default:
			// First non-flag argument is treated as direct input text.
			if input == "" {
				input = args[i]
			}
		}
	}
	return
}

//nolint:dupl // Patent CLI run functions follow the same parse→build→run→output pattern
func runPatentNovelty(ctx context.Context, args []string) error {
	input, outputFile, err := parseCLIArgs(args)
	if err != nil {
		return fmt.Errorf("patent novelty: %w", err)
	}
	if input == "" {
		printPatentUsage()
		return fmt.Errorf("patent novelty: 请提供发明描述或使用 -f <file>")
	}

	opts := []patent.GraphOption{}
	if retriever := domains.GetPatentRetriever(); retriever != nil {
		opts = append(opts, patent.WithRetriever(retriever))
	}
	compiled, cerr := patent.BuildNoveltyGraphWithRulesWithOpts(opts...)
	if cerr != nil {
		return fmt.Errorf("patent novelty: 分析引擎初始化失败: %w", cerr)
	}

	state, rerr := compiled.Run(ctx, graph.PregelState{
		patent.StateInput: input,
	})
	if rerr != nil {
		return fmt.Errorf("patent novelty: 分析执行失败: %w", rerr)
	}

	output := state.GetString(patent.StateOutput)
	if output == "" {
		return fmt.Errorf("patent novelty: 分析完成但未能生成输出")
	}

	if outputFile != "" {
		if serr := patent.SaveNoveltyReport(output, outputFile); serr != nil {
			return fmt.Errorf("patent novelty: 保存报告失败: %w", serr)
		}
		fmt.Fprintf(os.Stderr, "报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
	return nil
}

//nolint:dupl // Patent CLI run functions follow the same parse→build→run→output pattern
func runPatentOA(ctx context.Context, args []string) error {
	input, outputFile, err := parseCLIArgs(args)
	if err != nil {
		return fmt.Errorf("patent oa: %w", err)
	}
	if input == "" {
		printPatentUsage()
		return fmt.Errorf("patent oa: 请提供 OA 通知书文本或使用 -f <file>")
	}

	compiled, cerr := patent.BuildOAResponseGraph()
	if cerr != nil {
		return fmt.Errorf("patent oa: OA 答复引擎初始化失败: %w", cerr)
	}

	state, rerr := compiled.Run(ctx, graph.PregelState{
		patent.OAStateInput: input,
	})
	if rerr != nil {
		return fmt.Errorf("patent oa: OA 答复生成失败: %w", rerr)
	}

	output := state.GetString(patent.OAStateOutput)
	if output == "" {
		return fmt.Errorf("patent oa: OA 答复生成完成但未能生成输出")
	}

	if outputFile != "" {
		if serr := patent.SaveOAResponse(output, outputFile); serr != nil {
			return fmt.Errorf("patent oa: 保存答复书失败: %w", serr)
		}
		fmt.Fprintf(os.Stderr, "答复书已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
	return nil
}

//nolint:dupl // Patent CLI run functions follow the same parse→build→run→output pattern
func runPatentInvalidation(ctx context.Context, args []string) error {
	input, outputFile, err := parseCLIArgs(args)
	if err != nil {
		return fmt.Errorf("patent invalidation: %w", err)
	}
	if input == "" {
		printPatentUsage()
		return fmt.Errorf("patent invalidation: 请提供权利要求文本或使用 -f <file>")
	}

	opts := []patent.InvGraphOption{}
	if retriever := domains.GetPatentRetriever(); retriever != nil {
		opts = append(opts, patent.WithInvRetriever(retriever))
	}
	compiled, cerr := patent.BuildInvalidationGraphWithOpts(opts...)
	if cerr != nil {
		return fmt.Errorf("patent invalidation: 分析引擎初始化失败: %w", cerr)
	}

	state, rerr := compiled.Run(ctx, graph.PregelState{
		patent.InvStateInput: input,
	})
	if rerr != nil {
		return fmt.Errorf("patent invalidation: 分析执行失败: %w", rerr)
	}

	output := state.GetString(patent.InvStateOutput)
	if output == "" {
		return fmt.Errorf("patent invalidation: 分析完成但未能生成输出")
	}

	if outputFile != "" {
		if serr := patent.SaveNoveltyReport(output, outputFile); serr != nil {
			return fmt.Errorf("patent invalidation: 保存报告失败: %w", serr)
		}
		fmt.Fprintf(os.Stderr, "无效宣告分析报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
	return nil
}

func runPatentInfringement(ctx context.Context, args []string) error {
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
			if i+1 >= len(args) {
				return fmt.Errorf("patent infringement: -f 需要文件名参数")
			}
			data, rerr := util.ReadFile(args[i+1]) // CLI arg from user
			if rerr != nil {
				return fmt.Errorf("patent infringement: 读取文件失败: %w", rerr)
			}
			positional = append(positional, string(data))
			i++
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 2 {
		printPatentUsage()
		return fmt.Errorf("patent infringement: 需要两个参数：专利权利要求文本 和 被控侵权方案描述")
	}

	claimsText := positional[0]
	productText := positional[1]

	compiled, cerr := patent.BuildInfringementGraph()
	if cerr != nil {
		return fmt.Errorf("patent infringement: 分析引擎初始化失败: %w", cerr)
	}

	state, rerr := compiled.Run(ctx, graph.PregelState{
		patent.InfStatePatentClaims:   claimsText,
		patent.InfStateAccusedProduct: productText,
	})
	if rerr != nil {
		return fmt.Errorf("patent infringement: 分析执行失败: %w", rerr)
	}

	output := state.GetString(patent.InfStateOutput)
	if output == "" {
		return fmt.Errorf("patent infringement: 分析完成但未能生成输出")
	}

	if outputFile != "" {
		if serr := patent.SaveNoveltyReport(output, outputFile); serr != nil {
			return fmt.Errorf("patent infringement: 保存报告失败: %w", serr)
		}
		fmt.Fprintf(os.Stderr, "侵权分析报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
	return nil
}

//nolint:dupl // Patent CLI run functions follow the same parse→build→run→output pattern
func runPatentReexamination(ctx context.Context, args []string) error {
	input, outputFile, err := parseCLIArgs(args)
	if err != nil {
		return fmt.Errorf("patent reexamination: %w", err)
	}
	if input == "" {
		printPatentUsage()
		return fmt.Errorf("patent reexamination: 请提供驳回决定书文本或使用 -f <file>")
	}

	compiled, cerr := patent.BuildReexaminationGraph()
	if cerr != nil {
		return fmt.Errorf("patent reexamination: 复审请求书引擎初始化失败: %w", cerr)
	}

	state, rerr := compiled.Run(ctx, graph.PregelState{
		patent.ReexamStateInput: input,
	})
	if rerr != nil {
		return fmt.Errorf("patent reexamination: 复审请求书起草失败: %w", rerr)
	}

	output := state.GetString(patent.ReexamStateOutput)
	if output == "" {
		return fmt.Errorf("patent reexamination: 起草完成但未能生成输出")
	}

	if outputFile != "" {
		if serr := patent.SaveNoveltyReport(output, outputFile); serr != nil {
			return fmt.Errorf("patent reexamination: 保存报告失败: %w", serr)
		}
		fmt.Fprintf(os.Stderr, "复审请求书已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
	return nil
}
