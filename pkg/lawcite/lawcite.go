// Package lawcite 提供中文专利法域文本中法条引用的结构化抽取。
//
// 本包是「评测指标（agentcore/evaluate）与线上引用核验护栏（guardrails）
// 共享的同源引用理解」（见 docs/design/citation-verification-gate.md §3 决策四）：
// 从自由文本中识别「专利法第22条第3款」「《专利法实施细则》第四十二条」
// 「细则第68条」等引用，归一化中文数字、识别所属法律、并截取引用点语境，
// 供后续存在性/语境相关性核验使用。
//
// 纯函数、零依赖，可安全用于 AfterModelCall 热路径。
package lawcite

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Statute 标识一次引用所属的法律文件。
type Statute int

const (
	// StatuteUnknown 表示无法从上下文判定所属法律（承接语境缺失或超出窗口）。
	StatuteUnknown Statute = iota

	// StatutePatentLaw 表示《中华人民共和国专利法》。
	StatutePatentLaw

	// StatuteImplementingRules 表示《专利法实施细则》（含"实施条例""细则"等简称）。
	StatuteImplementingRules

	// StatuteExamGuideline 表示《专利审查指南》。
	StatuteExamGuideline
)

// String 返回法律文件的规范简称，用于核验提示文案。
func (s Statute) String() string {
	switch s {
	case StatutePatentLaw:
		return "专利法"
	case StatuteImplementingRules:
		return "专利法实施细则"
	case StatuteExamGuideline:
		return "专利审查指南"
	default:
		return "未知法律"
	}
}

// Citation 表示一次法条引用，含所属法律、条/款/项定位与引用点语境。
type Citation struct {
	Statute   Statute // 所属法律（由承接语境归一）
	Article   int     // 条
	Paragraph int     // 款（0 = 未指明）
	Item      int     // 项（0 = 未指明）
	Suffix    int     // "之一/之二/之三"（0 = 无）
	Context   string  // 引用点前后各 contextRunes 字，供用途声明提取
	Raw       string  // 原始匹配文本（中文数字已归一为阿拉伯数字）
}

// Key 返回去重键：同一法律下同一条/款/项/之N 视为同一引用。
func (c Citation) Key() string {
	return fmt.Sprintf("%d:%d:%d:%d:%d", c.Statute, c.Article, c.Paragraph, c.Item, c.Suffix)
}

// String 返回规范化的引用文本，如 "专利法实施细则第42条第1款"。
func (c Citation) String() string {
	var b strings.Builder
	if c.Statute != StatuteUnknown {
		b.WriteString(c.Statute.String())
	}
	fmt.Fprintf(&b, "第%d条", c.Article)
	if c.Paragraph > 0 {
		fmt.Fprintf(&b, "第%d款", c.Paragraph)
	}
	if c.Item > 0 {
		fmt.Fprintf(&b, "第%d项", c.Item)
	}
	switch c.Suffix {
	case 1:
		b.WriteString("之一")
	case 2:
		b.WriteString("之二")
	case 3:
		b.WriteString("之三")
	}
	return b.String()
}

// statuteWindow 是承接语境归一的最大回溯距离（字）。
// 法律文件名出现在引用点前该距离内时，引用归属于该法律。
const statuteWindow = 120

// contextRunes 是 Citation.Context 在引用点两侧各截取的长度（字）。
const contextRunes = 40

// citationPattern 匹配「第N条[第M款][第K项][之一/之二/之三]」。
// 调用前文本中的中文数字已被 normalizeChineseNumerals 归一为阿拉伯数字。
var citationPattern = regexp.MustCompile(`第(\d+)条(?:第(\d+)款)?(?:第(\d+)项)?(之一|之二|之三)?`)

// statutePatterns 按优先级排列的法律名称模式。
// 注意顺序：实施细则/实施条例必须先于"专利法"匹配——"专利法实施细则"
// 的"专利法"前缀命中会因其与细则全称的跨度重叠而被丢弃（见
// collectStatuteMentions 的重叠过滤），故无需在模式内排除。
var statutePatterns = []struct {
	statute Statute
	pattern *regexp.Regexp
}{
	{StatuteImplementingRules, regexp.MustCompile(`专利法实施细则|专利法实施条例|实施细则|实施条例|细则`)},
	{StatuteExamGuideline, regexp.MustCompile(`专利审查指南|审查指南`)},
	{StatutePatentLaw, regexp.MustCompile(`专利法`)},
}

// statuteMention 记录一次法律名称出现的位置与归属。
type statuteMention struct {
	pos     int // 字节偏移
	statute Statute
}

