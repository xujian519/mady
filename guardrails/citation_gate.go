package guardrails

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/lawcite"
)

// 本文件实现「法条引用核验 Gate」（docs/design/citation-verification-gate.md）。
//
// 它在 AfterModelCall 相位对模型输出中的法条引用做双级核验：
//   - R1 存在性：条号是否在法条有效范围内（拦凭空捏造的编号）；
//   - R2 语境相关性：答案对该条的用途声明与该条注册主题是否匹配
//     （拦张冠李戴型幻觉，如把分案申请依据错引为专利法第 47 条）。
//
// 误报防线（设计 §6）：静态表未覆盖（Unknown）或无用途声明可核对
// （Unverifiable）的引用一律放行；只有「明确声明用途 + 明确不匹配」
// 或「编号超范围」才触发处置。处置措辞为"存疑提示"，判断权留给专业人。
//
// P1b 范围：Light/Standard 处置（追加提示 + 可选 Recorder 留痕回调）。
// Strict 档的 SuppressPersist + ApprovalGate 联动在 P2 接入，
// 当前 Strict 按 Standard 行为处理。

// CitationVerdict 是单条法条引用的核验结论。
type CitationVerdict int

const (
	// VerdictValid 存在且语境匹配。
	VerdictValid CitationVerdict = iota

	// VerdictUnknown 静态表未覆盖该法条，无法核验，放行。
	VerdictUnknown

	// VerdictUnverifiable 引用处无用途声明可核对，放行。
	VerdictUnverifiable

	// VerdictSuspect 法条存在但用途声明与注册主题不匹配（张冠李戴疑点）。
	VerdictSuspect

	// VerdictInvalid 条号超出法条有效范围（幻觉编号疑点）。
	VerdictInvalid
)

// FlaggedCitation 是一条被标记的引用及其判定依据。
type FlaggedCitation struct {
	Citation lawcite.Citation // 被标记的引用（含语境）
	Verdict  CitationVerdict  // VerdictSuspect 或 VerdictInvalid
	Reason   string           // 人类可读的判定依据（写入提示文案）
}

// CitationReport 是一次输出核验的汇总，供 Recorder 留痕。
type CitationReport struct {
	Total   int               // 抽取到的引用总数（去重后）
	Flagged []FlaggedCitation // 被标记的引用

	// 各判定计数（P1c 新增，供 citation_validity 指标计分）：
	// 五者之和等于 Total；Flagged 即 Suspect+Invalid 的明细。
	Valid        int // 存在且语境匹配
	Unknown      int // 静态表未覆盖，无法核验
	Unverifiable int // 无用途声明可核对
	Suspect      int // 张冠李戴疑点
	Invalid      int // 编号超范围疑点
}

// CitationGateConfig 配置引用核验 Gate。
type CitationGateConfig struct {
	Level Level

	// Recorder 在 Level ≥ Standard 且存在被标记引用时回调，
	// 供 disclosure 等留痕系统消费（P2 接线；nil 时仅追加提示文案）。
	Recorder func(CitationReport)
}

// CitationGateOption 是 CitationGate 的函数式选项。
type CitationGateOption func(*CitationGateConfig)

// WithCitationGateLevel 设置核验处置等级。
func WithCitationGateLevel(l Level) CitationGateOption {
	return func(c *CitationGateConfig) { c.Level = l }
}

// WithCitationRecorder 设置核验报告留痕回调（Level ≥ Standard 生效）。
func WithCitationRecorder(r func(CitationReport)) CitationGateOption {
	return func(c *CitationGateConfig) { c.Recorder = r }
}

// NewCitationGate 创建引用核验 LifecycleHook。
func NewCitationGate(opts ...CitationGateOption) agentcore.LifecycleHook {
	cfg := CitationGateConfig{Level: LevelLight}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &citationGate{config: cfg}
}

type citationGate struct {
	agentcore.BaseLifecycleHook
	config CitationGateConfig
}

// AfterModelCall 核验模型输出中的法条引用并按等级处置。
// 工具调用回合（Response.ToolCalls 非空）不是面向用户的答案，直接跳过，
// 避免污染工具循环。
func (g *citationGate) AfterModelCall(_ context.Context, _ *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if mcc == nil || mcc.Response == nil || mcc.Err != nil {
		return
	}
	if len(mcc.Response.ToolCalls) > 0 {
		return
	}
	content := mcc.Response.Content
	if content == "" {
		return
	}

	report := VerifyCitations(content)
	if len(report.Flagged) == 0 {
		return
	}

	// Light 及以上：追加存疑提示（Strict 在 P2 前与 Standard 同行为）。
	mcc.Response.Content = content + FormatCitationWarnings(report)

	// Standard 及以上：留痕回调。
	if g.config.Level >= LevelStandard && g.config.Recorder != nil {
		g.config.Recorder(report)
	}
}

