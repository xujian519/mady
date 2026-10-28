// Package lawverify 提供模型可见的法条核验工具。
//
// 它与 guardrails 的引用门共用同一套解析与判定：lawcite.Extract 抽取引用，
// lawcite.Index.Resolve 给出结论。区别在于本工具是「按需查询」——模型主动核对
// 某条引用，而门禁是在模型输出后被动扫描。
package lawverify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/lawcite"
	"github.com/xujian519/mady/pkg/util"
)

// ToolName 是工具名。
const ToolName = "law_verify"

// Config 是工具配置。
type Config struct {
	// Indexes 覆盖随包索引；未提供的法律回退到 lawcite.BundledIndex。
	Indexes map[lawcite.Statute]*lawcite.Index
	// Skeleton 直接注入条号骨架；nil 时按 SkeletonPath 与默认路径加载。
	Skeleton *lawcite.Skeleton
	// SkeletonPath 指定骨架索引文件；为空时按默认路径探测。
	SkeletonPath string
}

// resolved 是一次核验的结果与来源。
type resolved struct {
	resolution lawcite.Resolution
	source     string // index | skeleton | none
}

// verifyArgs 是工具入参。
type verifyArgs struct {
	Text      string `json:"text"`
	Law       string `json:"law"`
	Article   int    `json:"article"`
	Paragraph int    `json:"paragraph"`
	Item      int    `json:"item"`
}

// NewTool 构造 law_verify 工具。
//
// 核验顺序：精校索引 → 条号骨架 → 未核验。骨架只回答条号是否存在，
// 绝不把结果提升为「已核验」。
func NewTool(cfg Config) *agentcore.Tool {
	resolve := makeResolver(cfg.Indexes, loadSkeleton(cfg))
	return &agentcore.Tool{
		Name:        ToolName,
		Description: "核验法条引用：给出条/款/项是否存在于随包索引或条号骨架、条文是否已转录、来源与核验日期。可用 text 传入含引用的文本，或用 law+article 直接指定。",
		ReadOnly:    true,
		Parameters:  toolParameters(),
		Func: func(_ context.Context, raw json.RawMessage) (any, error) {
			return runVerify(raw, resolve)
		},
	}
}

// toolParameters 返回工具的参数 schema。
func toolParameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "含法条引用的文本，如「依据专利法第22条第3款」。与 law+article 二选一。",
			},
			"law": map[string]any{
				"type":        "string",
				"description": "法律简称或全称：专利法 / 专利法实施细则 / 专利审查指南 / 商标法 / 著作权法 / 反不正当竞争法，或条号骨架中的法规名。",
			},
			"article": map[string]any{
				"type":        "integer",
				"description": "条号（与 law 搭配使用）。",
			},
			"paragraph": map[string]any{
				"type":        "integer",
				"description": "款号，可省略。",
			},
			"item": map[string]any{
				"type":        "integer",
				"description": "项号，可省略。",
			},
		},
	}
}

// runVerify 执行一次工具调用。
func runVerify(raw json.RawMessage, resolve resolver) (any, error) {
	var args verifyArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("law_verify: 参数解析失败: %w", err)
	}
	citations := collectCitations(args)
	if len(citations) == 0 {
		return nil, fmt.Errorf("law_verify: 未找到法条引用；请提供 text，或用 law+article 指定")
	}
	findings, counts := evaluate(citations, args.Law, resolve)
	return outputFindings(findings, counts), nil
}

// resolver 把一条引用解析成核验结论。
type resolver func(c lawcite.Citation, lawName string) resolved

// makeResolver 组合精校索引、随包索引与条号骨架。
func makeResolver(indexes map[lawcite.Statute]*lawcite.Index, skeleton *lawcite.Skeleton) resolver {
	return func(c lawcite.Citation, lawName string) resolved {
		if indexes != nil {
			if idx, ok := indexes[c.Statute]; ok {
				return resolved{idx.Resolve(c), "index"}
			}
		}
		if c.Statute != lawcite.StatuteUnknown {
			if idx, err := lawcite.BundledIndex(c.Statute); err == nil {
				return resolved{idx.Resolve(c), "index"}
			}
		}
		if skeleton != nil && lawName != "" {
			return resolved{skeleton.Resolve(lawName, c.Article), "skeleton"}
		}
		return resolved{lawcite.Resolution{
			Citation: c,
			Status:   lawcite.StatusUnverified,
			Reason:   fmt.Sprintf("《%s》尚无随包索引或条号骨架，未做核验", lawName),
		}, "none"}
	}
}

