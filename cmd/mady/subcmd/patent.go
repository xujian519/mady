package subcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/workflows/patent"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/util"
)

// RunPatentCLI dispatches `mady patent <subcommand> [args...]`.
//
// Subcommands:
//
//	mady patent novelty [<description> | -f <file>] [-o <file>]
//	mady patent oa [<oa_text> | -f <file>] [-o <file>]
func RunPatentCLI(ctx context.Context, args []string) error {
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

// runPatentPipeline 是 patent CLI run 函数的公共骨架：
// 解析参数 → 校验输入 → 构建图 → 执行 → 取输出 → 保存或打印。
// build 返回已绑定 ctx 的执行函数（执行图并返回输出文本），
// save 处理报告落盘（nil 时用 patent.SaveNoveltyReport）。
func runPatentPipeline(args []string, opName, usage string,
	build func(input string) (string, error),
	save func(output, file string) error,
) error {
	input, outputFile, err := parseCLIArgs(args)
	if err != nil {
		return fmt.Errorf("patent %s: %w", opName, err)
	}
	if input == "" {
		printPatentUsage()
		return fmt.Errorf("patent %s: %s", opName, usage)
	}

	output, rerr := build(input)
	if rerr != nil {
		return fmt.Errorf("patent %s: 分析执行失败: %w", opName, rerr)
	}
	if output == "" {
		return fmt.Errorf("patent %s: 分析完成但未能生成输出", opName)
	}

	if outputFile != "" {
		if save == nil {
			save = patent.SaveNoveltyReport
		}
		if serr := save(output, outputFile); serr != nil {
			return fmt.Errorf("patent %s: 保存报告失败: %w", opName, serr)
		}
		fmt.Fprintf(os.Stderr, "报告已保存到: %s\n", outputFile)
	} else {
		fmt.Println(output)
	}
	return nil
}

func runPatentNovelty(ctx context.Context, args []string) error { //nolint:dupl // novelty/invalidation 的图构建闭包结构相似但 GraphOption 类型不同
	return runPatentPipeline(args, "novelty", "请提供发明描述或使用 -f <file>", func(input string) (string, error) {
		opts := []patent.GraphOption{}
		if retriever := domains.GetPatentRetriever(); retriever != nil {
			opts = append(opts, patent.WithRetriever(retriever))
		}
		compiled, cerr := patent.BuildNoveltyGraphWithRulesWithOpts(opts...)
		if cerr != nil {
			return "", fmt.Errorf("分析引擎初始化失败: %w", cerr)
		}
		state, rerr := compiled.Run(ctx, graph.PregelState{patent.StateInput: input})
		if rerr != nil {
			return "", fmt.Errorf("分析执行失败: %w", rerr)
		}
		return state.GetString(patent.StateOutput), nil
	}, patent.SaveNoveltyReport)
}

func runPatentOA(ctx context.Context, args []string) error {
	return runPatentPipeline(args, "oa", "请提供 OA 通知书文本或使用 -f <file>", func(input string) (string, error) {
		compiled, cerr := patent.BuildOAResponseGraph()
		if cerr != nil {
			return "", fmt.Errorf("OA 答复引擎初始化失败: %w", cerr)
		}
		state, rerr := compiled.Run(ctx, graph.PregelState{patent.OAStateInput: input})
		if rerr != nil {
			return "", fmt.Errorf("OA 答复生成失败: %w", rerr)
		}
		return state.GetString(patent.OAStateOutput), nil
	}, patent.SaveOAResponse)
}

func runPatentInvalidation(ctx context.Context, args []string) error { //nolint:dupl // 与 novelty 的图构建闭包结构相似但 GraphOption 类型不同
	return runPatentPipeline(args, "invalidation", "请提供权利要求文本或使用 -f <file>", func(input string) (string, error) {
		opts := []patent.InvGraphOption{}
		if retriever := domains.GetPatentRetriever(); retriever != nil {
			opts = append(opts, patent.WithInvRetriever(retriever))
		}
		compiled, cerr := patent.BuildInvalidationGraphWithOpts(opts...)
		if cerr != nil {
			return "", fmt.Errorf("分析引擎初始化失败: %w", cerr)
		}
		state, rerr := compiled.Run(ctx, graph.PregelState{patent.InvStateInput: input})
		if rerr != nil {
			return "", fmt.Errorf("分析执行失败: %w", rerr)
		}
		return state.GetString(patent.InvStateOutput), nil
	}, patent.SaveNoveltyReport)
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
	return runPatentPipeline(args, "reexamination", "请提供驳回决定书文本或使用 -f <file>", func(input string) (string, error) {
		compiled, cerr := patent.BuildReexaminationGraph()
		if cerr != nil {
			return "", fmt.Errorf("复审请求书引擎初始化失败: %w", cerr)
		}
		state, rerr := compiled.Run(ctx, graph.PregelState{patent.ReexamStateInput: input})
		if rerr != nil {
			return "", fmt.Errorf("复审请求书起草失败: %w", rerr)
		}
		return state.GetString(patent.ReexamStateOutput), nil
	}, patent.SaveNoveltyReport)
}
