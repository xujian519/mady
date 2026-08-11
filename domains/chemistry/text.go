// Package chemistry 提供专利化学领域的文本级化学实体提取与校验。
//
// 本包移植自 Sati src/patent/chemistry/text.ts 的确定性提取逻辑
// （三级流水线第一级：正则候选），面向专利文本中的分子式与类 SMILES
// 词元做防幻觉校验：
//
//   - 分子式候选要求 ≥2 种真实元素（118 元素周期表校验），天然排除
//     C1-C6 范围写法、C12 章节号、式(I) 引用等单元素噪声，同时过滤
//     CD4/DNA/ATP 等生物医药缩略词；
//   - 类 SMILES 词元要求以真实元素字母开头且含结构要素（芳环小写/
//     环闭合数字/分支/双键/方括号），排除 M2=5、ratio=2 等非化学词元。
//
// 候选后续可经 LLM 复核与 RDKit 结构验证（本包不依赖外部库）。
package chemistry

import (
	"regexp"
	"strings"
)

// elementSymbols 为 118 种元素周期表符号：分子式/类 SMILES 词首真实性校验。
var elementSymbols = map[string]struct{}{
	"H": {}, "He": {}, "Li": {}, "Be": {}, "B": {}, "C": {}, "N": {}, "O": {}, "F": {}, "Ne": {},
	"Na": {}, "Mg": {}, "Al": {}, "Si": {}, "P": {}, "S": {}, "Cl": {}, "Ar": {}, "K": {}, "Ca": {},
	"Sc": {}, "Ti": {}, "V": {}, "Cr": {}, "Mn": {}, "Fe": {}, "Co": {}, "Ni": {}, "Cu": {}, "Zn": {},
	"Ga": {}, "Ge": {}, "As": {}, "Se": {}, "Br": {}, "Kr": {}, "Rb": {}, "Sr": {}, "Y": {}, "Zr": {},
	"Nb": {}, "Mo": {}, "Tc": {}, "Ru": {}, "Rh": {}, "Pd": {}, "Ag": {}, "Cd": {}, "In": {}, "Sn": {},
	"Sb": {}, "Te": {}, "I": {}, "Xe": {}, "Cs": {}, "Ba": {}, "La": {}, "Ce": {}, "Pr": {}, "Nd": {},
	"Pm": {}, "Sm": {}, "Eu": {}, "Gd": {}, "Tb": {}, "Dy": {}, "Ho": {}, "Er": {}, "Tm": {}, "Yb": {},
	"Lu": {}, "Hf": {}, "Ta": {}, "W": {}, "Re": {}, "Os": {}, "Ir": {}, "Pt": {}, "Au": {}, "Hg": {},
	"Tl": {}, "Pb": {}, "Bi": {}, "Po": {}, "At": {}, "Rn": {}, "Fr": {}, "Ra": {}, "Ac": {}, "Th": {},
	"Pa": {}, "U": {}, "Np": {}, "Pu": {}, "Am": {}, "Cm": {}, "Bk": {}, "Cf": {}, "Es": {}, "Fm": {},
	"Md": {}, "No": {}, "Lr": {}, "Rf": {}, "Db": {}, "Sg": {}, "Bh": {}, "Hs": {}, "Mt": {}, "Ds": {},
	"Rg": {}, "Cn": {}, "Nh": {}, "Fl": {}, "Mc": {}, "Lv": {}, "Ts": {}, "Og": {},
}

// regexpMustCompile 编译正则，失败时 panic（均为包内静态字面量，必编译通过）。
func regexpMustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

var (
	// hillGroupRe 匹配单个 Hill 元素组：大写字母（可带一个小写字母）+ 可选数字。
	hillGroupRe = regexpMustCompile(`[A-Z][a-z]?`)
	// hillFormRe 匹配整个 Hill 记法公式：元素组序列。
	hillFormRe = regexpMustCompile(`^[A-Z][a-z]?\d*(?:[A-Z][a-z]?\d*)*$`)
	// formulaCandidateRe 为分子式候选的宽松模式（元素组序列，含 C?H? 前缀特判）。
	// TS 原式含 (?<![A-Za-z0-9])/(?![A-Za-z0-9]) 前后断言，Go RE2 不支持，
	// 改由 extractFormulaCandidates 对每个匹配做手工边界校验。
	formulaCandidateRe = regexpMustCompile(`(?:C\d*(?:H\d+)?|H\d+|[A-Z][a-z]?\d*)(?:[A-Z][a-z]?\d*)+`)
	// smilesTokenRe 匹配类 SMILES 连续词元候选（4-200 字符）。
	smilesTokenRe = regexpMustCompile(`[A-Za-z0-9@+\-\\#=()\[\]/%.]{4,200}`)
	// smilesCharsRe 为 SMILES 合法字符集。
	smilesCharsRe = regexpMustCompile(`^[A-Za-z0-9@+\-\\#=()\[\]/%.]+$`)
	// smilesAlphaRe 要求词元至少含一个字母。
	smilesAlphaRe = regexpMustCompile(`[A-Za-z]`)
	// smilesHeadRe 匹配词元首部元素（或芳环小写原子），方括号原子可选。
	smilesHeadRe = regexpMustCompile(`^\[?(Cl|Br|[A-Z][a-z]?|[bcnops])`)
	// smilesStructureRe 匹配键/分支/方括号等结构要素。
	smilesStructureRe = regexpMustCompile(`[=#()\[\]\\@%]`)
	// smilesAromaticRe 匹配芳环小写原子。
	smilesAromaticRe = regexpMustCompile(`[bcnops]`)
)

