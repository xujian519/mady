// Package deliver 渲染专利律师交付物模板，并对成品做合规检查与交付件一致性核对。
//
// 模板是随包分发的受控 HTML：按 id 标注可注入的容器（如 claims-body），品牌通过
// CSS 变量注入，段落编号由模板上的 data-paragraph-numbering 声明。引擎用 DOM 解析
// 而不是字符串拼接，避免注入内容里的标签被误当模板结构。
package deliver

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

//go:embed data/templates/patent
var templateFS embed.FS

const templateRoot = "data/templates/patent"

// Brand 是交付物的品牌变量。
type Brand struct {
	// Firm 是事务所名称。
	Firm string
	// Confidential 是密级标记。
	Confidential string
	// Disclaimer 是免责声明。
	Disclaimer string
	// Accent 是主题色（如 #1f3a5f）。
	Accent string
}

// RenderOptions 是渲染选项。
type RenderOptions struct {
	// Brand 覆盖模板的 CSS 变量。
	Brand Brand
	// SkipNumbering 关闭段落编号（默认应用模板声明）。
	SkipNumbering bool
}

// manifest 是模板清单。
type manifest struct {
	Templates       []string `json:"templates"`
	InternalRecords []string `json:"internalRecords"`
}

// Templates 返回模板 id 列表（按 manifest 声明顺序）。
func Templates() ([]string, error) {
	data, err := templateFS.ReadFile(templateRoot + "/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("deliver: 读取模板清单: %w", err)
	}
	var parsed manifest
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("deliver: 解析模板清单: %w", err)
	}
	return parsed.Templates, nil
}

// IsInternalRecord 返回该模板是否记录内部工作内容（合规检查范围随之变化）。
func IsInternalRecord(id string) bool {
	data, err := templateFS.ReadFile(templateRoot + "/manifest.json")
	if err != nil {
		return false
	}
	var parsed manifest
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false
	}
	return slices.Contains(parsed.InternalRecords, id)
}

// Render 渲染一份交付物 HTML。
//
// slots 的键是模板里带 id 的容器，值是 HTML 片段；未提供的容器保持模板原样。
func Render(templateID string, slots map[string]string, opts RenderOptions) (string, error) {
	path := fmt.Sprintf("%s/%s/assets/template.html", templateRoot, templateID)
	data, err := templateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("deliver: 模板 %s 不存在: %w", templateID, err)
	}
	document, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("deliver: 解析模板 %s: %w", templateID, err)
	}

	if err := injectBrand(document, opts.Brand); err != nil {
		return "", err
	}
	for id, fragment := range slots {
		target := findElementByID(document, id)
		if target == nil {
			return "", fmt.Errorf("deliver: 模板 %s 没有 id=%s 的容器", templateID, id)
		}
		if err := replaceChildren(target, fragment); err != nil {
			return "", fmt.Errorf("deliver: 注入 %s: %w", id, err)
		}
	}
	if !opts.SkipNumbering {
		applyNumbering(document)
	}

	var buffer bytes.Buffer
	if err := html.Render(&buffer, document); err != nil {
		return "", fmt.Errorf("deliver: 输出 HTML: %w", err)
	}
	return "<!doctype html>\n" + strings.TrimPrefix(buffer.String(), "<!DOCTYPE html>\n"), nil
}

// injectBrand 在 head 末尾追加 :root 变量覆盖块。
func injectBrand(document *html.Node, brand Brand) error {
	style := brandStylesheet(brand)
	if style == "" {
		return nil
	}
	head := findElement(document, "head")
	if head == nil {
		return nil
	}
	styleNode, err := html.ParseFragment(strings.NewReader("<style>"+style+"</style>"), head)
	if err != nil {
		return fmt.Errorf("deliver: 构造品牌样式: %w", err)
	}
	for _, node := range styleNode {
		head.AppendChild(node)
	}
	return nil
}

// brandStylesheet 生成品牌 CSS 变量。
func brandStylesheet(brand Brand) string {
	variables := make([]string, 0, 4)
	if brand.Firm != "" {
		variables = append(variables, "--sati-doc-firm: "+cssString(brand.Firm))
	}
	if brand.Confidential != "" {
		variables = append(variables, "--sati-doc-confidential: "+cssString(brand.Confidential))
	}
	if brand.Disclaimer != "" {
		variables = append(variables, "--sati-doc-disclaimer: "+cssString(brand.Disclaimer))
	}
	if brand.Accent != "" {
		variables = append(variables, "--sati-doc-accent: "+cssString(brand.Accent))
	}
	if len(variables) == 0 {
		return ""
	}
	return ":root{" + strings.Join(variables, ";") + "}"
}

