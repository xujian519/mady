package generate

import (
	"regexp"
	"strings"
)

// DefaultSVGMaxBytes 是 SVG 输入/产物的大小上限（字节）。
const DefaultSVGMaxBytes = 2_000_000

var (
	svgRootRE  = regexp.MustCompile(`(?is)<svg[\s>]`)
	svgEndRE   = regexp.MustCompile(`(?i)</svg>`)
	textTagRE  = regexp.MustCompile(`(?is)<text\b(?:"[^"]*"|'[^']*'|[^>"'])*>.*?</text>`)
	stripTagRE = regexp.MustCompile(`<[^>]*>`)
	decimalRE  = regexp.MustCompile(`&#(\d+);`)
)

// AssertSafeSVG 校验 SVG 文本：拒绝 ENTITY/CDATA（防实体膨胀）、超限与缺少
// 根元素的输入。Graphviz 自带的 `<!DOCTYPE svg PUBLIC …>` 不含实体声明，放行。
func AssertSafeSVG(svg string, maxBytes int) error {
	lower := strings.ToLower(svg)
	if strings.Contains(lower, "<!entity") || strings.Contains(lower, "<![cdata[") {
		return newError(ErrUnsafeSVG, "SVG 包含 ENTITY/CDATA 等不安全结构，拒绝处理")
	}
	if maxBytes > 0 && len(svg) > maxBytes {
		return newError(ErrTooLarge, "SVG 过大（>%d 字节），拒绝处理", maxBytes)
	}
	if !svgRootRE.MatchString(svg) || !svgEndRE.MatchString(svg) {
		return newError(ErrInvalidSVG, "非 SVG 文档：缺少 <svg> 根元素")
	}
	return nil
}

// Reference 是一条标注参考：label 子串匹配图内文本，numeral 追加在末尾。
type Reference struct {
	Label   string `json:"label"`
	Numeral string `json:"numeral"`
}

// AnnotatedSVG 是标注结果。
type AnnotatedSVG struct {
	SVG string `json:"svg"`
	// Warnings 列出未在任何文本元素中命中的参考 label。
	Warnings []string `json:"warnings,omitempty"`
}

// AnnotateSVG 在 SVG 文本元素的匹配文本末尾追加 ` (numeral)` 参考标号。
//
// 每个 <text> 至多命中一个参考（按传入顺序取首个匹配）；同一参考可命中多个
// 文本元素（同件在图内多处出现时同号标注）。未命中的参考作为 warning 返回，
// 不静默吞掉——图面标号缺失需要可追溯。
func AnnotateSVG(svg string, references []Reference, maxBytes int) (AnnotatedSVG, error) {
	if err := AssertSafeSVG(svg, maxBytes); err != nil {
		return AnnotatedSVG{}, err
	}
	for _, reference := range references {
		if strings.TrimSpace(reference.Label) == "" {
			return AnnotatedSVG{}, newError(ErrInvalidLabel, "参考 label 不能为空")
		}
		if strings.TrimSpace(reference.Numeral) == "" {
			return AnnotatedSVG{}, newError(ErrInvalidLabel, "参考 %q 的 numeral 不能为空", reference.Label)
		}
	}
	if len(references) == 0 {
		return AnnotatedSVG{SVG: svg}, nil
	}

	lowered := make([]string, len(references))
	for i, reference := range references {
		lowered[i] = strings.ToLower(strings.TrimSpace(reference.Label))
	}

	var builder strings.Builder
	cursor := 0
	matched := make([]bool, len(references))
	for _, loc := range textTagRE.FindAllStringIndex(svg, -1) {
		whole := svg[loc[0]:loc[1]]
		content := strings.ToLower(textContent(whole))
		hit := -1
		for i, label := range lowered {
			if strings.Contains(content, label) {
				hit = i
				break
			}
		}
		if hit < 0 {
			continue
		}
		matched[hit] = true
		suffix := " (" + escapeXMLText(strings.TrimSpace(references[hit].Numeral)) + ")"
		insertAt := strings.LastIndex(whole, "</tspan>")
		if insertAt < 0 {
			insertAt = strings.LastIndex(whole, "</text>")
		}
		if insertAt < 0 {
			continue
		}
		builder.WriteString(svg[cursor:loc[0]])
		builder.WriteString(whole[:insertAt])
		builder.WriteString(suffix)
		builder.WriteString(whole[insertAt:])
		cursor = loc[1]
	}
	builder.WriteString(svg[cursor:])

	var warnings []string
	for i, reference := range references {
		if !matched[i] {
			warnings = append(warnings, reference.Label)
		}
	}
	return AnnotatedSVG{SVG: builder.String(), Warnings: warnings}, nil
}

// textContent 提取文本元素中的可见文本（剥离子标签并解码常见实体）。
func textContent(textElement string) string {
	inner := stripTagRE.ReplaceAllString(textElement, "")
	return decodeEntities(inner)
}

func decodeEntities(text string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
	)
	decoded := replacer.Replace(text)
	return decimalRE.ReplaceAllStringFunc(decoded, func(entity string) string {
		match := decimalRE.FindStringSubmatch(entity)
		if match == nil {
			return entity
		}
		var code int
		for _, digit := range match[1] {
			code = code*10 + int(digit-'0')
		}
		return string(rune(code))
	})
}

func escapeXMLText(text string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(text)
}
