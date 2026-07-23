package specdrafting

import (
	"strings"

	"github.com/xujian519/mady/domains/doctmpl"
)

// SpecBuilder 模板组装引擎，当 LLM 不可用时作为降级路径。
type SpecBuilder struct {
	templates *doctmpl.TemplateStore
}

// NewSpecBuilder 创建说明书构建器。
func NewSpecBuilder(templates *doctmpl.TemplateStore) *SpecBuilder {
	return &SpecBuilder{templates: templates}
}

// Build 基于输入生成说明书草案。
func (b *SpecBuilder) Build(input SpecInput) *SpecOutput {
	domain := input.TechDomain
	if domain == "" {
		domain = DomainGeneral
	}
	sections := b.buildSections(input, domain)
	abstract := b.buildAbstract(input, sections)
	totalWords := 0
	for _, s := range sections {
		totalWords += s.WordCnt
	}
	return &SpecOutput{
		Title:    resolveTitle(input.Title, domain),
		Abstract: abstract,
		Sections: sections,
		Metadata: SpecMetadata{
			PatentType:   input.PatentType,
			TechDomain:   domain,
			FeatureCount: len(input.Features),
			HasDrawings:  input.HasDrawings,
			WordCount:    totalWords,
		},
		Timestamp: timestamp(),
	}
}

func (b *SpecBuilder) buildSections(input SpecInput, domain TechDomain) []SpecSection {
	// 尝试从模板加载
	if b.templates != nil {
		if tmpl, ok := b.templates.FindByName(domainToTemplateName(domain)); ok {
			if secs := b.buildFromTemplate(input, tmpl); len(secs) > 0 {
				return secs
			}
		}
	}
	// 降级：默认内容
	secs := []SpecSection{
		{Name: SecTechField, Title: "技术领域", Content: b.defaultTechField(input, domain)},
		{Name: SecBackground, Title: "背景技术", Content: b.defaultBackground(input)},
		{Name: SecContent, Title: "发明内容", Content: b.defaultContent(input, domain)},
		{Name: SecDrawings, Title: "附图说明", Content: b.defaultDrawings(input)},
		{Name: SecEmbodiment, Title: "具体实施方式", Content: b.defaultEmbodiment(input, domain)},
	}
	for i := range secs {
		secs[i].WordCnt = ChineseCharCount(secs[i].Content)
	}
	return secs
}

func (b *SpecBuilder) buildFromTemplate(input SpecInput, tmpl doctmpl.DocTemplate) []SpecSection {
	vars := b.buildVars(input)
	body := doctmpl.ResolveDoc(tmpl, vars)
	if len(body) == 0 {
		return nil
	}
	return parseSections(body, input)
}

func (b *SpecBuilder) buildVars(input SpecInput) map[string]string {
	title := resolveTitle(input.Title, input.TechDomain)
	vars := map[string]string{
		"invention_name":    title,
		"tech_field":        input.Title,
		"core_problem":      firstOr(input.Problems, "提供一种改进的技术方案"),
		"tech_solution":     buildSolution(input),
		"background_desc":   buildBG(input),
		"embodiment_detail": buildEmb(input),
	}
	vars["effect_1"] = safeIndex(input.Effects, 0, "提供了一种改进的技术实现方案")
	vars["effect_2"] = safeIndex(input.Effects, 1, "提升了性能和可靠性")
	vars["problem_1"] = safeIndex(input.Problems, 0, "现有技术方案效率有待提高")
	vars["problem_2"] = safeIndex(input.Problems, 1, "现有技术方案的成本较高")
	vars["fig_2_desc"] = "本发明实施例的模块组成示意图"
	return vars
}

// =============================================================================
// 默认内容
// =============================================================================

func (b *SpecBuilder) defaultTechField(input SpecInput, domain TechDomain) string {
	prefix := "本发明"
	if input.PatentType == PatentTypeUtilityModel {
		prefix = "本实用新型"
	}
	field := domainToTechField(domain)
	return prefix + "涉及" + field + "技术领域，具体涉及一种" + resolveTitle(input.Title, domain) + "。"
}

func (b *SpecBuilder) defaultBackground(input SpecInput) string {
	problem := firstOr(input.Problems, "现有技术方案存在改进空间")
	return "目前，现有技术中存在以下问题：" + problem + "。因此，需要提供一种新的技术方案来解决上述问题。"
}

func (b *SpecBuilder) defaultContent(input SpecInput, domain TechDomain) string {
	prefix := "本发明"
	if input.PatentType == PatentTypeUtilityModel {
		prefix = "本实用新型"
	}
	problem := firstOr(input.Problems, "提供一种改进的技术方案")
	solution := buildSolution(input)
	effect := firstOr(input.Effects, "提供了一种改进的技术实现方案")
	chemNote := ""
	if domain == DomainChemical {
		chemNote = "\n\n以下技术效果需通过具体实验数据加以证实。"
	}
	return prefix + "要解决的技术问题是" + problem + "。\n\n" +
		"为解决上述技术问题，" + prefix + "采用如下技术方案：\n" +
		solution + "\n\n" +
		"与现有技术相比，" + prefix + "具有以下有益效果：\n" +
		"1. " + effect + "。" + chemNote
}

func (b *SpecBuilder) defaultDrawings(input SpecInput) string {
	switch {
	case input.PatentType == PatentTypeUtilityModel:
		return "图1为本实用新型实施例的结构示意图。\n图2为本实用新型实施例的装配示意图。"
	case input.HasDrawings:
		return "图1为本发明实施例的方法流程图。\n图2为本发明实施例的系统结构示意图。"
	default:
		return "（无附图）"
	}
}