// VerifyCitations 对文本中的法条引用做 R1/R2 双级核验。
// 导出供评测指标（citation_validity）与回放脚本复用。
func VerifyCitations(text string) CitationReport {
	citations := lawcite.Unique(lawcite.Extract(text))
	report := CitationReport{Total: len(citations)}
	for _, c := range citations {
		verdict, reason := verifyOne(c)
		switch verdict {
		case VerdictValid:
			report.Valid++
		case VerdictUnknown:
			report.Unknown++
		case VerdictUnverifiable:
			report.Unverifiable++
		case VerdictSuspect:
			report.Suspect++
		case VerdictInvalid:
			report.Invalid++
		}
		if verdict == VerdictSuspect || verdict == VerdictInvalid {
			report.Flagged = append(report.Flagged, FlaggedCitation{
				Citation: c, Verdict: verdict, Reason: reason,
			})
		}
	}
	return report
}

// verifyOne 核验单条引用，返回判定与依据。
func verifyOne(c lawcite.Citation) (CitationVerdict, string) {
	topics, maxArticle := citationTopics(c.Statute)
	if topics == nil {
		// 法律归属未知或该法无主题表 → 无法核验。
		return VerdictUnknown, ""
	}

	// R1 存在性。
	if maxArticle > 0 && c.Article > maxArticle {
		return VerdictInvalid, fmt.Sprintf("编号超出《%s》有效范围（共 %d 条）",
			c.Statute.String(), maxArticle)
	}

	// R2 语境相关性。
	keywords, ok := topics[c.Article]
	if !ok {
		return VerdictUnknown, ""
	}
	purpose, display, trailing := extractPurpose(c)
	if purposeEmpty(purpose) || isEnumeration(trailing) {
		return VerdictUnverifiable, ""
	}
	for _, kw := range keywords {
		if strings.Contains(purpose, kw) {
			return VerdictValid, ""
		}
	}
	// 本条主题未命中时，Suspect 需要更强的证据：用途描述命中了
	// **另一条**的注册主题（交叉匹配）。仅"本条没命中"判 Unverifiable——
	// 宽松转述（如把第 33 条说成"关于专利权范围的变更"）一律放行，
	// 这是回放校准出的关键误报防线。
	crossStatute, crossArticle, crossKW := crossMatchTopics(c.Statute, c.Article, purpose)
	if crossKW == "" {
		return VerdictUnverifiable, ""
	}
	return VerdictSuspect, fmt.Sprintf("用途描述（%s）与本条注册主题（%s）不一致，更接近《%s》第%d条的主题（%s）",
		truncateRunes(display, 20), strings.Join(keywords, "、"),
		crossStatute.String(), crossArticle, crossKW)
}

// enumStarters 是枚举接续符：引用紧随其后出现另一个引用时，
// 用途声明属于整个引用列表而非本条，R2 无法判定（放行）。
// 例："专利法第22条、第23条、第26条……所列情形不予驳回"。
// 注意：逗号不在其列——"根据专利法第X条，<用途声明>"是标准句式。
var enumStarters = []string{"、", "或", "及", "和"}

// isEnumeration 判断引用后置文本是否以枚举接续符开头。
func isEnumeration(trailing string) bool {
	t := strings.TrimLeft(trailing, " \t（）()*")
	for _, s := range enumStarters {
		if strings.HasPrefix(t, s) {
			return true
		}
	}
	return false
}

// crossMatchNoise 是交叉匹配噪声词表：这些词虽属某条注册主题（本条
// 自证有效），但过于泛化（"审查""使用""公告"等出现在大量无关语境），
// 用于交叉匹配会把正确引用误判为张冠李戴。交叉匹配只信高区分度主题词。
var crossMatchNoise = map[string]bool{
	"实施": true, "使用": true, "许可": true, "公告": true, "决定": true,
	"审查": true, "放弃": true, "请求": true, "转让": true, "撤回": true,
	"检索": true, "制造": true, "销售": true, "进口": true, "支持": true,
	"定义": true, "补偿": true, "年费": true, "公布": true, "副本": true,
}