// ChemicalExtraction 为文本化学候选提取结果（去重保序）。
type ChemicalExtraction struct {
	// Formulas 为分子式候选（Hill 序，≥2 种真实元素）。
	Formulas []string `json:"formulas"`
	// SmilesTokens 为类 SMILES 词元候选。
	SmilesTokens []string `json:"smiles_tokens"`
}

// IsValidHillFormula 校验 Hill 记法分子式（防幻觉硬门槛）：
// 元素符号+数字序列、全部为真实元素、且含 ≥2 种元素。
// 垃圾输入（如 ABC!@#）不得直通 usable。
func IsValidHillFormula(formula string) bool {
	f := strings.TrimSpace(formula)
	if len(f) == 0 || len(f) > 128 {
		return false
	}
	if !hillFormRe.MatchString(f) {
		return false
	}
	seen := make(map[string]struct{})
	for _, g := range hillGroupRe.FindAllString(f, -1) {
		if _, ok := elementSymbols[g]; !ok {
			return false
		}
		seen[g] = struct{}{}
	}
	// 至少两种真实元素（排除 C12 / N2 单元素写法）
	return len(seen) >= 2
}

// ExtractFormulaCandidates 提取文本中的分子式候选（Hill 序，≥2 种真实元素）。
func ExtractFormulaCandidates(text string) []string {
	found := make(map[string]struct{})
	out := make([]string, 0, 4)
	cursor := 0
	for cursor < len(text) {
		loc := formulaCandidateRe.FindStringIndex(text[cursor:])
		if loc == nil {
			break
		}
		start, end := cursor+loc[0], cursor+loc[1]
		// 手工边界校验：等价 TS 的 (?<![A-Za-z0-9]) 与 (?![A-Za-z0-9])
		leftOK := start == 0 || !isAlphaNumByte(text[start-1])
		rightOK := end >= len(text) || !isAlphaNumByte(text[end])
		if !leftOK || !rightOK {
			// 边界不干净时从当前位置 +1 继续，避免错过后续独立词元
			cursor = start + 1
			continue
		}
		token := text[start:end]
		// 至少两种元素且全部为真实元素（排除 C12 / CD4 / DNA / ATP 等）
		if countDistinctElements(token) >= 2 && IsValidHillFormula(token) {
			if _, ok := found[token]; !ok {
				found[token] = struct{}{}
				out = append(out, token)
			}
		}
		cursor = end
	}
	return out
}

// ExtractSmilesCandidates 提取文本中的类 SMILES 词元候选
// （以真实元素开头且含结构要素的连续词元）。
func ExtractSmilesCandidates(text string) []string {
	found := make(map[string]struct{})
	out := make([]string, 0, 4)
	for _, loc := range smilesTokenRe.FindAllStringIndex(text, -1) {
		token := text[loc[0]:loc[1]]
		if !looksLikeSmiles(token) {
			continue
		}
		if _, ok := found[token]; !ok {
			found[token] = struct{}{}
			out = append(out, token)
		}
	}
	return out
}

// ExtractChemicalCandidates 统一提取入口：分子式 + 类 SMILES 词元（去重保序）。
func ExtractChemicalCandidates(text string) ChemicalExtraction {
	return ChemicalExtraction{
		Formulas:     ExtractFormulaCandidates(text),
		SmilesTokens: ExtractSmilesCandidates(text),
	}
}

// countDistinctElements 统计词元中出现的不同元素数。
func countDistinctElements(token string) int {
	seen := make(map[string]struct{})
	for _, g := range hillGroupRe.FindAllString(token, -1) {
		seen[g] = struct{}{}
	}
	return len(seen)
}

// looksLikeSmiles 判断词元是否像类 SMILES：以真实元素字母开头（或芳环
// 小写原子），且含键/分支/方括号等结构要素或芳环+环闭合数字。
func looksLikeSmiles(token string) bool {
	if !isPlausibleSmilesSyntax(token) {
		return false
	}
	// 须以真实元素字母开头（或方括号原子）——排除 M2=5 / ratio=2 等非化学词元
	headMatch := smilesHeadRe.FindStringSubmatch(token)
	if headMatch == nil {
		return false
	}
	head := headMatch[1]
	isAromatic := head == strings.ToLower(head)
	if !isAromatic {
		if _, ok := elementSymbols[head]; !ok {
			return false
		}
	}
	// 键/分支/方括号等结构要素，或芳环小写 + 环闭合数字（c1ccccc1）
	hasBondOrBranch := smilesStructureRe.MatchString(token)
	hasAromaticRingClosure := smilesAromaticRe.MatchString(token) && countDigits(token) >= 2
	return hasBondOrBranch || hasAromaticRingClosure
}

// isPlausibleSmilesSyntax 为语法级 SMILES 预检：字符集与基本形态检查，
// 不能证明结构合法。
func isPlausibleSmilesSyntax(value string) bool {
	s := strings.TrimSpace(value)
	if len(s) == 0 || len(s) > 512 {
		return false
	}
	if !smilesCharsRe.MatchString(s) {
		return false
	}
	return smilesAlphaRe.MatchString(s)
}

// countDigits 统计字符串中 ASCII 数字个数。
func countDigits(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			n++
		}
	}
	return n
}

// isAlphaNumByte 判断字节是否为 ASCII 字母或数字。
func isAlphaNumByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