// loadSkeleton 按配置与默认路径加载骨架；加载失败时按「无骨架」运行。
func loadSkeleton(cfg Config) *lawcite.Skeleton {
	if cfg.Skeleton != nil {
		return cfg.Skeleton
	}
	path := cfg.SkeletonPath
	if path == "" {
		path = defaultSkeletonPath()
	}
	if path == "" {
		return nil
	}
	skeleton, err := lawcite.LoadSkeletonFile(path)
	if err != nil {
		return nil
	}
	return skeleton
}

// evaluate 核验全部引用并计数。
func evaluate(citations []lawcite.Citation, fallbackLaw string, resolve resolver) ([]finding, map[lawcite.Status]int) {
	findings := make([]finding, 0, len(citations))
	counts := make(map[lawcite.Status]int, 5)
	for _, citation := range citations {
		lawName := fallbackLaw
		if citation.Statute != lawcite.StatuteUnknown {
			lawName = citation.Statute.String()
		}
		outcome := resolve(citation, lawName)
		counts[outcome.resolution.Status]++
		findings = append(findings, newFinding(outcome, lawName))
	}
	return findings, counts
}

// collectCitations 从文本或结构化参数收集待核验引用。
//
// 无法识别的法律名不直接报错：它可能是条号骨架里的一部地方性法规，交给骨架判定；
// 骨架也没有时 resolve 会返回未核验并说明原因。
func collectCitations(args verifyArgs) []lawcite.Citation {
	if strings.TrimSpace(args.Text) != "" {
		return lawcite.Extract(args.Text)
	}
	if strings.TrimSpace(args.Law) == "" || args.Article <= 0 {
		return nil
	}
	statute, _ := lawcite.ParseStatute(args.Law)
	return []lawcite.Citation{{
		Statute:   statute,
		Article:   args.Article,
		Paragraph: args.Paragraph,
		Item:      args.Item,
	}}
}

// finding 是单条引用的核验结果。
type finding struct {
	Citation   string   `json:"citation"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
	Source     string   `json:"source"`
	Text       string   `json:"text,omitempty"`
	SourceDoc  string   `json:"source_doc,omitempty"`
	VerifiedOn string   `json:"verified_on,omitempty"`
	Topics     []string `json:"topics,omitempty"`
}

// newFinding 把核验结论转成工具输出。
func newFinding(outcome resolved, lawName string) finding {
	res := outcome.resolution
	f := finding{
		Citation: citationLabel(res, lawName),
		Status:   res.Status.String(),
		Reason:   res.Reason,
		Source:   outcome.source,
	}
	if res.Article != nil {
		f.Text = res.Article.Text
		f.SourceDoc = res.Article.SourceDoc
		f.VerifiedOn = res.Article.VerifiedOn
		f.Topics = res.Article.Topics
	}
	return f
}

// citationLabel 生成引用标签；骨架条目没有 Statute，用法律名拼接。
func citationLabel(res lawcite.Resolution, lawName string) string {
	if res.Citation.Statute != lawcite.StatuteUnknown {
		return res.Citation.String()
	}
	if lawName != "" {
		return fmt.Sprintf("《%s》第%d条", lawName, res.Citation.Article)
	}
	return fmt.Sprintf("第%d条", res.Citation.Article)
}

// defaultSkeletonPath 探测条号骨架的默认位置。
//
// 先看 $MADY_HOME/law-skeleton/index.json.gz，再看仓库内
// data/law/skeleton/index.json.gz；都不存在时返回空字符串，工具按「无骨架」运行。
func defaultSkeletonPath() string {
	candidates := make([]string, 0, 2)
	if home, err := util.MadyHome(); err == nil {
		candidates = append(candidates, filepath.Join(home, "law-skeleton", "index.json.gz"))
	}
	candidates = append(candidates, filepath.Join("data", "law", "skeleton", "index.json.gz"))
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// outputFindings 汇总结果并生成一句概述。
func outputFindings(findings []finding, counts map[lawcite.Status]int) map[string]any {
	parts := make([]string, 0, 5)
	for _, entry := range []struct {
		status lawcite.Status
		label  string
	}{
		{lawcite.StatusVerified, "条已核验"},
		{lawcite.StatusExistOnly, "条条号存在但条文未转录"},
		{lawcite.StatusOutOfRange, "条条号越界"},
		{lawcite.StatusNarrowingMismatch, "条款项越界"},
		{lawcite.StatusUnverified, "条未核验"},
	} {
		if n := counts[entry.status]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, entry.label))
		}
	}
	summary := strings.Join(parts, "，")
	if summary == "" {
		summary = "未发现引用"
	}
	return map[string]any{
		"findings": findings,
		"summary":  summary + "。条文未转录或未核验的引用不能作为结论依据。",
	}
}