// cssString 把文本包成带双引号的 CSS 字符串字面量。
func cssString(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\A `)
	return `"` + replacer.Replace(text) + `"`
}

// findElementByID 深度优先查找 id 匹配的元素。
func findElementByID(node *html.Node, id string) *html.Node {
	return findElementMatching(node, func(candidate *html.Node) bool {
		return candidate.Type == html.ElementNode && attribute(candidate, "id") == id
	})
}

// findElement 查找指定标签名的第一个元素。
func findElement(node *html.Node, tag string) *html.Node {
	return findElementMatching(node, func(candidate *html.Node) bool {
		return candidate.Type == html.ElementNode && candidate.Data == tag
	})
}

// findElementMatching 深度优先查找满足条件的元素。
func findElementMatching(node *html.Node, match func(*html.Node) bool) *html.Node {
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElementMatching(child, match); found != nil {
			return found
		}
	}
	return nil
}

// attribute 返回元素属性值。
func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

// replaceChildren 清空元素子节点并填入 HTML 片段。
func replaceChildren(target *html.Node, fragment string) error {
	for child := target.FirstChild; child != nil; {
		next := child.NextSibling
		target.RemoveChild(child)
		child = next
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), target)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		target.AppendChild(node)
	}
	return nil
}

// numberingExemplar 解析编号形态，如 "[0001]" → 前缀 "["、后缀 "]"、宽度 4。
type numberingExemplar struct {
	prefix string
	suffix string
	width  int
}

// parseNumberingExemplar 从声明值解析编号形态。
func parseNumberingExemplar(value string) (numberingExemplar, bool) {
	start, end := -1, -1
	for i, r := range value {
		if r >= '0' && r <= '9' {
			start = i
			break
		}
	}
	if start < 0 {
		return numberingExemplar{}, false
	}
	for i := start; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			end = i
			break
		}
	}
	if end < 0 {
		end = len(value)
	}
	width := end - start
	if width == 0 {
		return numberingExemplar{}, false
	}
	return numberingExemplar{prefix: value[:start], suffix: value[end:], width: width}, true
}

// applyNumbering 对声明了 data-paragraph-numbering 的容器内的段落逐段编号。
//
// 编号写成字面文本而非 CSS 计数器：同一份 HTML 会被 PDF 与 HTML→DOCX 两条链路消费，
// 只有字面文本能让两边读到同一串编号。
func applyNumbering(document *html.Node) {
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if declared := attribute(node, "data-paragraph-numbering"); declared != "" {
				if exemplar, ok := parseNumberingExemplar(declared); ok {
					numberParagraphs(node, exemplar)
				}
				return // 嵌套声明各自编号，不与外层共用序号
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
}

// numberParagraphs 给容器内的 p/li 写编号。
//
// 与 dsh 的语义对齐：标题/表格/figure/预格式段不编号；段落不嵌套（编号后不再下探
// 子节点，避免 li 与其内 p 各占一个序号）；只有图片或纯空白的段落不占号；段落开头
// 已有的同形态编号先剥掉再重写，避免重复渲染叠加。
func numberParagraphs(container *html.Node, exemplar numberingExemplar) {
	sequence := 0
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6", "table", "figure", "pre":
				return // 标题、表格、图、预格式段不编号
			case "p", "li":
				stripExistingNumber(node, exemplar)
				if !hasVisibleText(node) {
					return // 只有图片或空白的段落不占号
				}
				sequence++
				label := exemplar.prefix + padNumber(sequence, exemplar.width) + exemplar.suffix + " "
				insertLeadingText(node, label)
				return // 段落不嵌套：编号后不再遍历其子节点
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
}

// hasVisibleText 判断段落是否有可见文本；只有图片或纯空白的段落不占号。
func hasVisibleText(node *html.Node) bool {
	text := strings.ReplaceAll(textContent(node), " ", " ")
	return strings.TrimSpace(text) != ""
}

// stripExistingNumber 剥掉段落开头已有的同形态编号（含两侧空白）。
func stripExistingNumber(node *html.Node, exemplar numberingExemplar) {
	text := firstTextNode(node)
	if text == nil {
		return
	}
	value := strings.TrimLeft(text.Data, " 　")
	if !strings.HasPrefix(value, exemplar.prefix) {
		return
	}
	rest := value[len(exemplar.prefix):]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 || !strings.HasPrefix(rest[digits:], exemplar.suffix) {
		return
	}
	text.Data = strings.TrimLeft(rest[digits+len(exemplar.suffix):], " 　")
}

// firstTextNode 返回元素内第一个文本节点。
func firstTextNode(node *html.Node) *html.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			return child
		}
		if found := firstTextNode(child); found != nil {
			return found
		}
	}
	return nil
}

// insertLeadingText 在元素开头插入一段文本。
func insertLeadingText(node *html.Node, text string) {
	textNode := &html.Node{Type: html.TextNode, Data: text}
	if node.FirstChild != nil {
		node.InsertBefore(textNode, node.FirstChild)
		return
	}
	node.AppendChild(textNode)
}

// padNumber 把序号补零到指定宽度。
func padNumber(value, width int) string {
	text := strconv.Itoa(value)
	for len(text) < width {
		text = "0" + text
	}
	return text
}

// TemplateIDs 返回排序后的模板 id，供报告与测试使用。
func TemplateIDs() ([]string, error) {
	ids, err := Templates()
	if err != nil {
		return nil, err
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return sorted, nil
}
