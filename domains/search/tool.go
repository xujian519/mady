package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval/domain"
)

// ToolName 是 patent_search_commander 工具的注册名。
const ToolName = "patent_search_commander"

// CommanderToolConfig 配置 patent_search_commander 工具的默认行为。
type CommanderToolConfig struct {
	// Retriever 是多源检索器（通常为 ego-browser 驱动的 CompositeRetriever）。
	// nil 时工具不注册（静默降级）。
	Retriever domain.DomainRetriever
	// MaxRounds 默认最大轮次（默认 4）。
	MaxRounds int
	// PerRound 默认每轮条数（默认 10）。
	PerRound int
}

// NewCommanderTool 创建多轮渐进式专利检索编排工具。
//
// 工具封装 search.Commander：LLM 一次调用，内部完成 场景识别 → 策略模板 →
// 多轮检索 → 反思收敛 → 综合报告 的完整编排，返回 Markdown 报告。
// Retriever 不可用（nil）时返回 nil（工具不注册，与 patent_web_search 一致）。
//
// 输入参数：
//   - query（必填）：检索主题，如 "骨髓腔输液装置的现有技术"
//   - scene（可选）：场景 auto/oa/invalidation/infringement/fto/academic
//   - country（可选）：cn 中国 / global 全球
//   - ipcs（可选）：已知 IPC 约束数组
//   - max_rounds（可选）：最大轮次，默认 4
//   - per_round（可选）：每轮条数，默认 10
func NewCommanderTool(cfg *CommanderToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &CommanderToolConfig{}
	}
	commander := NewCommander(cfg.Retriever,
		WithMaxRounds(cfg.MaxRounds),
		WithPerRound(cfg.PerRound),
	)
	if commander == nil {
		return nil
	}

	return &agentcore.Tool{
		Name: ToolName,
		Description: "多轮渐进式专利检索编排器（Search Commander）：自动执行" +
			"宽语义检索 → IPC/申请人过滤 → 二次验证 → 穷举覆盖 的多轮策略，" +
			"统一调度 Google Patents/CNIPA/Espacenet（ego-browser 驱动）多源，" +
			"每轮反思收敛，输出对比文件总表与遗漏分析报告。适用于现有技术调查、" +
			"查新、无效宣告证据收集、FTO、侵权排查等需要系统性检索的场景。" +
			"区别于 patent_web_search（单轮关键词检索）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "检索主题（必填），如 '骨髓腔输液装置'、'深度学习图像识别'",
				},
				"scene": map[string]any{
					"type":        "string",
					"enum":        []any{"auto", "oa", "invalidation", "infringement", "fto", "academic"},
					"description": "检索场景：auto 自动识别（默认）/ oa 现有技术调查 / invalidation 无效证据 / infringement 侵权排查 / fto 自由实施 / academic 学术+专利",
				},
				"country": map[string]any{
					"type":        "string",
					"enum":        []any{"cn", "global"},
					"description": "国家范围：cn 中国 / global 全球（默认不限定）",
				},
				"ipcs": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "已知 IPC 分类号约束（可选），如 ['G06F 17/30']",
				},
				"max_rounds": map[string]any{
					"type":        "integer",
					"description": "最大检索轮次，默认 4",
				},
				"per_round": map[string]any{
					"type":        "integer",
					"description": "每轮返回条数，默认 10",
				},
			},
			"required": []any{"query"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Query     string   `json:"query"`
				Scene     string   `json:"scene"`
				Country   string   `json:"country"`
				IPCs      []string `json:"ipcs"`
				MaxRounds int      `json:"max_rounds"`
				PerRound  int      `json:"per_round"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("patent_search_commander 参数错误: %w", err)
			}
			if strings.TrimSpace(input.Query) == "" {
				return nil, fmt.Errorf("patent_search_commander: query 不能为空")
			}
			req := Request{
				Query:     input.Query,
				Scene:     Scene(input.Scene),
				Country:   input.Country,
				IPCs:      input.IPCs,
				MaxRounds: input.MaxRounds,
				PerRound:  input.PerRound,
			}
			report, err := commander.Run(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("patent_search_commander 执行失败: %w", err)
			}
			return report.Markdown(), nil
		},
		ReadOnly: true,
	}
}

// CommanderExtension 将 patent_search_commander 工具注册进 Agent 的扩展。
//
// 与 tools.Extension 平级：通过 Init 时 RegisterTools 注入。
// ego-browser 不可用（Retriever 为 nil）时工具不注册，扩展本身仍是空操作，
// 保证装配层无需感知数据源可用性。
type CommanderExtension struct {
	tool *agentcore.Tool
}

// NewCommanderExtension 构造扩展。retriever 为 nil 时生成无工具的空扩展。
func NewCommanderExtension(retriever domain.DomainRetriever) *CommanderExtension {
	return &CommanderExtension{tool: NewCommanderTool(&CommanderToolConfig{Retriever: retriever})}
}

// Name 返回扩展名。
func (e *CommanderExtension) Name() string { return "search-commander" }

// Init 注册工具到 Agent。
func (e *CommanderExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	if e.tool != nil {
		agent.RegisterTools(e.tool)
	}
	return nil
}

// Dispose 空实现。
func (e *CommanderExtension) Dispose() error { return nil }

// Tools 返回扩展管理的工具（供 ToolProvider 探测）。
func (e *CommanderExtension) Tools() []*agentcore.Tool {
	if e.tool == nil {
		return nil
	}
	return []*agentcore.Tool{e.tool}
}

// 编译期接口合规检查。
var _ agentcore.Extension = (*CommanderExtension)(nil)
var _ agentcore.ToolProvider = (*CommanderExtension)(nil)
