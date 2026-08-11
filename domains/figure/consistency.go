// Package figure 提供专利附图跨图一致性的确定性核验。
//
// 本包移植自 Sati src/patent/figure/multi-figure-consistency.ts（P2-1），
// 面向同一案件的多张附图（图1、图2…）做跨图对齐核验：
//
//   - 引用标记冲突检测（symbol/category/name/value 四维度）；
//   - 图号连续性检查；
//   - 图文对齐（权利要求/说明书引用的标记是否全部出现在附图中）；
//   - 单引脚孤立网络（跨图聚合后仍仅单元件连接的非电源网络）。
//
// 纯函数、无模型依赖：输入为各附图电学分析结果，输出一致性报告。
// 数据模型对齐 Sati figure/types.ts（ElectricalComponent/ElectricalNet/
// FigureAnalysisResult），可由 vision/ocr 流水线产出输入数据后调用本包。
package figure

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ElectricalComponent 为电学符号元件（符号级识别结果，对应附图标记）。
type ElectricalComponent struct {
	// Ref 为附图标记（如 "R1"，与图面字母前缀+编号一致）。
	Ref string
	// Symbol 为符号库 id（resistor/capacitor/…；无法匹配时为 "unknown"）。
	Symbol string
	// Category 为元件大类（passive/semiconductor/ic/…）。
	Category string
	// Name 为元件中文名称（如 "电阻"）。
	Name string
	// Value 为电气参数（图上明确标注时，如 "10kΩ"；否则为空串）。
	Value string
}

// ElectricalNet 为电路网络（电气连通关系）。
type ElectricalNet struct {
	// Name 为网络名（VCC / GND / 节点号如 N1）。
	Name string
	// ConnectedRefs 为连接到该网络的元件引脚，格式 "元件标号.引脚号"（如 "R1.1"）。
	ConnectedRefs []string
}

// ElectricalAnalysis 为单张附图的电学深度分析结果。
type ElectricalAnalysis struct {
	// Components 为识别出的元件列表。
	Components []ElectricalComponent
	// Nets 为识别出的网络列表。
	Nets []ElectricalNet
}

// FigureAnalysisResult 为单张附图分析结果（一致性核验所需子集，
// 完整字段见 Sati figure/types.ts 的 FigureAnalysisResult）。
type FigureAnalysisResult struct {
	// ImagePath 为分析图片路径（工作区相对路径）。
	ImagePath string
	// FigureNumber 为附图编号。
	FigureNumber int
	// FigureType 为附图类型（structure/flowchart/circuit/…）。
	FigureType string
	// Electrical 为电学深度分析结果（仅附图类型为 circuit/schematic 时非 nil）。
	Electrical *ElectricalAnalysis
}

// ComponentConflict 为跨图组件冲突。
type ComponentConflict struct {
	// Kind 为冲突维度："symbol" | "category" | "name" | "value"。
	Kind string
	// Ref 为发生冲突的附图标记。
	Ref string
	// FigureNumbers 为出现该 ref 的图号列表。
	FigureNumbers []int
	// Values 为各图号下的不同取值。
	Values []ConflictValue
	// Message 为人类可读的冲突描述。
	Message string
}

// ConflictValue 为某图号下冲突维度的一个取值。
type ConflictValue struct {
	// FigureNumber 为图号。
	FigureNumber int
	// Value 为该图号下的取值。
	Value string
}

// GlobalComponent 为同一 ref 跨图合并后的全局组件。
type GlobalComponent struct {
	ElectricalComponent
	// FigureNumbers 为出现该组件的图号列表。
	FigureNumbers []int
}

// GlobalNet 为按网络名跨图合并的全局网络。
type GlobalNet struct {
	// Name 为网络名。
	Name string
	// ConnectedRefs 为跨图聚合后的连接元件引脚。
	ConnectedRefs []string
	// FigureNumbers 为出现该网络的图号列表。
	FigureNumbers []int
}

// FigureConsistencyReport 为多图一致性核验结果。
type FigureConsistencyReport struct {
	// FigureCount 为参与分析的附图数量。
	FigureCount int
	// GlobalComponents 为全局组件索引：同一 ref 跨图合并后的最佳描述。
	GlobalComponents map[string]GlobalComponent
	// GlobalNets 为全局网络索引：按 net name 合并跨图连接。
	GlobalNets map[string]GlobalNet
	// Conflicts 为跨图组件冲突。
	Conflicts []ComponentConflict
	// MissingFigureNumbers 为图号不连续时缺失的图号。
	MissingFigureNumbers []int
	// MissingRefs 为权利要求/说明书中引用但在附图中未出现的 ref。
	MissingRefs []string
	// OrphanNets 为单引脚非电源网络（跨图聚合后仍单引脚）。
	OrphanNets []string
	// Warnings 为警告列表（人类可读）。
	Warnings []string
	// Summary 为文本摘要。
	Summary string
	// Consistent 为是否无冲突且无缺漏。
	Consistent bool
}