// Extract 从 text 中提取全部法条引用，按出现顺序返回。
//
// 规则：
//   - 中文数字（第X条/款/项/章/节/点/部分）先归一为阿拉伯数字；
//   - 每条引用的 Statute 取其引用点前 statuteWindow 字内最近的一次法律名称
//     （即承接语境：未指明法律时沿用上文最近明确的法律），窗口外为 StatuteUnknown；
//   - 同一引用重复出现时会重复返回（各自携带自己的 Context），
//     需要唯一集合时请用 Unique。
func Extract(text string) []Citation {
	norm := normalizeChineseNumerals(text)

	locs := citationPattern.FindAllStringSubmatchIndex(norm, -1)
	if len(locs) == 0 {
		return nil
	}

	mentions := collectStatuteMentions(norm)

	// 字节偏移 → 字数偏移 的换算表（rune 边界处有效；匹配点必在边界上）。
	runes := []rune(norm)
	byteToRune := make([]int, len(norm)+1)
	bytePos := 0
	for i, r := range runes {
		bytePos += len(string(r))
		byteToRune[bytePos] = i + 1
	}

	var out []Citation
	for _, loc := range locs {
		start := byteToRune[loc[0]]
		end := byteToRune[loc[1]]
		c := Citation{
			Article:   mustAtoi(norm[loc[2]:loc[3]]),
			Paragraph: groupAtoi(norm, loc, 4),
			Item:      groupAtoi(norm, loc, 6),
			Suffix:    suffixIndex(groupText(norm, loc, 8)),
			Raw:       norm[loc[0]:loc[1]],
			Statute:   resolveStatute(mentions, byteToRune, start),
		}
		from := start - contextRunes
		if from < 0 {
			from = 0
		}
		to := end + contextRunes
		if to > len(runes) {
			to = len(runes)
		}
		c.Context = string(runes[from:to])
		out = append(out, c)
	}
	return out
}

// Unique 按 Key 去重，保留每个引用首次出现的位置与语境。
func Unique(cs []Citation) []Citation {
	seen := make(map[string]bool, len(cs))
	var out []Citation
	for _, c := range cs {
		k := c.Key()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}

// collectStatuteMentions 返回文本中全部法律名称出现位置（按字节偏移升序）。
// 同一跨度被高优先级模式覆盖时，低优先级模式的重叠命中被丢弃。
func collectStatuteMentions(s string) []statuteMention {
	type span struct{ start, end int }
	var covered []span
	var mentions []statuteMention
	for _, sp := range statutePatterns {
		for _, loc := range sp.pattern.FindAllStringIndex(s, -1) {
			overlap := false
			for _, c := range covered {
				if loc[0] < c.end && loc[1] > c.start {
					overlap = true
					break
				}
			}
			if overlap {
				continue
			}
			covered = append(covered, span{loc[0], loc[1]})
			mentions = append(mentions, statuteMention{pos: loc[0], statute: sp.statute})
		}
	}
	// 按位置升序（resolveStatute 依赖有序）。
	for i := 1; i < len(mentions); i++ {
		for j := i; j > 0 && mentions[j].pos < mentions[j-1].pos; j-- {
			mentions[j], mentions[j-1] = mentions[j-1], mentions[j]
		}
	}
	return mentions
}

// resolveStatute 返回引用点前 statuteWindow 字内最近的法律归属。
// mentions 与 byteToRune 均为字节域，此处统一换算为字数距离；
// 窗口内无命中时返回 StatuteUnknown。
func resolveStatute(mentions []statuteMention, byteToRune []int, startRune int) Statute {
	best := StatuteUnknown
	bestDist := statuteWindow + 1
	for _, m := range mentions {
		mRune := byteToRune[m.pos]
		if mRune >= startRune {
			break
		}
		if dist := startRune - mRune; dist < bestDist {
			bestDist = dist
			best = m.statute
		}
	}
	return best
}

// suffixIndex 将「之一/之二/之三」映射为 1/2/3，其余为 0。
func suffixIndex(s string) int {
	switch s {
	case "之一":
		return 1
	case "之二":
		return 2
	case "之三":
		return 3
	default:
		return 0
	}
}

// groupText 返回第 n 个子匹配的文本（未匹配时为空串）。
// n 为 FindAllStringSubmatchIndex 的下标（组号*2）。
func groupText(s string, loc []int, n int) string {
	if loc[n] < 0 {
		return ""
	}
	return s[loc[n]:loc[n+1]]
}

// groupAtoi 返回第 n 个子匹配的整数值（未匹配时为 0）。
func groupAtoi(s string, loc []int, n int) int {
	return mustAtoi(groupText(s, loc, n))
}

// mustAtoi 转换纯数字串，空串或异常返回 0（正则已保证只含数字，不会失败）。
func mustAtoi(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// ============================================================================
// 中文数字归一化（与 agentcore/evaluate 口径一致，P1c 后 metrics.go 改调本包）
// ============================================================================

var (
	cnDigits = map[rune]int{
		'〇': 0, '零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
	}
	cnUnits = map[rune]int{
		'十': 10, '百': 100, '千': 1000,
	}
	cnLawNumeralPattern = regexp.MustCompile(`第([〇零一二两三四五六七八九十百千]+)(条|款|项|章|节|点|部分)`)
)

// chineseToArabic 将中文数字串转为整数（支持到 9999，如 "二十二"→22、"一百二十三"→123）。
func chineseToArabic(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	var result, current int
	for _, r := range s {
		if digit, ok := cnDigits[r]; ok {
			current = digit
		} else if unit, ok := cnUnits[r]; ok {
			if current == 0 {
				current = 1
			}
			result += current * unit
			current = 0
		} else {
			return 0, false
		}
	}
	result += current
	return result, true
}

// normalizeChineseNumerals 仅将「第X条/款/项/章/节/点/部分」中的中文数字
// 替换为阿拉伯数字，不触碰普通中文文本。
// 例："专利法第二十二条第三款" → "专利法第22条第3款"。
func normalizeChineseNumerals(s string) string {
	return cnLawNumeralPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := cnLawNumeralPattern.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		numCn := sub[1]
		rest := match[len("第")+len(numCn):]
		if n, ok := chineseToArabic(numCn); ok {
			return "第" + strconv.Itoa(n) + rest
		}
		return match
	})
}
