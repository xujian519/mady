package specdrafting

import (
	"fmt"
	"regexp"
	"strings"
)

// 量化与化学表征检查：均报告「缺少应有细节」——只写效果、不写数字的效果句，
// 以及化学领域通篇没有任何产物表征手段的说明书。
// SMILES 校验依赖 RDKit；Mady 不随包该原生依赖，故与上游一致地跳过（记录为未覆盖）。

// vagueEffectRE 是空泛效果表述（"效果显著/大幅提升"等）。
var vagueEffectRE = regexp.MustCompile(`(?:效果|性能)(?:显著|良好|优异|优越|极佳|大幅|大大提高|明显提升|显著提高|大幅提升|明显改善|显著改善|明显|好)|(?:大大|显著|明显|大幅|有效)(?:提高|提升|改善|降低|减少|增强)`)

// effectDigitalRE 是效果句中的量化痕迹（数字、百分号）。
var effectDigitalRE = regexp.MustCompile(`[0-9]|％|%`)

// chemCharacterizationTerms 是化学领域至少要出现一项的产物表征手段。
var chemCharacterizationTerms = []string{
	"NMR", "核磁", "MS", "质谱", "IR", "红外", "元素分析", "XRPD", "XRD", "X射线", "X-射线",
	"晶胞参数", "空间群", "熔点", "旋光度", "UV", "紫外", "HPLC", "高效液相", "GC", "气相色谱",
}

const maxEffectSnippetRunes = 40

// CheckEffectQuantification 返回断言了效果却没有数字/百分比的句子（截断到 40 字）。
func CheckEffectQuantification(text string) []string {
	var hits []string
	for _, raw := range regexp.MustCompile(`[。；\n]`).Split(text, -1) {
		sentence := strings.TrimSpace(raw)
		if sentence == "" {
			continue
		}
		if vagueEffectRE.MatchString(sentence) && !effectDigitalRE.MatchString(sentence) {
			runes := []rune(sentence)
			if len(runes) > maxEffectSnippetRunes {
				runes = runes[:maxEffectSnippetRunes]
			}
			hits = append(hits, string(runes))
		}
	}
	return hits
}

// CheckChemicalCharacterization 返回正文中完全缺失的表征手段（化学领域检查用）。
func CheckChemicalCharacterization(text string) []string {
	var missing []string
	for _, term := range chemCharacterizationTerms {
		if !strings.Contains(text, term) {
			missing = append(missing, term)
		}
	}
	return missing
}

// CheckQuantification 把量化与化学表征检查组装为违规：
// 空泛效果为提示级，化学领域缺全部表征手段为错误级。
func CheckQuantification(text string, domain TechDomain) []Violation {
	var violations []Violation
	if hits := CheckEffectQuantification(text); len(hits) > 0 {
		violations = append(violations, Violation{
			RuleName: "effect_quantification",
			// 提示级：Mady 的判级策略把任一 warning 直接判 blocked，
			// 而「效果句缺数字」只是写作建议，不应卡住交付。
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("发现 %d 处效果表述缺少量化数据：%s", len(hits), strings.Join(hits, "；")),
			Suggestion: "把「效果显著/大幅提升」改写为可测结果（如「热导率由 1.8 W/(m·K) 提升至 2.6 W/(m·K)」）",
		})
	}
	if domain == DomainChemical && strings.TrimSpace(text) != "" {
		missing := CheckChemicalCharacterization(text)
		if len(missing) == len(chemCharacterizationTerms) {
			violations = append(violations, Violation{
				RuleName:   "chemical_characterization",
				Severity:   SeverityError,
				Message:    "化学领域说明书未记载任何产物表征手段（NMR/质谱/红外/元素分析/XRPD/熔点/旋光度/色谱等一项都没有）",
				Suggestion: "补充至少一项结构或组成表征方法及其实测结果，使本领域技术人员能够确认所制得的产品",
			})
		}
	}
	return violations
}