func (b *SpecBuilder) defaultEmbodiment(input SpecInput, domain TechDomain) string {
	emb := "下面结合附图对本发明的具体实施方式进行详细说明。\n\n实施例1\n\n"
	emb += "本实施例提供" + resolveTitle(input.Title, domain) + "的一种具体实现方式。\n"
	if len(input.Features) > 0 {
		emb += "具体包括：\n"
		for i, f := range input.Features {
			if i >= 5 {
				break
			}
			emb += "- " + f.Description
			if f.Function != "" {
				emb += "，用于" + f.Function
			}
			emb += "\n"
		}
	} else {
		emb += "具体包括技术特征：\n- 技术特征的详细结构/步骤描述\n"
	}
	emb += "\n以上所述仅为本发明的优选实施例。"
	return emb
}

func (b *SpecBuilder) buildAbstract(input SpecInput, _ []SpecSection) string {
	abstract := "本发明涉及" + input.Title + "技术领域"
	if p := firstOr(input.Problems, ""); p != "" {
		abstract += "，解决了" + p
	}
	if e := firstOr(input.Effects, ""); e != "" {
		abstract += "，实现了" + e
	}
	abstract += "。"
	if ChineseCharCount(abstract) > 300 {
		runes := []rune(abstract)
		// 按 rune 截断确保 UTF-8 安全，再从尾部向前找到第 300 个中文字符的位置
		end := len(runes)
		cnt := 0
		for i, r := range runes {
			if r >= 0x4E00 && r <= 0x9FFF {
				cnt++
				if cnt == 300 {
					end = i + 1
					break
				}
			}
		}
		abstract = string(runes[:end]) + "。"
	}
	return abstract
}

// =============================================================================
// 辅助
// =============================================================================

func domainToTechField(domain TechDomain) string {
	switch domain {
	case DomainMechanical:
		return "机械结构"
	case DomainElectrical:
		return "电路与电子"
	case DomainChemical:
		return "化学与材料"
	case DomainSoftware:
		return "数据处理与计算机"
	default:
		return "工程"
	}
}

func domainToTemplateName(domain TechDomain) string {
	switch domain {
	case DomainMechanical:
		return "mechanical-spec"
	case DomainElectrical:
		return "electrical-spec"
	case DomainChemical:
		return "chemical-spec"
	case DomainSoftware:
		return "software-spec"
	default:
		return "mechanical-spec"
	}
}

func resolveTitle(title string, _ TechDomain) string {
	if title == "" {
		return "一种技术方案"
	}
	return title
}

func buildSolution(input SpecInput) string {
	if len(input.Features) == 0 {
		return "包括以下技术特征：特征1、特征2、特征3。"
	}
	r := "包括：\n"
	for i, f := range input.Features {
		if i >= 5 {
			r += "- 以及其他必要技术特征\n"
			break
		}
		r += "- " + f.Description
		if f.Function != "" {
			r += "，用于" + f.Function
		}
		r += "\n"
	}
	return r
}

func buildBG(input SpecInput) string {
	if input.PriorArt != "" {
		return input.PriorArt
	}
	if len(input.Problems) > 0 {
		return "现有技术中存在" + input.Problems[0] + "的问题。"
	}
	return "现有技术方案在性能和效率方面存在改进空间。"
}

func buildEmb(input SpecInput) string {
	ref := ""
	if input.HasDrawings {
		ref = "如图1所示，"
	}
	detail := ref + "本实施例提供" + resolveTitle(input.Title, input.TechDomain) + "。\n"
	for i, f := range input.Features {
		if i >= 3 {
			break
		}
		detail += "- " + f.Description + "\n"
	}
	return detail
}

// sectionTitleMap 将章节标题中文名映射到 SpecSectionName。
var sectionTitleMap = map[string]SpecSectionName{
	"技术领域":   SecTechField,
	"背景技术":   SecBackground,
	"发明内容":   SecContent,
	"实用新型内容": SecContent,
	"附图说明":   SecDrawings,
	"具体实施方式": SecEmbodiment,
}

// parseSections 从 Markdown 文本中按 ## 标题切分说明书章节。
// 返回非空切片表示成功从模板渲染结果中提取了章节内容。
func parseSections(md string, _ SpecInput) []SpecSection {
	if md == "" {
		return nil
	}
	runes := []rune(md)
	var sections []SpecSection
	i := 0
	for i < len(runes) {
		// 查找 ## 标题
		if i+1 < len(runes) && runes[i] == '#' && runes[i+1] == '#' {
			// 跳过 ##
			j := i + 2
			// 跳过空白
			for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t') {
				j++
			}
			// 读取标题行
			titleStart := j
			for j < len(runes) && runes[j] != '\n' {
				j++
			}
			titleText := string(runes[titleStart:j])
			j++ // 跳过换行

			// 读取内容到下一个 ## 或结尾
			contentStart := j
			for j < len(runes) {
				if j+1 < len(runes) && runes[j] == '\n' && runes[j+1] == '#' {
					break
				}
				if runes[j] == '#' && (j == 0 || runes[j-1] == '\n') {
					break
				}
				j++
			}
			content := strings.TrimSpace(string(runes[contentStart:j]))

			// 映射章节名
			if name, ok := sectionTitleMap[titleText]; ok && content != "" {
				sections = append(sections, SpecSection{
					Name:    name,
					Title:   titleText,
					Content: content,
					WordCnt: ChineseCharCount(content),
				})
			}
			i = j
			continue
		}
		i++
	}
	if len(sections) == 0 {
		return nil
	}
	return sections
}

func firstOr(strs []string, def string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return def
}

func safeIndex(strs []string, i int, def string) string {
	if i >= 0 && i < len(strs) {
		return strs[i]
	}
	return def
}
