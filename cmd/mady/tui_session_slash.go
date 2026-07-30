package main

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/workflows/patent"
	"github.com/xujian519/mady/graph"
)

// runSingleInputSlashWorkflow is the shared handler for single-input patent
// slash commands (novelty / oa / invalidation / reexamination). It extracts
// the text after the command name, validates it, runs the compiled graph,
// and persists+displays the result. Each caller provides its own build+run
// closure and usage hint.
func (s *tuiSession) runSingleInputSlashWorkflow(
	ctx slashCtx,
	cmdName string,
	usage string,
	runGraph func(input string) (string, error),
) {
	text := strings.TrimSpace(strings.TrimPrefix(ctx.input, "/"+cmdName))
	text = strings.Trim(text, `"'`)
	if text == "" {
		s.app.PrintSystem(usage)
		return
	}

	output, err := runGraph(text)
	if err != nil {
		s.app.PrintError(err)
		return
	}
	if output == "" {
		s.app.PrintSystem("分析完成但未能生成输出结果。")
		return
	}

	s.persistSlashMessages(ctx.input, output)
	s.app.PrintSystem(output)
}

// handleNoveltySlash 处理 /novelty <描述> 斜杠命令——直接运行新颖性分析 Pregel 图，
// 绕过 LLM 意图分类，结果输出到聊天面板并经 AgentStore 持久化。
//
//nolint:dupl // Slash handlers follow the same runSingleInputSlashWorkflow pattern intentionally
func (s *tuiSession) handleNoveltySlash(ctx slashCtx) {
	s.runSingleInputSlashWorkflow(ctx, "novelty",
		"用法: /novelty <发明描述>\n"+
			"示例: /novelty \"一种基于深度学习的图像识别方法，包括卷积神经网络...\"",
		func(input string) (string, error) {
			opts := []patent.GraphOption{}
			if retriever := domains.GetPatentRetriever(); retriever != nil {
				opts = append(opts, patent.WithRetriever(retriever))
			}
			compiled, err := patent.BuildNoveltyGraphWithRulesWithOpts(opts...)
			if err != nil {
				return "", fmt.Errorf("新颖性分析引擎初始化失败: %w", err)
			}
			state, err := compiled.Run(s.ctx, graph.PregelState{
				patent.StateInput: input,
			})
			if err != nil {
				return "", fmt.Errorf("新颖性分析执行失败: %w", err)
			}
			return state.GetString(patent.StateOutput), nil
		})
}

// handleOASlash 处理 /oa <OA通知书文本> 斜杠命令——直接运行 OA 答复起草 Pregel 图。
//
//nolint:dupl // Slash handlers follow the same runSingleInputSlashWorkflow pattern intentionally
func (s *tuiSession) handleOASlash(ctx slashCtx) {
	s.runSingleInputSlashWorkflow(ctx, "oa",
		"用法: /oa <OA通知书文本>\n"+
			"示例: /oa \"审查员认为权利要求1不具备专利法第22条第2款规定的新颖性\"",
		func(input string) (string, error) {
			var oaOpts []patent.OAGraphOption
			if s.fc.Provider != nil {
				oaOpts = append(oaOpts, patent.WithOAProvider(s.fc.Provider))
			}
			compiled, err := patent.BuildOAResponseGraphWithOpts(oaOpts...)
			if err != nil {
				return "", fmt.Errorf("OA 答复引擎初始化失败: %w", err)
			}
			state, err := compiled.Run(s.ctx, graph.PregelState{
				patent.OAStateInput: input,
			})
			if err != nil {
				return "", fmt.Errorf("OA 答复生成失败: %w", err)
			}
			return state.GetString(patent.OAStateOutput), nil
		})
}

// handleInvalidationSlash 处理 /invalidation <权利要求文本> 斜杠命令——
// 直接运行无效宣告分析 Pregel 图。
//
//nolint:dupl // Slash handlers follow the same runSingleInputSlashWorkflow pattern intentionally
func (s *tuiSession) handleInvalidationSlash(ctx slashCtx) {
	s.runSingleInputSlashWorkflow(ctx, "invalidation",
		"用法: /invalidation <权利要求文本>\n"+
			"示例: /invalidation \"1. 一种图像处理方法...\n请求人主张第22条第2款新颖性无效\"",
		func(input string) (string, error) {
			opts := []patent.InvGraphOption{}
			if retriever := domains.GetPatentRetriever(); retriever != nil {
				opts = append(opts, patent.WithInvRetriever(retriever))
			}
			compiled, err := patent.BuildInvalidationGraphWithOpts(opts...)
			if err != nil {
				return "", fmt.Errorf("无效宣告分析引擎初始化失败: %w", err)
			}
			state, err := compiled.Run(s.ctx, graph.PregelState{
				patent.InvStateInput: input,
			})
			if err != nil {
				return "", fmt.Errorf("无效宣告分析执行失败: %w", err)
			}
			return state.GetString(patent.InvStateOutput), nil
		})
}

// handleReexaminationSlash 处理 /reexamination <驳回决定书> 斜杠命令——
// 直接运行复审请求书起草 Pregel 图。
//
//nolint:dupl // Slash handlers follow the same runSingleInputSlashWorkflow pattern intentionally
func (s *tuiSession) handleReexaminationSlash(ctx slashCtx) {
	s.runSingleInputSlashWorkflow(ctx, "reexamination",
		"用法: /reexamination <驳回决定书文本>\n"+
			"示例: /reexamination \"驳回决定编号：2024-001\n审查员认为权利要求1不具备新颖性...\"",
		func(input string) (string, error) {
			compiled, err := patent.BuildReexaminationGraph()
			if err != nil {
				return "", fmt.Errorf("复审请求书引擎初始化失败: %w", err)
			}
			state, err := compiled.Run(s.ctx, graph.PregelState{
				patent.ReexamStateInput: input,
			})
			if err != nil {
				return "", fmt.Errorf("复审请求书起草失败: %w", err)
			}
			return state.GetString(patent.ReexamStateOutput), nil
		})
}

// handleInfringementSlash 处理 /infringement <权利要求> <被控方案> 斜杠命令——
// 直接运行侵权比对分析 Pregel 图。两个参数以 | 分隔。
func (s *tuiSession) handleInfringementSlash(ctx slashCtx) {
	raw := strings.TrimSpace(strings.TrimPrefix(ctx.input, "/infringement"))
	raw = strings.Trim(raw, `"'`)
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		s.app.PrintSystem("用法: /infringement <权利要求文本> | <被控侵权方案>\n" +
			"两个参数以 | 分隔。\n" +
			"示例: /infringement 1. 一种装置包括A和B。 | 被控产品包含A和C")
		return
	}

	claimsText := strings.TrimSpace(parts[0])
	productText := strings.TrimSpace(parts[1])

	compiled, err := patent.BuildInfringementGraph()
	if err != nil {
		s.app.PrintError(fmt.Errorf("侵权分析引擎初始化失败: %w", err))
		return
	}

	state, err := compiled.Run(s.ctx, graph.PregelState{
		patent.InfStatePatentClaims:   claimsText,
		patent.InfStateAccusedProduct: productText,
	})
	if err != nil {
		s.app.PrintError(fmt.Errorf("侵权分析执行失败: %w", err))
		return
	}

	output := state.GetString(patent.InfStateOutput)
	if output == "" {
		s.app.PrintSystem("分析完成但未能生成输出结果。")
		return
	}

	s.persistSlashMessages(ctx.input, output)
	s.app.PrintSystem(output)
}
