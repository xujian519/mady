package evaluate

import (
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/xujian519/mady/pkg/lawcite"
)

// Metric scores a single prediction against a reference answer, returning a
// value in [0,1] where 1 is best. Implementations must be deterministic for a
// given (prediction, reference) pair so that results are reproducible.
// Metric scores a single prediction against a reference answer, returning a
// value in [0,1] where 1 is best. Implementations must be deterministic for a
// given (prediction, reference) pair so that results are reproducible.
type Metric interface {
	// Name is the metric identifier used in reports and aggregate maps.
	Name() string
	// Compute returns the score in [0,1].
	Compute(prediction, reference string) float64
}

// CitationAwareMetric is a Metric that can accept per-case required citations.
type CitationAwareMetric interface {
	Metric
	// WithCitations returns a new metric instance that uses the given per-case
	// required citations instead of any default set at construction time.
	WithCitations(citations []string) Metric
}
type MetricFunc struct {
	MetricName string
	Run        func(prediction, reference string) float64
}

func (m MetricFunc) Name() string                { return m.MetricName }
func (m MetricFunc) Compute(p, r string) float64 { return m.Run(p, r) }

// ============================================================================
// ExactMatch
// ============================================================================

// ExactMatch scores 1 when prediction equals reference (after optional
// case-folding and whitespace trimming), 0 otherwise.
type ExactMatch struct {
	CaseSensitive bool
}

func (m ExactMatch) Name() string { return "exact_match" }

func (m ExactMatch) Compute(prediction, reference string) float64 {
	p := strings.TrimSpace(prediction)
	r := strings.TrimSpace(reference)
	if !m.CaseSensitive {
		p = strings.ToLower(p)
		r = strings.ToLower(r)
	}
	if p == r {
		return 1
	}
	return 0
}

// ============================================================================
// F1Score (token-level)
// ============================================================================

// F1Score computes token-level precision, recall, and their harmonic mean.
// Tokenization is rune-based (single-character tokens) so it works for both
// Chinese and English text without an external tokenizer.
type F1Score struct{}

func (F1Score) Name() string { return "f1" }

func (F1Score) Compute(prediction, reference string) float64 {
	predTokens := tokenize(prediction)
	refTokens := tokenize(reference)
	if len(predTokens) == 0 && len(refTokens) == 0 {
		return 1
	}
	if len(predTokens) == 0 || len(refTokens) == 0 {
		return 0
	}

	refCounts := make(map[string]int, len(refTokens))
	for _, t := range refTokens {
		refCounts[t]++
	}

	var overlap int
	predCounts := make(map[string]int, len(predTokens))
	for _, t := range predTokens {
		predCounts[t]++
	}
	for t, pc := range predCounts {
		if rc := refCounts[t]; rc < pc {
			overlap += rc
		} else {
			overlap += pc
		}
	}
	if overlap == 0 {
		return 0
	}
	precision := float64(overlap) / float64(len(predTokens))
	recall := float64(overlap) / float64(len(refTokens))
	return 2 * precision * recall / (precision + recall)
}

// ============================================================================
// KeywordRecall
// ============================================================================

// KeywordRecall measures what fraction of the reference's keywords appear in
// the prediction. Keywords are extracted from the reference via [ExtractKeywords]
// unless an explicit keyword set is provided.
type KeywordRecall struct {
	// Keywords, when non-empty, overrides automatic extraction.
	Keywords []string
}

func (m KeywordRecall) Name() string { return "keyword_recall" }

func (m KeywordRecall) Compute(prediction, reference string) float64 {
	keywords := m.Keywords
	if len(keywords) == 0 {
		keywords = ExtractKeywords(reference)
	}
	if len(keywords) == 0 {
		return 1
	}
	lower := strings.ToLower(prediction)
	hit := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hit++
		}
	}
	return float64(hit) / float64(len(keywords))
}

// ============================================================================
// CitationCompleteness
// ============================================================================