// refPrefixes 为符号库已知标号前缀（对齐 Sati symbols 库注释：
// R/C/L/D/Q/U/IC/XT/K/F/BAT/GND…），用于提取附图标记时排除普通数字。
var refPrefixes = map[string]bool{
	"R": true, "C": true, "L": true, "D": true, "Q": true,
	"U": true, "IC": true, "XT": true, "K": true, "F": true,
	"BAT": true, "GND": true,
}

var (
	// claimRefTokenRe 匹配中文（0-6 个）+ 字母数字段（如 "电阻R1"）。
	// Go RE2 不支持 \u4e00-\u9fa5 转义，使用字面字符范围 [一-龥]。
	claimRefTokenRe = regexp.MustCompile(`[一-龥]{0,6}?[A-Za-z]{1,4}\d{1,4}`)
	// refSuffixRe 提取 token 末尾的标号段（如 "包括电阻R1" → "R1"）。
	refSuffixRe = regexp.MustCompile(`[A-Za-z]+\d+$`)
	// refSplitRe 拆分标号为字母前缀 + 数字（"IC1" → "IC" + "1"）。
	refSplitRe = regexp.MustCompile(`^([A-Za-z]+)(\d+)$`)
)

// claimRefLimit 为单次提取附图标记的数量上限。
const claimRefLimit = 40

// CheckFigureConsistency 检查多个附图分析结果的一致性。
//
// figures 为附图分析结果（内部按 figureNumber 排序，不修改调用方数据）；
// claimContext 为权利要求或说明书文本（可为空串），用于提取期望出现的附图标记。
func CheckFigureConsistency(figures []FigureAnalysisResult, claimContext string) FigureConsistencyReport {
	figs := append([]FigureAnalysisResult(nil), figures...)
	sort.Slice(figs, func(i, j int) bool { return figs[i].FigureNumber < figs[j].FigureNumber })

	figureNumbers := make([]int, 0, len(figs))
	for _, f := range figs {
		figureNumbers = append(figureNumbers, f.FigureNumber)
	}

	globalComponents, conflicts := aggregateComponents(figs)
	globalNets := aggregateNets(figs)

	var warnings []string

	// 图号连续性（从 1 到最大图号检查缺失）
	missingFigureNumbers := findMissingFigureNumbers(figureNumbers)
	if len(missingFigureNumbers) > 0 {
		warnings = append(warnings, fmt.Sprintf("附图编号不连续，缺失图号：%s", joinInts(missingFigureNumbers)))
	}

	// 跨图引用的 ref 冲突（symbol/category 等不一致）提示
	for _, c := range conflicts {
		warnings = append(warnings, c.Message)
	}

	// 权利要求/说明书中的标记是否都在图中出现
	var missingRefs []string
	if claimContext != "" {
		missingRefs = findMissingRefs(claimContext, globalComponents)
		if len(missingRefs) > 0 {
			warnings = append(warnings, fmt.Sprintf("权利要求/说明书中引用但未在附图中识别：%s", strings.Join(missingRefs, ", ")))
		}
	}

	// 单引脚非电源网络（跨图聚合后仍仅单元件连接）
	orphanNets := findOrphanNets(globalNets)
	if len(orphanNets) > 0 {
		warnings = append(warnings, fmt.Sprintf("跨图聚合后仍仅单元件连接的网络：%s", strings.Join(orphanNets, ", ")))
	}

	consistent := len(conflicts) == 0 && len(missingFigureNumbers) == 0 && len(missingRefs) == 0

	return FigureConsistencyReport{
		FigureCount:          len(figs),
		GlobalComponents:     globalComponents,
		GlobalNets:           globalNets,
		Conflicts:            conflicts,
		MissingFigureNumbers: missingFigureNumbers,
		MissingRefs:          missingRefs,
		OrphanNets:           orphanNets,
		Warnings:             warnings,
		Summary: buildSummary(consistencyStats{
			figureCount:     len(figs),
			componentCount:  len(globalComponents),
			netCount:        len(globalNets),
			conflictCount:   len(conflicts),
			missingFigCount: len(missingFigureNumbers),
			missingRefCount: len(missingRefs),
			orphanNetCount:  len(orphanNets),
		}),
		Consistent: consistent,
	}
}

