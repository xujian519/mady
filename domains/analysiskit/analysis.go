// Package analysiskit 提供专利分析模块（novelty / inventiveness / enablement /
// infringement 等）共享的装配基础设施：
//
//  1. 法条框架骨架：ArticleFrameworkProvider 查询接口 + Framework 查询器 +
//     格式化工具。此前 novelty/inventiveness/enablement 各自复制了一份
//     （~80 行 × 3），差异仅在法条 id 与降级默认文本，已收敛为注入式设计。
//  2. Pregel 装配器：BuildPregel 封装「AddNode 循环 + AddEdge 循环 + Compile」
//     样板（6 个分析模块各复制一份 ~20 行），统一为单一入口。
//
// 各模块保留实质差异：法条数据文本（defaultA22xFramework 等）与节点定义。
package analysiskit

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// 法条框架骨架
// =============================================================================

// ArticleFrameworkProvider 是法条框架查询的抽象接口。
// 生产环境由 domains/rules.Engine 经适配实现，测试/降级场景由 nil 实现。
// 使用接口而非直接引用 domains/rules 包，避免引入 transitive build 依赖。
type ArticleFrameworkProvider interface {
	Article(id string) ArticleFrameworkData
}

// ArticleFrameworkData 是法条框架的纯数据镜像（避免依赖 domains/rules 包）。
type ArticleFrameworkData struct {
	Name             string
	LawRef           string
	GuidelineRef     string
	Steps            []ArticleStepData
	ConclusionSchema map[string]string
	ApplicableTo     []string
}

// ArticleStepData 是单步判断步骤的数据镜像。
type ArticleStepData struct {
	Order        int
	Name         string
	InputHint    string
	OutputSchema map[string]string
}

// Framework 提供特定法条判断框架的查询。
// provider 为 nil 时降级为内置默认框架文本（fallback）。
type Framework struct {
	provider ArticleFrameworkProvider
	ids      []string      // 查询的 article id（含别名，按序尝试）
	fallback func() string // 降级框架文本（领域特有）
}

// NewFramework 创建绑定到 ArticleFrameworkProvider 的 Framework 查询器。
// ids 按序尝试（如 "patent-law-a22.2" → "A22.2"）；provider 为 nil 或未命中时
// 使用 fallback。
func NewFramework(provider ArticleFrameworkProvider, ids []string, fallback func() string) *Framework {
	return &Framework{provider: provider, ids: ids, fallback: fallback}
}

// GetArticleFramework 返回法条判断框架（provider 命中优先，否则降级默认文本）。
func (f *Framework) GetArticleFramework() string {
	if f.provider != nil {
		for _, id := range f.ids {
			if af := f.provider.Article(id); af.Name != "" {
				return FormatArticleData(af)
			}
		}
	}
	return f.fallback()
}

// FormatArticleData 将 ArticleFrameworkData 格式化为 Markdown 文本。
func FormatArticleData(af ArticleFrameworkData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", af.Name)
	fmt.Fprintf(&b, "**法条依据**：%s\n\n", af.LawRef)
	if af.GuidelineRef != "" {
		fmt.Fprintf(&b, "**审查指南依据**：%s\n\n", af.GuidelineRef)
	}

	b.WriteString("### 判断步骤\n\n")
	for _, step := range af.Steps {
		fmt.Fprintf(&b, "**第 %d 步：%s**\n", step.Order, step.Name)
		if step.InputHint != "" {
			fmt.Fprintf(&b, "- 输入：%s\n", step.InputHint)
		}
		for key, desc := range step.OutputSchema {
			fmt.Fprintf(&b, "- %s：%s\n", key, desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("### 结论模式\n\n")
	for key, desc := range af.ConclusionSchema {
		fmt.Fprintf(&b, "- %s：%s\n", key, desc)
	}

	if len(af.ApplicableTo) > 0 {
		fmt.Fprintf(&b, "\n**适用场景**：%s\n", strings.Join(af.ApplicableTo, "、"))
	}

	return b.String()
}

// =============================================================================
// Pregel 装配器
// =============================================================================

// AssemblePregel 按 nodes 与 edges 装配一个 Pregel 图（不编译）。
// 封装各分析模块重复的「AddNode 循环 + AddEdge 循环」样板；返回未编译图，
// 由调用方追加条件边（SetConditionalEdge）等拓扑后自行 Compile。
func AssemblePregel(nodes map[string]graph.PregelNode, edges [][2]string) (*graph.PregelGraph, error) {
	pg := graph.NewPregelGraph()
	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("add node %q: %w", name, err)
		}
	}
	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("add edge %q→%q: %w", e[0], e[1], err)
		}
	}
	return pg, nil
}
