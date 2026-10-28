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
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/lawcite"
)

// ToolName 是工具名。
const ToolName = "law_verify"

// Config 是工具配置。
type Config struct {
	// Indexes 覆盖随包索引；未提供的法律回退到 lawcite.BundledIndex。
	Indexes map[lawcite.Statute]*lawcite.Index
}

// NewTool 构造 law_verify 工具。
func NewTool(cfg Config) *agentcore.Tool {
	indexes := cfg.Indexes
	resolve := func(c lawcite.Citation) lawcite.Resolution {
		if indexes != nil {
			if idx, ok := indexes[c.Statute]; ok {
				return idx.Resolve(c)
			}
		}
		idx, err := lawcite.BundledIndex(c.Statute)
		if err != nil {
			return lawcite.Resolution{
				Citation: c,
				Status:   lawcite.StatusUnverified,
				Reason:   fmt.Sprintf("《%s》尚无随包条文索引，未做核验", c.Statute),
			}
		}
		return idx.Resolve(c)
	}

	return &agentcore.Tool{
		Name:        ToolName,
		Description: "核验法条引用：给出条/款/项是否存在于随包索引、条文是否已转录、来源与核验日期。可用 text 传入含引用的文本，或用 law+article 直接指定。",
		ReadOnly:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "含法条引用的文本，如「依据专利法第22条第3款」。与 law+article 二选一。",
				},
				"law": map[string]any{
					"type":        "string",
					"description": "法律简称或全称：专利法 / 专利法实施细则 / 专利审查指南 / 商标法 / 著作权法 / 反不正当竞争法。",
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
		},
		Func: func(_ context.Context, raw json.RawMessage) (any, error) {
			var args struct {
				Text      string `json:"text"`
				Law       string `json:"law"`
				Article   int    `json:"article"`
				Paragraph int    `json:"paragraph"`
				Item      int    `json:"item"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, fmt.Errorf("law_verify: 参数解析失败: %w", err)
			}

			citations, err := collectCitations(args.Text, args.Law, args.Article, args.Paragraph, args.Item)
			if err != nil {
				return nil, err
			}
			if len(citations) == 0 {
				return nil, fmt.Errorf("law_verify: 未找到法条引用；请提供 text，或用 law+article 指定")
			}

			findings := make([]finding, 0, len(citations))
			counts := map[lawcite.Status]int{}
			for _, citation := range citations {
				res := resolve(citation)
				counts[res.Status]++
				findings = append(findings, newFinding(res))
			}
			return outputFindings(findings, counts), nil
		},
	}
}

// collectCitations 从文本或结构化参数收集待核验引用。
func collectCitations(text, law string, article, paragraph, item int) ([]lawcite.Citation, error) {
	if strings.TrimSpace(text) != "" {
		return lawcite.Extract(text), nil
	}
	if strings.TrimSpace(law) == "" || article <= 0 {
		return nil, nil
	}
	statute, ok := lawcite.ParseStatute(law)
	if !ok {
		return nil, fmt.Errorf("law_verify: 无法识别法律 %q；可用简称见工具说明", law)
	}
	return []lawcite.Citation{{
		Statute:   statute,
		Article:   article,
		Paragraph: paragraph,
		Item:      item,
	}}, nil
}

// finding 是单条引用的核验结果。
type finding struct {
	Citation   string   `json:"citation"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
	Text       string   `json:"text,omitempty"`
	SourceDoc  string   `json:"source_doc,omitempty"`
	VerifiedOn string   `json:"verified_on,omitempty"`
	Topics     []string `json:"topics,omitempty"`
}

// newFinding 把核验结论转成工具输出。
func newFinding(res lawcite.Resolution) finding {
	f := finding{
		Citation: res.Citation.String(),
		Status:   res.Status.String(),
		Reason:   res.Reason,
	}
	if res.Article != nil {
		f.Text = res.Article.Text
		f.SourceDoc = res.Article.SourceDoc
		f.VerifiedOn = res.Article.VerifiedOn
		f.Topics = res.Article.Topics
	}
	return f
}

// outputFindings 汇总结果并生成一句概述。
func outputFindings(findings []finding, counts map[lawcite.Status]int) map[string]any {
	parts := make([]string, 0, 4)
	if n := counts[lawcite.StatusVerified]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d 条已核验", n))
	}
	if n := counts[lawcite.StatusExistOnly]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d 条条号存在但条文未转录", n))
	}
	if n := counts[lawcite.StatusOutOfRange]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d 条条号越界", n))
	}
	if n := counts[lawcite.StatusNarrowingMismatch]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d 条款项越界", n))
	}
	if n := counts[lawcite.StatusUnverified]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d 条未核验", n))
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