// aggregateComponents 聚合全部附图的组件索引并检测跨图四维度冲突。
func aggregateComponents(figs []FigureAnalysisResult) (map[string]GlobalComponent, []ComponentConflict) {
	globalComponents := make(map[string]GlobalComponent)
	var conflicts []ComponentConflict
	for i := range figs {
		if figs[i].Electrical == nil {
			continue
		}
		for _, c := range figs[i].Electrical.Components {
			existing, ok := globalComponents[c.Ref]
			if !ok {
				globalComponents[c.Ref] = GlobalComponent{
					ElectricalComponent: c,
					FigureNumbers:       []int{figs[i].FigureNumber},
				}
				continue
			}
			merged, newConflicts := mergeComponent(existing, c, figs[i].FigureNumber)
			globalComponents[c.Ref] = merged
			conflicts = append(conflicts, newConflicts...)
		}
	}
	return globalComponents, conflicts
}

// mergeComponent 将单个组件并入全局组件：合并图号、检测四维度差异并生成冲突。
func mergeComponent(existing GlobalComponent, c ElectricalComponent, figNumber int) (GlobalComponent, []ComponentConflict) {
	if !containsInt(existing.FigureNumbers, figNumber) {
		existing.FigureNumbers = append(existing.FigureNumbers, figNumber)
		sort.Ints(existing.FigureNumbers)
	}

	var diffs []componentDiff
	if existing.Symbol != c.Symbol {
		diffs = append(diffs, componentDiff{kind: "symbol", a: existing.Symbol, b: c.Symbol})
	}
	if existing.Category != c.Category {
		diffs = append(diffs, componentDiff{kind: "category", a: existing.Category, b: c.Category})
	}
	if existing.Name != c.Name {
		diffs = append(diffs, componentDiff{kind: "name", a: existing.Name, b: c.Name})
	}
	if existing.Value != "" && c.Value != "" && existing.Value != c.Value {
		diffs = append(diffs, componentDiff{kind: "value", a: existing.Value, b: c.Value})
	}

	var conflicts []ComponentConflict
	for _, d := range diffs {
		conflicts = append(conflicts, ComponentConflict{
			Kind:          d.kind,
			Ref:           c.Ref,
			FigureNumbers: append([]int(nil), existing.FigureNumbers...),
			Values: []ConflictValue{
				{FigureNumber: existing.FigureNumbers[0], Value: d.a},
				{FigureNumber: figNumber, Value: d.b},
			},
			Message: fmt.Sprintf("标记 %s 在图 %s 中 %s 不一致：%s / %s",
				c.Ref, joinInts(existing.FigureNumbers), d.kind, d.a, d.b),
		})
	}

	// 保留更具体的值（有 value 的优先）
	if existing.Value == "" && c.Value != "" {
		existing.Value = c.Value
	}
	return existing, conflicts
}

// aggregateNets 聚合全部附图的网络索引（按网络名跨图合并连接）。
func aggregateNets(figs []FigureAnalysisResult) map[string]GlobalNet {
	globalNets := make(map[string]GlobalNet)
	for i := range figs {
		if figs[i].Electrical == nil {
			continue
		}
		for _, n := range figs[i].Electrical.Nets {
			existing, ok := globalNets[n.Name]
			if !ok {
				globalNets[n.Name] = GlobalNet{
					Name:          n.Name,
					ConnectedRefs: append([]string(nil), n.ConnectedRefs...),
					FigureNumbers: []int{figs[i].FigureNumber},
				}
				continue
			}
			if !containsInt(existing.FigureNumbers, figs[i].FigureNumber) {
				existing.FigureNumbers = append(existing.FigureNumbers, figs[i].FigureNumber)
				sort.Ints(existing.FigureNumbers)
			}
			for _, r := range n.ConnectedRefs {
				if !containsString(existing.ConnectedRefs, r) {
					existing.ConnectedRefs = append(existing.ConnectedRefs, r)
				}
			}
			globalNets[n.Name] = existing
		}
	}
	return globalNets
}

// findMissingFigureNumbers 返回 1..max 中未出现的图号（升序）。
func findMissingFigureNumbers(figureNumbers []int) []int {
	var missing []int
	if len(figureNumbers) == 0 {
		return missing
	}
	present := make(map[int]bool, len(figureNumbers))
	for _, n := range figureNumbers {
		present[n] = true
	}
	maxNum := figureNumbers[len(figureNumbers)-1]
	for n := 1; n <= maxNum; n++ {
		if !present[n] {
			missing = append(missing, n)
		}
	}
	return missing
}

