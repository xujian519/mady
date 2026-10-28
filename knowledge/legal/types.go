// Package legal 提供法律知识库的四类检索：法规条文、司法解释、裁判案例与
// 指导性案例。数据来自部署本地的 SQLite 库（laws-full*.db 或 install 产物），
// 库不可用时降级为内存记录检索。
package legal

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Scope 是四类司法检索的记录类别。
type Scope string

// Search scope constants.
const (
	ScopeRegulation             Scope = "regulation"              // 法规条文
	ScopeJudicialInterpretation Scope = "judicial_interpretation" // 司法解释
	ScopeCaseLaw                Scope = "case_law"                // 裁判案例
	ScopeGuidingCase            Scope = "guiding_case"            // 指导性案例
)

// AllScopes 返回全部检索类别，用于工具参数枚举与错误提示。
func AllScopes() []Scope {
	return []Scope{ScopeRegulation, ScopeJudicialInterpretation, ScopeCaseLaw, ScopeGuidingCase}
}

// Valid reports whether the scope is one of the four supported queries.
func (s Scope) Valid() bool {
	switch s {
	case ScopeRegulation, ScopeJudicialInterpretation, ScopeCaseLaw, ScopeGuidingCase:
		return true
	default:
		return false
	}
}

// Label 返回类别中文名，用于结果摘要。
func (s Scope) Label() string {
	switch s {
	case ScopeRegulation:
		return "法规条文"
	case ScopeJudicialInterpretation:
		return "司法解释"
	case ScopeCaseLaw:
		return "裁判案例"
	case ScopeGuidingCase:
		return "指导性案例"
	default:
		return string(s)
	}
}

// Level 是 law.level 列的取值（法规层级）。
type Level string

// Legal level constants.
const (
	LevelConstitution             Level = "宪法"
	LevelLaw                      Level = "法律"
	LevelAdministrativeRegulation Level = "行政法规"
	LevelLocalRegulation          Level = "地方性法规"
	LevelDepartmentRule           Level = "部门规章"
	LevelJudicialInterpretation   Level = "司法解释"
	LevelCase                     Level = "案例"
	LevelOther                    Level = "其他"
)

// AllLevels 返回 law.level 的全部取值，用于工具参数枚举。
func AllLevels() []Level {
	return []Level{
		LevelConstitution, LevelLaw, LevelAdministrativeRegulation, LevelLocalRegulation,
		LevelDepartmentRule, LevelJudicialInterpretation, LevelCase, LevelOther,
	}
}

// Valid reports whether the level is a recognized law.level value.
func (l Level) Valid() bool {
	switch l {
	case LevelConstitution, LevelLaw, LevelAdministrativeRegulation, LevelLocalRegulation,
		LevelDepartmentRule, LevelJudicialInterpretation, LevelCase, LevelOther:
		return true
	default:
		return false
	}
}

// defaultRegulationLevels 是未指定层级时「法规条文」检索的范围。
// 司法解释与案例各有独立类别，不混入法规检索。
var defaultRegulationLevels = []Level{LevelLaw, LevelAdministrativeRegulation}

// Validity 是法规在检索时点的效力状态。
//
// 只有来源库明确标为未废止且带有施行日期的记录才判为 in-force；日期缺失
// 判为 unverified——把「不知道」当成「有效」会让检索结论失真。
type Validity string

// Validity constants.
const (
	ValidityInForce         Validity = "in-force"
	ValidityExpired         Validity = "expired"
	ValidityNotYetEffective Validity = "not-yet-effective"
	ValidityUnverified      Validity = "unverified"
)

// Record 是一条法律知识记录。law 表行与 guiding_cases 表行共用本结构，
// 未使用的字段留空。Records 也用于无库时的内存降级。
type Record struct {
	ID                string   `json:"id"`
	Level             Level    `json:"level,omitempty"`
	Name              string   `json:"name"`
	Subtitle          string   `json:"subtitle,omitempty"`
	Content           string   `json:"content,omitempty"`
	Category          string   `json:"category,omitempty"`
	PublishDate       string   `json:"publish_date,omitempty"`
	EffectiveDate     string   `json:"effective_date,omitempty"`
	Expired           bool     `json:"expired,omitempty"`
	GuidingCaseNumber string   `json:"guiding_case_number,omitempty"`
	Court             string   `json:"court,omitempty"`
	CaseType          string   `json:"case_type,omitempty"`
	KeyPoints         string   `json:"key_points,omitempty"`
	RelatedLaws       []string `json:"related_laws,omitempty"`
	FullText          string   `json:"full_text,omitempty"`
}