// crossMatchTopics 在全部主题表中查找命中 purpose 的**另一条**注册主题，
// 返回其法律、条号与命中的关键词；无命中时 crossKW 为空。
// 噪声词表中的泛化词不参与交叉匹配。
func crossMatchTopics(selfStatute lawcite.Statute, selfArticle int, purpose string) (crossStatute lawcite.Statute, crossArticle int, crossKW string) {
	for _, statute := range []lawcite.Statute{lawcite.StatutePatentLaw, lawcite.StatuteImplementingRules} {
		topics, _ := citationTopics(statute)
		for article, keywords := range topics {
			if statute == selfStatute && article == selfArticle {
				continue
			}
			for _, kw := range keywords {
				if crossMatchNoise[kw] {
					continue
				}
				// "无效宣告"特例：当被核验条本身可作为无效宣告理由
				// （invalidationGrounds）时，用途描述中的"无效宣告"是同位命名
				// ——"无效宣告理由（专利法第X条）"中第 X 条即被新增的理由本身——
				// 而非把无效宣告程序张冠李戴给第 X 条，不构成交叉匹配证据。
				// （v0.8 回放 patent_exam_2009_a2_02 误报校准。）
				if kw == "无效宣告" && selfStatute == lawcite.StatutePatentLaw && invalidationGrounds[selfArticle] {
					continue
				}
				if strings.Contains(purpose, kw) {
					return statute, article, kw
				}
			}
		}
	}
	return 0, 0, ""
}

// extractPurpose 提取引用的用途声明文本，返回（匹配文本, 展示文本）。
//
// 中文法律写作的用途声明有两种语序，只取一侧必然误报：
//   - 后置式："根据专利法第47条（分案申请）…"、"专利法第22条第3款规定的创造性…"
//   - 前置式："权利要求得不到说明书支持，不符合专利法第26条第4款"、
//     "被宣告无效的专利权视为自始不存在（专利法第47条）"
//
// 因此匹配文本 = 引用点前子句 + 引用点后子句（均在句界内截取，
// 防止上一句的话题串扰）；展示文本优先取后置子句（ hallucination
// 案例中用途声明通常紧跟引用）。
func extractPurpose(c lawcite.Citation) (match, display, trailing string) {
	idx := strings.Index(c.Context, c.Raw)
	if idx < 0 {
		return "", "", ""
	}
	leading := c.Context[:idx]
	trailing = c.Context[idx+len(c.Raw):]

	// 前置子句：最后一个句界符（。；换行）之后到引用点。
	if cut := strings.LastIndexAny(leading, "。；\n"); cut >= 0 {
		leading = leading[cut+1:]
	}
	// 后置子句：引用点之后到第一个句界符（。换行）。
	rawTrailing := trailing
	if cut := strings.IndexAny(trailing, "。\n"); cut >= 0 {
		trailing = trailing[:cut]
	}

	leading = strings.TrimSpace(leading)
	trailing = strings.TrimSpace(trailing)
	display = trailing
	if display == "" {
		display = leading
	}
	return leading + " " + trailing, display, rawTrailing
}

// purposeConnectors 是引用句式中的常见连接语，本身不构成用途声明。
var purposeConnectors = []string{
	"专利法实施细则", "专利法实施条例", "实施细则", "实施条例", "专利法", "审查指南", "细则",
	"根据", "依据", "按照", "依照", "参照", "符合", "违反", "详见", "参见",
	"的相关规定", "的规定", "规定", "所述", "要求",
}

// purposeEmpty 判断匹配文本在剔除法律名称与连接语后是否还有实质内容
// （以是否残留汉字为准）。无实质内容 → VerdictUnverifiable（放行），
// 这是"专利法第36条"这类裸引用不被误报的关键。
func purposeEmpty(match string) bool {
	s := match
	for _, conn := range purposeConnectors {
		s = strings.ReplaceAll(s, conn, "")
	}
	for _, r := range s {
		if r >= '一' && r <= '鿿' {
			return false
		}
	}
	return true
}

// truncateRunes 按字截断，超长追加省略号。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// FormatCitationWarnings 渲染存疑提示（对照 tone-style-guide：
// 用"请人工核对"的存疑措辞，给出判定依据，不断言错误）。
func FormatCitationWarnings(report CitationReport) string {
	var b strings.Builder
	b.WriteString("\n\n---\n⚠️ 引用核验提示（以下法条引用请人工核对）：")
	for _, f := range report.Flagged {
		fmt.Fprintf(&b, "\n- 「%s」：%s", f.Citation.String(), f.Reason)
	}
	return b.String()
}
