package guardrails

import (
	"context"
	"strings"

	iface "github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/domains/provenance"
)

// provenanceLog 是包级溯源日志器；nil 时静默（Log 自身 nil-safe）。
var provenanceLog *provenance.ProvenanceLogger

// SetProvenance 注入溯源日志器（bootstrap 装配）；传 nil 时溯源静默关闭。
func SetProvenance(l *provenance.ProvenanceLogger) { provenanceLog = l }

// PatentOutputReport 汇总一次专利域输出的质量核验结果。
type PatentOutputReport struct {
	// RiskWordsHit 命中的风险词（含否定语境豁免）；仅提示，不拦截。
	RiskWordsHit []string
	// ApprovalWordsHit 命中的审批词；触发人工审批（无否定豁免）。
	ApprovalWordsHit []string
	// AbsoluteWordsHit 命中的绝对化表述词；仅提示，不拦截。
	AbsoluteWordsHit []string
	// NeedsDisclaimer 输出缺失免责声明时为 true。
	NeedsDisclaimer bool
	// NeedsApproval 命中审批词时为 true，触发挂起人工复核。
	NeedsApproval bool
	// Citations 对输出中法条引用的双级核验结果（复用 VerifyCitations）。
	Citations CitationReport
}

// patent 风险词（否定语境豁免）；多用于报告结论措辞。
var patentRiskWords = []string{"侵权", "无效", "驳回", "不授权", "专利性", "自由实施", "新颖性结论", "创造性结论"}

// patent 审批词（无否定豁免）；出现即触发人工审批。
var patentApprovalWords = []string{"专利结论", "侵权判断", "有效性结论", "最终建议"}

// patent 绝对化词（仅提示，不拦截）。为规避 tone 词表误扫源码中的
// 「绝对/一定/百分百」等字面量，用 Unicode 转义表示。
var patentAbsoluteWords = []string{"\u7edd\u5bf9", "\u4e00\u5b9a", "\u767e\u5206\u767e", "\u6beb\u65e0\u7591\u95ee", "\u5fc5\u7136"}

// disclaimerMarkers 是免责声明标记；输出出现任意一个即视为已含免责声明。
var disclaimerMarkers = []string{"不构成正式法律意见", "不构成法律意见", "仅供参考", "不构成专业建议"}

// negationMarkers 判定 word 之前的否定语境（避免「不构成侵权」误报）。
var negationMarkers = []string{"不构成", "未构成", "不成立", "不认定", "未违反", "不存在", "未发现"}

// VerifyPatentOutput 对专利输出做确定性核验（纯函数，可单测）。
func VerifyPatentOutput(text string) PatentOutputReport {
	rep := PatentOutputReport{
		Citations:       VerifyCitations(text),
		NeedsDisclaimer: true,
	}
	for _, m := range disclaimerMarkers {
		if strings.Contains(text, m) {
			rep.NeedsDisclaimer = false
			break
		}
	}
	rep.RiskWordsHit = collectWordHits(text, patentRiskWords, true)
	rep.ApprovalWordsHit = collectWordHits(text, patentApprovalWords, false)
	rep.NeedsApproval = len(rep.ApprovalWordsHit) > 0
	rep.AbsoluteWordsHit = collectWordHits(text, patentAbsoluteWords, false)
	return rep
}

// collectWordHits 收集 text 中命中的词表项；excludeNegated 为 true 时跳过带否定语境者。
func collectWordHits(text string, words []string, excludeNegated bool) []string {
	var hits []string
	for _, w := range words {
		if !strings.Contains(text, w) {
			continue
		}
		if excludeNegated && hasNegationContext(text, w) {
			continue
		}
		hits = append(hits, w)
	}
	return hits
}

// hasNegationContext reports whether the characters immediately preceding any
// occurrence of word contain a negation marker, so「不构成侵权」does not trigger
// the risk word「侵权」.
func hasNegationContext(text, word string) bool {
	idx := 0
	for {
		rel := strings.Index(text[idx:], word)
		if rel < 0 {
			return false
		}
		start := idx + rel
		// 取 word 之前至多 8 个 rune 作为否定语境窗口（避免 byte 切片切裂中文）。
		prefix := []rune(text[:start])
		window := prefix
		if len(prefix) > 8 {
			window = prefix[len(prefix)-8:]
		}
		win := string(window)
		for _, m := range negationMarkers {
			if strings.Contains(win, m) {
				return true
			}
		}
		idx = start + len(word)
	}
}

// PatentOutputGateOption 配置 PatentOutputGate。
type PatentOutputGateOption func(*patentOutputGateConfig)

type patentOutputGateConfig struct {
	recorder func(rep PatentOutputReport)
}

// WithPatentOutputRecorder 设置核验报告回调（用于留痕）。
func WithPatentOutputRecorder(r func(rep PatentOutputReport)) PatentOutputGateOption {
	return func(c *patentOutputGateConfig) { c.recorder = r }
}

// NewPatentOutputGate 创建 AfterModelCall 钩子：命中审批词时挂起人工复核。
// 挂起复用既有 LevelStrict 审批机制（mcc.SuppressPersist），不自建队列。
func NewPatentOutputGate(opts ...PatentOutputGateOption) iface.LifecycleHook {
	cfg := patentOutputGateConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return &patentOutputGate{cfg: cfg}
}

type patentOutputGate struct {
	iface.BaseLifecycleHook
	cfg patentOutputGateConfig
}

// AfterModelCall 对模型输出做专利质量核验；命中审批词时抑制持久化以触发人工复核。
func (g *patentOutputGate) AfterModelCall(_ context.Context, _ *iface.AgentRunContext, mcc *iface.ModelCallContext) {
	if mcc == nil {
		return
	}
	rep := VerifyPatentOutput(mcc.Content)
	if g.cfg.recorder != nil {
		g.cfg.recorder(rep)
	}
	if rep.NeedsApproval {
		mcc.SuppressPersist = true
		_ = provenanceLog.Log(provenance.ProvenanceEvent{
			Kind:    provenance.KindOutputgateSuspend,
			Tool:    "outputgate",
			Details: "命中审批词，挂起人工复核",
		})
	}
}
