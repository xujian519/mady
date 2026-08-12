package evaluate

import (
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/xujian519/mady/pkg/lawcite"
)

// ============================================================================

// CitationCompleteness measures what fraction of required citation identifiers
// appear in the prediction. This is essential for legal/patent workflows where
// every conclusion must trace back to specific source documents.
type CitationCompleteness struct {
	// Required is the set of citation identifiers (docIDs, article numbers,
	// etc.) that must appear in the prediction.
	Required []string
}

// Name returns "citation_completeness".
func (m CitationCompleteness) Name() string { return "citation_completeness" }

// WithCitations returns a new CitationCompleteness using the per-case citations.
func (m CitationCompleteness) WithCitations(citations []string) Metric {
	m.Required = citations
	return m
}

// Compute returns the fraction of required citations found in the prediction.
// Returns 1 when no citations are required.
func (m CitationCompleteness) Compute(prediction, _ string) float64 {
	if len(m.Required) == 0 {
		return 1
	}
	// Remove spaces to handle "第 26 条" vs "第26条" formatting variants.
	lowerPred := strings.ToLower(strings.ReplaceAll(prediction, " ", ""))
	normPred := lawcite.Normalize(lowerPred)
	predSet := extractLawCitations(normPred)

	hit := 0
	for _, c := range m.Required {
		lowerC := strings.ToLower(strings.ReplaceAll(c, " ", ""))
		normC := lawcite.Normalize(lowerC)

		matched := false
		requiredSet := extractLawCitations(normC)
		if len(requiredSet) > 0 {
			matched = citationSetMatches(requiredSet, predSet)
		}
		if !matched {
			matched = strings.Contains(lowerPred, lowerC) || strings.Contains(normPred, normC)
		}
		if matched {
			hit++
		}
	}
	return float64(hit) / float64(len(m.Required))
}

// extractLawCitations 从文本中抽取归一化法条引用键（"第22条第3款"格式）。
// P1c 起委托 pkg/lawcite.Extract——与线上引用核验 Gate（guardrails）
// 共享同一抽取源，本包不再维护私有正则与中文数字归一化副本
// （docs/design/citation-verification-gate.md §3 决策四）。
// 键不含"之一/之二/之三"后缀，保持 v0.8 基线口径不变。
func extractLawCitations(s string) map[string]bool {
	set := make(map[string]bool)
	for _, c := range lawcite.Extract(s) {
		key := "第" + strconv.Itoa(c.Article) + "条"
		if c.Paragraph > 0 {
			key += "第" + strconv.Itoa(c.Paragraph) + "款"
		}
		if c.Item > 0 {
			key += "第" + strconv.Itoa(c.Item) + "项"
		}
		set[key] = true
	}
	return set
}

// citationSetMatches reports whether the required citation set is covered by the
// prediction set. A required citation without "款" matches any pred citation
// that shares the same article prefix (e.g., "第22条" matches "第22条第3款" or
// "第22条第3款第2项"). A required citation with "款" but without "项" also matches
// a more specific pred citation that shares the same article+paragraph prefix.
func citationSetMatches(required, pred map[string]bool) bool {
	for rc := range required {
		if pred[rc] {
			return true
		}
		// Article-only required matches any paragraph/item variant.
		if !strings.Contains(rc, "款") {
			for pc := range pred {
				if strings.HasPrefix(pc, rc) {
					return true
				}
			}
			continue
		}
		// Article+paragraph required matches any item variant of the same paragraph.
		if !strings.Contains(rc, "项") {
			for pc := range pred {
				if strings.HasPrefix(pc, rc) {
					return true
				}
			}
		}
	}
	return false
}

// ============================================================================
// CitationValidity
// ============================================================================

// CitationValidityReport 是 CitationValidity 指标所需的核验汇总。
// 字段语义与 guardrails.CitationReport 对应字段一致，由装配侧注入适配器
// 完成映射，使本包不直接依赖 guardrails（evaluate 不得反向
// 引用扩展层）。
type CitationValidityReport struct {
	Total        int // 抽取到的引用总数（去重后）
	Valid        int // 存在且语境匹配
	Unknown      int // 静态表未覆盖，无法核验
	Unverifiable int // 无用途声明可核对
	Suspect      int // 张冠李戴疑点
	Invalid      int // 编号超范围疑点
}

// CitationVerifier 核验一段文本中的法条引用并返回汇总。
// 由调用方（如 cmd/mady eval 入口）注入 guardrails.VerifyCitations 适配实现。
type CitationVerifier func(text string) CitationValidityReport

// DefaultCitationVerifier 是不核验的兜底实现：全文无任何可核验引用，
// 始终返回空 report，使 Compute 返回 1（无依据扣分）。
// 装配侧应通过 SetCitationVerifier 注入真实实现。
var DefaultCitationVerifier CitationVerifier = func(_ string) CitationValidityReport { return CitationValidityReport{} }

// currentCitationVerifier 当前生效的引用核验器。
//
// 用 atomic.Pointer 存储，允许 SetCitationVerifier 与 Compute 并发安全：
// 评估 CLI（mady eval --workers N）会并发调用 Compute，
// 而装配阶段（init/main 启动期）调用 Set，二者不能有 data race。
// 设计上 Set 仅在初始化阶段调用，但 atomic 防御误用。
var currentCitationVerifier atomic.Pointer[CitationVerifier]

func init() {
	// 初始化为 DefaultCitationVerifier，避免 Load 返回 nil。
	def := DefaultCitationVerifier
	currentCitationVerifier.Store(&def)
}

// SetCitationVerifier 原子地注入引用核验实现。
// 可在任意时刻调用（含 main 初始化期和运行时），与正在执行的 Compute 无 data race。
// 传 nil 重置为 DefaultCitationVerifier。
func SetCitationVerifier(v CitationVerifier) {
	if v == nil {
		v = DefaultCitationVerifier
	}
	currentCitationVerifier.Store(&v)
}

// getCitationVerifier 返回当前核验器，供 Compute 与测试读取。
// 返回值保证非 nil（init 已设置默认值）。
func getCitationVerifier() CitationVerifier {
	return *currentCitationVerifier.Load()
}

// CitationValidity 通过与线上引用核验 Gate 同源的核验源（guardrails.VerifyCitations
// 的 R1 存在性 + R2 语境相关性，见 docs/design/citation-verification-gate.md §8）
// 评分法条引用的可信度。
//
// 得分 = Valid 引用数 ÷ 可核验引用数（Unknown/Unverifiable 不计入分母——
// 静态表未覆盖或无用途声明的引用既不加分也不扣分，与 Gate 的放行语义一致）。
// 全文无任何可核验引用时得 1（无依据扣分）。
//
// 默认走 DefaultCitationVerifier（不核验，返回 1），调用方应在装配阶段通过
// SetCitationVerifier 注入真实实现（如 guardrails.VerifyCitations 经类型适配后）。
type CitationValidity struct{}

// Name returns "citation_validity".
func (m CitationValidity) Name() string { return "citation_validity" }

// Compute returns the fraction of verifiable citations that are valid.
func (m CitationValidity) Compute(prediction, _ string) float64 {
	report := getCitationVerifier()(prediction)
	verifiable := report.Total - report.Unknown - report.Unverifiable
	if verifiable <= 0 {
		return 1
	}
	return float64(report.Valid) / float64(verifiable)
}

// ============================================================================
// LengthScore
