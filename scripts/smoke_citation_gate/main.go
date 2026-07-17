// smoke_citation_gate 是引用核验 Gate 的端到端单题冒烟工具（P1b 接线后验证）。
//
// 用本地 oMLX 端点对 2008_a31_02（v0.8 已确认的法条编号幻觉题：
// 分案申请依据被错引为"专利法第47条"）跑完整 domains.PatentAgentConfig
// Agent——含 CitationGate + LevelStrict 护栏 + ApprovalGate 的完整 hook 链，
// 验证"⚠️ 引用核验提示"在真实生成输出中的呈现效果。
//
// 环境变量：
//   - OMLX_API_KEY（必须，本地 oMLX 端点密钥）
//   - OMLX_BASE_URL（默认 http://127.0.0.1:8000/v1）
//   - OMLX_MODEL（默认 gemma-4-12B-it-8bit）
//   - SMOKE_CASE（默认 patent_exam_2008_a31_02，可换其他 P2A caseID）
//   - SMOKE_MAX_TOKENS（默认 8192；前台 300s 受限场景可先 2048 快速探路）
//   - SMOKE_MAX_TURNS（默认 6）
//   - SMOKE_FILE（离线核验模式：跳过 LLM 生成，直接对该文件跑
//     guardrails.VerifyCitations 输出判定报告——任何文本都可核验，零成本）
//
// 用法：
//
//	set -a && source .env && set +a && go run ./scripts/smoke_citation_gate
//
// 输出：完整答案写入 $TMPDIR/mady_smoke_citation_gate_<case>.md，
// 并在 stdout 报告核验提示命中情况。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/evaluate/benchmark"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/provider/chatcompat"
)

// envInt 读取整型环境变量，缺省或非法时返回 def。
func envInt(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n := int64(0)
	if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
		return n
	}
	return def
}

// verifyFile 离线核验既有文本文件（SMOKE_FILE 模式）。
func verifyFile(file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取失败 %s: %v\n", file, err)
		os.Exit(1)
	}
	report := guardrails.VerifyCitations(string(data))
	fmt.Printf("离线核验 %s（%d 字）\n", file, len([]rune(string(data))))
	fmt.Printf("引用总数 %d：Valid %d / Unknown %d / Unverifiable %d / Suspect %d / Invalid %d\n",
		report.Total, report.Valid, report.Unknown, report.Unverifiable, report.Suspect, report.Invalid)
	if len(report.Flagged) == 0 {
		fmt.Println("→ 无需标记：全部引用通过核验或按防线放行（Gate 不标注正确）")
		return
	}
	fmt.Println("→ 标记明细" + guardrails.FormatCitationWarnings(report))
}

func main() {
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OMLX_API_KEY 未设置（用法：set -a && source .env && set +a && go run ./scripts/smoke_citation_gate）")
		os.Exit(1)
	}
	baseURL := os.Getenv("OMLX_BASE_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8000/v1"
	}
	model := os.Getenv("OMLX_MODEL")
	if model == "" {
		model = "gemma-4-12B-it-8bit"
	}
	caseID := os.Getenv("SMOKE_CASE")
	if caseID == "" {
		caseID = "patent_exam_2008_a31_02"
	}
	maxTokens := envInt("SMOKE_MAX_TOKENS", 8192)
	maxTurns := envInt("SMOKE_MAX_TURNS", 6)

	// 离线核验模式：跳过 LLM，直接核验既有文本文件。
	if file := os.Getenv("SMOKE_FILE"); file != "" {
		verifyFile(file)
		return
	}

	// 定位题目。
	var input string
	for _, c := range benchmark.AllCases() {
		if c.ID == caseID {
			input = c.Input
			break
		}
	}
	if input == "" {
		fmt.Fprintf(os.Stderr, "题库中未找到 case %s\n", caseID)
		os.Exit(1)
	}
	fmt.Printf("冒烟题目：%s（输入 %d 字）\n端点：%s 模型：%s\n\n", caseID, len([]rune(input)), baseURL, model)

	// 端到端：完整 PatentAgentConfig（CitationGate + Strict 护栏 + ApprovalGate
	// 完整 hook 链），与 TUI/Server 产品路径同一配置。
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "smoke-citation-gate",
			Model:     model,
			Provider:  chatcompat.New(chatcompat.Config{APIKey: apiKey, BaseURL: baseURL}),
			MaxTokens: maxTokens,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:      maxTurns, // 限制轮次：ApprovalGate Steer 注入后仍能收尾
			ExecutionMode: agentcore.ModeSerial,
		},
	}
	cfg := domains.PatentAgentConfig(base)

	agent := agentcore.New(cfg)
	defer agent.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	start := time.Now()
	out, err := agent.Run(ctx, input)
	elapsed := time.Since(start).Round(time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Agent 运行失败（%s）：%v\n", elapsed, err)
		os.Exit(1)
	}
	fmt.Printf("生成完成（%s，输出 %d 字）\n", elapsed, len([]rune(out)))

	// 完整输出落盘（不入库）。
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("mady_smoke_citation_gate_%s.md", caseID))
	if werr := os.WriteFile(outPath, []byte(out), 0o600); werr != nil {
		fmt.Fprintln(os.Stderr, "输出落盘失败:", werr)
	} else {
		fmt.Println("完整输出:", outPath)
	}

	// 核验提示命中检查。
	fmt.Println()
	if idx := strings.Index(out, "引用核验提示"); idx >= 0 {
		fmt.Println("✅ 端到端命中：输出含「⚠️ 引用核验提示」——")
		tail := out[idx:]
		if len([]rune(tail)) > 800 {
			tail = string([]rune(tail)[:800]) + "…"
		}
		fmt.Println(tail)
	} else {
		fmt.Println("⚠️ 未命中：输出不含「引用核验提示」（本次生成未复现幻觉，或引用均通过核验）")
		fmt.Println("—— 输出尾部 400 字 ——")
		runes := []rune(out)
		if len(runes) > 400 {
			fmt.Println(string(runes[len(runes)-400:]))
		} else {
			fmt.Println(out)
		}
	}
}