// Hit 是一条检索命中。
type Hit struct {
	Scope         Scope    `json:"scope"`
	ID            string   `json:"id"`
	Level         Level    `json:"level,omitempty"`
	Name          string   `json:"name"`
	Subtitle      string   `json:"subtitle,omitempty"`
	Content       string   `json:"content,omitempty"`
	Category      string   `json:"category,omitempty"`
	PublishDate   string   `json:"publish_date,omitempty"`
	EffectiveDate string   `json:"effective_date,omitempty"`
	Validity      Validity `json:"validity,omitempty"`
	CaseNumber    string   `json:"guiding_case_number,omitempty"`
	Court         string   `json:"court,omitempty"`
	CaseType      string   `json:"case_type,omitempty"`
	DecisionDate  string   `json:"decision_date,omitempty"`
	KeyPoints     string   `json:"key_points,omitempty"`
	RelatedLaws   []string `json:"related_laws,omitempty"`
	// RelevanceScore 越小越相关；内存降级时越大越相关。
	RelevanceScore float64 `json:"relevance_score"`
	// Via 说明命中通道：fts 全文、like 降级匹配、memory 内存记录。
	Via string `json:"via"`
}

// 检索通道与上限。
const (
	viaFTS    = "fts"
	viaLike   = "like"
	viaMemory = "memory"

	// DefaultLimit 与 MaxLimit 对齐 dsh legal_knowledge_search 的口径。
	DefaultLimit = 5
	MaxLimit     = 20

	// maxContentRunes 限制单条命中的正文长度，避免整份法规挤占上下文。
	maxContentRunes = 2000
)

// SearchOptions 是四类检索共用的选项。
type SearchOptions struct {
	// Limit 是返回条数上限（1–20，默认 5）。
	Limit int
	// Level 仅对法规检索生效；为空时检索法律与行政法规。
	Level Level
	// AsOf 是效力判定时点（YYYY-MM-DD），默认今天。
	AsOf string
	// IncludeUnverified 控制是否返回效力未核验的记录（默认 true）。
	IncludeUnverified *bool
}

// normalizeLimit 把上限收敛到 [1, MaxLimit]。
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

// resolveAsOf 返回效力判定日期；非法格式报错而不是静默按今天处理。
func resolveAsOf(asOf string) (string, error) {
	trimmed := strings.TrimSpace(asOf)
	if trimmed == "" {
		return time.Now().Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return "", fmt.Errorf("as_of 必须是 YYYY-MM-DD 格式的合法日期（收到 %q）", asOf)
	}
	return trimmed, nil
}

// includeUnverified 返回是否展示未核验记录（默认展示）。
func (o SearchOptions) includeUnverified() bool {
	if o.IncludeUnverified == nil {
		return true
	}
	return *o.IncludeUnverified
}

// resolveValidity 判定记录在 asOf 时点的效力。
//
// 规则：来源标为已废止 → expired；有施行日期且晚于 asOf → not-yet-effective；
// 有施行日期 → in-force；无施行日期 → unverified。
func resolveValidity(expired bool, effectiveDate, asOf string) Validity {
	if expired {
		return ValidityExpired
	}
	trimmed := strings.TrimSpace(effectiveDate)
	if trimmed == "" {
		return ValidityUnverified
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return ValidityUnverified
	}
	if asOf < trimmed {
		return ValidityNotYetEffective
	}
	return ValidityInForce
}

// visible 报告该效力状态的记录是否应出现在检索结果中。
// 已废止与尚未生效一律不返回；未核验由调用方决定。
func visible(v Validity, includeUnverified bool) bool {
	switch v {
	case ValidityExpired, ValidityNotYetEffective:
		return false
	case ValidityUnverified:
		return includeUnverified
	default:
		return true
	}
}

// truncateContent 按字符截断正文，超出时追加省略标记。
func truncateContent(text string) string {
	if utf8.RuneCountInString(text) <= maxContentRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxContentRunes]) + "…（已截断）"
}