// CitationCompleteness measures what fraction of required citation identifiers
// appear in the prediction. This is essential for legal/patent workflows where
// every conclusion must trace back to specific source documents.
type CitationCompleteness struct {
	// Required is the set of citation identifiers (docIDs, article numbers,
	// etc.) that must appear in the prediction.
	Required []string
}

func (m CitationCompleteness) Name() string { return "citation_completeness" }

// WithCitations returns a new CitationCompleteness using the per-case citations.
func (m CitationCompleteness) WithCitations(citations []string) Metric {
	m.Required = citations
	return m
}

func (m CitationCompleteness) Compute(prediction, _ string) float64 {
	if len(m.Required) == 0 {
		return 1
	}
	lowerPred := strings.ToLower(prediction)
	normPred := lawcite.Normalize(lowerPred)
	predSet := extractLawCitations(normPred)

	hit := 0
	for _, c := range m.Required {
		lowerC := strings.ToLower(c)
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
// 完成映射，使本包不直接依赖 guardrails（agentcore/evaluate 不得反向
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

func (m CitationValidity) Name() string { return "citation_validity" }

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
// ============================================================================

// LengthScore rewards predictions whose rune length falls within an acceptable
// band. This discourages both terse non-answers and rambling outputs. The score
// is triangular: 1 inside [Min, Ideal], linearly decaying toward 0 outside.
type LengthScore struct {
	Min   int // minimum acceptable length (runes)
	Ideal int // length at which the score is 1.0
	Max   int // maximum acceptable length (runes)
}

// DefaultLengthScore returns a LengthScore tuned for paragraph-length answers.
func DefaultLengthScore() LengthScore {
	return LengthScore{Min: 50, Ideal: 500, Max: 3000}
}

func (m LengthScore) Name() string { return "length_score" }

func (m LengthScore) Compute(prediction, _ string) float64 {
	n := runeLen(prediction)
	min := m.Min
	if min <= 0 {
		min = 50
	}
	ideal := m.Ideal
	if ideal <= 0 {
		ideal = 500
	}
	max := m.Max
	if max <= 0 {
		max = 3000
	}
	if n < min {
		return float64(n) / float64(min)
	}
	if n > max {
		if max <= 0 {
			return 0
		}
		excess := n - max
		decayWindow := max / 2
		if decayWindow <= 0 {
			return 0
		}
		score := 1 - float64(excess)/float64(decayWindow)
		if score < 0 {
			return 0
		}
		return score
	}
	if n <= ideal {
		return float64(n-min) / float64(ideal-min)
	}
	return float64(max-n) / float64(max-ideal)
}

// ============================================================================
// Helpers
// ============================================================================

// tokenize splits text into single-rune tokens (lowercased), skipping
// whitespace and common punctuation. This is a deliberately simple tokenizer
// that works adequately for both Chinese and English F1 computation.
func tokenize(s string) []string {
	var tokens []string
	for _, r := range strings.ToLower(s) {
		if isSkipRune(r) {
			continue
		}
		tokens = append(tokens, string(r))
	}
	return tokens
}

func isSkipRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	case ',', '.', '!', '?', ';', ':', '"', '\'', '`':
		return true
	case '\uff0c', '\u3002', '\uff01', '\uff1f', '\uff1b', '\uff1a', '\u201c', '\u201d', '\u2018', '\u2019', '\u3001':
		return true
	case '(', ')', '[', ']', '{', '}', '\uff08', '\uff09', '\u3010', '\u3011':
		return true
	}
	return false
}