// findMissingRefs 返回权利要求/说明书中引用但未在附图中识别的标记。
func findMissingRefs(claimContext string, globalComponents map[string]GlobalComponent) []string {
	allFigureRefs := make(map[string]bool, len(globalComponents))
	for ref := range globalComponents {
		allFigureRefs[ref] = true
	}
	var missing []string
	for _, r := range ExtractClaimRefs(claimContext) {
		if !allFigureRefs[r] {
			missing = append(missing, r)
		}
	}
	return missing
}

// findOrphanNets 返回跨图聚合后仍仅单元件连接的非电源网络。
func findOrphanNets(globalNets map[string]GlobalNet) []string {
	var orphan []string
	for name, net := range globalNets {
		if isPowerNet(name) {
			continue
		}
		refs := make(map[string]bool)
		for _, r := range net.ConnectedRefs {
			base := strings.SplitN(r, ".", 2)[0]
			if base != "" {
				refs[base] = true
			}
		}
		if len(refs) <= 1 {
			orphan = append(orphan, name)
		}
	}
	sort.Strings(orphan)
	return orphan
}

// ExtractClaimRefs 从权利要求/说明书文本提取附图标记（仅符号库已知前缀，
// 避免误报普通数字）。最多返回 claimRefLimit 个，按文本出现顺序。
func ExtractClaimRefs(text string) []string {
	if text == "" {
		return nil
	}
	found := make(map[string]bool)
	out := make([]string, 0, 8)
	for _, token := range claimRefTokenRe.FindAllString(text, -1) {
		inner := refSuffixRe.FindString(token)
		if inner == "" {
			continue
		}
		m := refSplitRe.FindStringSubmatch(inner)
		if m == nil {
			continue
		}
		prefix, number := m[1], m[2]
		// 仅收录符号库已知前缀的标记（R1/C2/IC1…），排除普通文本数字
		if !refPrefixes[prefix] {
			continue
		}
		ref := prefix + number
		if !found[ref] {
			found[ref] = true
			out = append(out, ref)
			if len(out) >= claimRefLimit {
				break
			}
		}
	}
	return out
}

// componentDiff 描述跨图组件某维度的取值差异。
type componentDiff struct {
	kind string
	a    string
	b    string
}

// consistencyStats 为摘要统计项。
type consistencyStats struct {
	figureCount     int
	componentCount  int
	netCount        int
	conflictCount   int
	missingFigCount int
	missingRefCount int
	orphanNetCount  int
}

// buildSummary 生成文本摘要（对齐 Sati buildSummary）。
func buildSummary(s consistencyStats) string {
	parts := []string{fmt.Sprintf("附图 %d 张，合并识别 %d 个元件、%d 个网络。", s.figureCount, s.componentCount, s.netCount)}
	if s.conflictCount > 0 {
		parts = append(parts, fmt.Sprintf("跨图标记冲突 %d 处。", s.conflictCount))
	}
	if s.missingFigCount > 0 {
		parts = append(parts, fmt.Sprintf("缺失图号 %d 个。", s.missingFigCount))
	}
	if s.missingRefCount > 0 {
		parts = append(parts, fmt.Sprintf("权利要求/说明书中有 %d 个引用未在附图中识别。", s.missingRefCount))
	}
	if s.orphanNetCount > 0 {
		parts = append(parts, fmt.Sprintf("孤立网络 %d 个。", s.orphanNetCount))
	}
	if s.conflictCount+s.missingFigCount+s.missingRefCount+s.orphanNetCount == 0 {
		parts = append(parts, "跨图一致性检查通过。")
	}
	return strings.Join(parts, "")
}

// isPowerNet 判断网络名是否为电源/地类网络（GND/VCC/VDD/VEE 及其前缀）。
func isPowerNet(name string) bool {
	upper := strings.ToUpper(name)
	return upper == "GND" || upper == "VCC" || upper == "VDD" || upper == "VEE" ||
		strings.HasPrefix(upper, "VCC") || strings.HasPrefix(upper, "VDD")
}

// containsInt 判断整数切片是否包含指定值。
func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// containsString 判断字符串切片是否包含指定值。
func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// joinInts 将整数切片转为逗号分隔字符串。
func joinInts(s []int) string {
	if len(s) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s))
	for _, n := range s {
		parts = append(parts, fmt.Sprintf("%d", n))
	}
	return strings.Join(parts, ",")
}