// ExtractKeywords pulls salient terms from a reference string. It splits on
// delimiters and keeps tokens of at least 2 runes, deduplicating the result.
// This is a heuristic fallback for KeywordRecall when no explicit keyword set
// is available.
func ExtractKeywords(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '，' ||
			r == ';' || r == '；' || r == '|' || r == '、' || r == '。'
	})
	seen := make(map[string]bool, len(fields))
	var result []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if runeLen(f) < 2 || seen[f] {
			continue
		}
		seen[f] = true
		result = append(result, f)
	}
	return result
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// EvidenceGroundedness measures what fraction of evidence IDs cited in the
// prediction are valid (i.e. appear in the known evidence set). This catches
// hallucinated citations — a core risk in patent novelty assessment where a
// conclusion may reference a non-existent prior-art document.
//
// A prediction that cites no evidence scores 0 (ungrounded), not 1: the metric
// rewards evidence-backed conclusions. A prediction citing only valid IDs scores 1.
// (对齐 docs/specs/design-prior-art-retrieval-stage.md 第四节.)
type EvidenceGroundedness struct {
	// ValidEvidence is the set of known-valid evidence IDs (docIDs) available
	// to the model. Citations outside this set are treated as hallucinated.
	ValidEvidence []string
}

func (m EvidenceGroundedness) Name() string { return "evidence_groundedness" }

// WithCitations returns a new EvidenceGroundedness using the per-case valid
// evidence IDs. This lets the same metric instance adapt to each case's
// retrieved evidence set.
func (m EvidenceGroundedness) WithCitations(citations []string) Metric {
	m.ValidEvidence = citations
	return m
}

// Compute extracts cited evidence IDs from the prediction and returns the
// fraction that are valid. Returns 0 when no citations are present (ungrounded)
// and 1 when all citations are valid.
func (m EvidenceGroundedness) Compute(prediction, _ string) float64 {
	cited := extractCitedIDs(prediction)
	if len(cited) == 0 {
		return 0 // no evidence cited → ungrounded
	}
	if len(m.ValidEvidence) == 0 {
		return 0 // no valid evidence set available → cannot ground
	}
	validSet := make(map[string]bool, len(m.ValidEvidence))
	for _, e := range m.ValidEvidence {
		validSet[e] = true
	}
	valid := 0
	for c := range cited {
		if validSet[c] {
			valid++
		}
	}
	return float64(valid) / float64(len(cited))
}

// RuleComplianceCompleteness measures what fraction of confirmed rules are
// actually referenced in the prediction. This catches the "confirmed but
// ignored" gap: a rule may pass human review yet never surface in the final
// output, indicating the workflow skipped a required check.
// (对齐 docs/specs/design-rule-acquisition-stage.md 第五节.)
type RuleComplianceCompleteness struct {
	// Required is the set of confirmed rule IDs (e.g. NOV-001, A22.2) that
	// must be referenced in the prediction.
	Required []string
}

func (m RuleComplianceCompleteness) Name() string { return "rule_compliance_completeness" }

// WithCitations returns a new RuleComplianceCompleteness using the per-case
// confirmed rule set.
func (m RuleComplianceCompleteness) WithCitations(citations []string) Metric {
	m.Required = citations
	return m
}

// Compute returns the fraction of required rule IDs found in the prediction.
// Returns 1 when Required is empty (no rules to check → trivially complete).
func (m RuleComplianceCompleteness) Compute(prediction, _ string) float64 {
	if len(m.Required) == 0 {
		return 1
	}
	lowerPred := strings.ToLower(prediction)
	hit := 0
	for _, r := range m.Required {
		if strings.Contains(lowerPred, strings.ToLower(r)) {
			hit++
		}
	}
	return float64(hit) / float64(len(m.Required))
}

// extractCitedIDs pulls bracketed/doc-id-style identifiers from text.
// Recognizes patterns like "[CN001]", "doc_id: CN001", "CN001" bare tokens
// that match typical evidence-ID formats (alphanumeric with optional prefix).
var citedIDPattern = regexp.MustCompile(`(?:doc_id[:\s]*|\[)([A-Za-z0-9_\-]{3,})(?:\])?`)

func extractCitedIDs(text string) map[string]bool {
	matches := citedIDPattern.FindAllStringSubmatch(text, -1)
	out := make(map[string]bool, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out[m[1]] = true
		}
	}
	return out
}
