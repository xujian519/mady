package claimdrafting

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// =============================================================================
// LLMDrafter LLM 撰写器
// =============================================================================

// LLMDrafter 通过 LLM 增强权利要求撰写质量。
// 当 LLM provider 可用时，提供优化表述、从零撰写、措辞优化等能力。
// 当 provider 不可用时，降级为纯规则引擎模式。
type LLMDrafter struct {
	provider Provider // LLM provider 接口
	builder  *ClaimBuilder
	engine   *RuleEngine // 规则引擎（用于 LLM 输出的二次验证）
}

// Provider 是 LLM provider 接口，用于抽象实际的 LLM 调用。
type Provider interface {
	// Complete 发送提示词并返回完成结果。
	Complete(prompt string) (string, error)
	// Available 返回 provider 是否可用。
	Available() bool
}

// NewLLMDrafter 创建一个 LLM 撰写器。
// provider 可为 nil（此时降级为纯规则引擎）。
// engine 可为 nil（此时跳过 LLM 输出的规则验证）。
func NewLLMDrafter(provider Provider, builder *ClaimBuilder, engine *RuleEngine) *LLMDrafter {
	return &LLMDrafter{
		provider: provider,
		builder:  builder,
		engine:   engine,
	}
}

// DraftFromScratch 使用 LLM 从头撰写权利要求书。
// 流程：先通过规则引擎 builder 生成降级输出，再尝试 LLM 增强。
// LLM 返回的文本被解析为结构化 Claim 对象；解析失败时静默降级到 builder 输出。
func (d *LLMDrafter) DraftFromScratch(input DraftInput) (*DraftOutput, error) {
	if d == nil {
		panic("claimdrafting: LLMDrafter.DraftFromScratch called on nil receiver")
	}
	if d.builder == nil {
		return NewClaimBuilder("", "").Build(input)
	}

	// 步骤 1：builder 降级输出（确保总有可用结果）
	fallback, err := d.builder.Build(input)
	if err != nil {
		return nil, err
	}

	// 步骤 2：尝试 LLM 增强
	if d.provider == nil || !d.provider.Available() {
		return fallback, nil
	}

	prompt := d.buildPrompt(input)
	result, err := d.provider.Complete(prompt)
	if err != nil {
		return fallback, nil
	}

	// 步骤 3：解析 LLM 结果
	if parsed := parseClaimsFromLLM(result, input); parsed != nil {
		// 通过规则引擎验证 LLM 生成的 claims（而非复用 builder 的警告）
		if d.engine != nil {
			allClaims := parsed.Claims.Claims()
			violations := d.engine.Validate(allClaims, input)
			for _, v := range violations {
				if v.Severity == SeverityWarning || v.Severity == SeverityInfo {
					parsed.Warnings = append(parsed.Warnings, "["+v.RuleName+"] "+v.Message)
				}
			}
		}
		return parsed, nil
	}

	return fallback, nil
}

// =============================================================================
// LLM 结果解析器
// =============================================================================

// claimNumPattern 匹配 "N. " 格式的权利要求编号前缀。
var claimNumPattern = regexp.MustCompile(`(\d+)\.\s+`)

// parseClaimsFromLLM 将 LLM 生成的权利要求文本解析为结构化 DraftOutput。
// text 预期格式：
//
//	权利要求书
//
//	1. preamble，其特征在于，characterized。
//	2. 根据权利要求1所述的limitation。
//	3. 根据权利要求1或2所述的limitation。
//
// 采用部分解析策略：解析成功的保留，解析失败的单条被跳过并通过 Warnings 告警。
// 仅当没有任何独立权利要求被成功解析时返回 nil（触发调用方降级到 builder）。
func parseClaimsFromLLM(text string, input DraftInput) *DraftOutput {
	claimTexts := splitClaimTexts(text)
	if len(claimTexts) == 0 {
		return nil
	}

	var indClaims, depClaims []Claim
	var parseFailures int
	for _, ct := range claimTexts {
		c := parseSingleClaim(ct)
		if c == nil {
			parseFailures++
			continue
		}
		if c.Kind == "independent" {
			indClaims = append(indClaims, *c)
		} else {
			depClaims = append(depClaims, *c)
		}
	}

	if len(indClaims) == 0 {
		return nil // 必须至少有一个独立权利要求
	}

	// claimType 取第一个独立权利要求的类型（所有独立权利要求类型一致）
	claimType := indClaims[0].ClaimType

	draftOutput := &DraftOutput{
		Claims: &ClaimSet{
			IndependentClaims: indClaims,
			DependentClaims:   depClaims,
		},
		InputMeta: struct {
			Domain       TechDomain `json:"domain"`
			ClaimType    ClaimType  `json:"claim_type"`
			FeatureCount int        `json:"feature_count"`
		}{
			Domain:       input.TechDomain,
			ClaimType:    claimType,
			FeatureCount: len(input.Features),
		},
	}

	if parseFailures > 0 {
		draftOutput.Warnings = append(draftOutput.Warnings,
			fmt.Sprintf("LLM 输出中有 %d 条权利要求解析失败，已自动跳过", parseFailures))
	}

	return draftOutput
}

// splitClaimTexts 将全文按 "N. " 模式切分为单个权利要求文本块。
func splitClaimTexts(text string) []string {
	// 去除标题行（如 "权利要求书"）
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) > 1 && strings.Contains(lines[0], "权利要求书") {
		text = lines[1]
	}

	locs := claimNumPattern.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		return nil
	}

	var claims []string
	for i, loc := range locs {
		start := loc[0]
		var end int
		if i+1 < len(locs) {
			end = locs[i+1][0]
		} else {
			end = len(text)
		}
		claims = append(claims, strings.TrimSpace(text[start:end]))
	}
	return claims
}

// parseSingleClaim 解析单条权利要求文本。
func parseSingleClaim(text string) *Claim {
	m := claimNumPattern.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	number, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}

	// 提取 "N. " 之后的正文
	dotIdx := strings.Index(text, ". ")
	if dotIdx < 0 {
		return nil
	}
	body := strings.TrimSpace(text[dotIdx+2:])

	// 去除末尾句号（支持中英文）
	body = strings.TrimSuffix(body, "。")
	body = strings.TrimSuffix(body, ".")

	// 从属权利要求：以 "根据权利要求" 开头（需优先于 "其特征在于" 检查，
	// 因为从属权利要求的限定部分也可能包含 "其特征在于"）。
	if strings.HasPrefix(body, "根据权利要求") {
		after := body[len("根据权利要求"):]

		// 查找 "所述的" 分隔位置
		sepIdx := strings.Index(after, "所述的")
		if sepIdx < 0 {
			return nil
		}
		depStr := after[:sepIdx]
		limitation := strings.TrimSpace(after[sepIdx+len("所述的"):])

		// 解析引用编号：支持 "1"、"1或2"、"1、2或3"
		depStr = strings.ReplaceAll(depStr, "、", "或")
		parts := strings.Split(depStr, "或")
		var dependsOn []int
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if n, err := strconv.Atoi(p); err == nil {
				dependsOn = append(dependsOn, n)
			}
		}
		if len(dependsOn) == 0 {
			return nil
		}

		claimType := inferClaimType(limitation)

		return &Claim{
			Number:     number,
			Kind:       "dependent",
			DependsOn:  dependsOn,
			Limitation: limitation,
			ClaimType:  claimType,
		}
	}

	// 独立权利要求：含 "其特征在于"
	if idx := strings.Index(body, "其特征在于"); idx >= 0 {
		preamble := strings.TrimSpace(body[:idx])
		preamble = strings.TrimSuffix(preamble, "，")
		preamble = strings.TrimSuffix(preamble, ",")

		characterized := strings.TrimSpace(body[idx+len("其特征在于"):])

		claimType := inferClaimType(preamble + " " + characterized)

		return &Claim{
			Number:        number,
			Kind:          "independent",
			Preamble:      preamble,
			Characterized: characterized,
			ClaimType:     claimType,
		}
	}

	return nil
}

// inferClaimType 根据文本推断权利要求类型（产品/方法）。
// 采用产品关键词优先策略：检测到"装置""设备""系统"等产品关键词时优先判定为 Product 类型，
// 避免含"方法"关键词的产品权利要求被误判为 Method。
func inferClaimType(text string) ClaimType {
	lower := strings.ToLower(text)

	// 产品关键词（优先级高）
	productKW := []string{"装置", "设备", "系统", "组合物", "电路", "器具", "仪器", "器械", "组件"}
	for _, kw := range productKW {
		if strings.Contains(lower, kw) {
			return ClaimTypeProduct
		}
	}

	// 方法关键词
	methodKW := []string{"方法", "工艺", "流程", "步骤"}
	for _, kw := range methodKW {
		if strings.Contains(lower, kw) {
			return ClaimTypeMethod
		}
	}

	return ClaimTypeProduct
}

// =============================================================================
// Prompt 构建
// =============================================================================

// buildPrompt 构建 LLM 提示词。
func (d *LLMDrafter) buildPrompt(input DraftInput) string {
	var fb, pb, eb strings.Builder
	for _, f := range input.Features {
		fmt.Fprintf(&fb, "- %s（类别：%s，重要性：%s）\n", f.Description, f.Category, f.Importance)
	}
	for _, p := range input.Problems {
		pb.WriteString("- " + p + "\n")
	}
	for _, e := range input.Effects {
		eb.WriteString("- " + e + "\n")
	}
	featuresStr := fb.String()
	problemsStr := pb.String()
	effectsStr := eb.String()

	var b strings.Builder
	b.WriteString("你是一位资深专利代理师。请根据以下技术交底书，按照中国专利法和审查指南的要求撰写权利要求书。\n\n")
	b.WriteString("## 发明名称\n")
	b.WriteString(input.Title)
	b.WriteString("\n\n")
	if input.TechDomain != "" {
		b.WriteString("## 技术领域\n")
		b.WriteString(string(input.TechDomain))
		b.WriteString("\n\n")
	}
	b.WriteString("## 技术问题\n")
	b.WriteString(problemsStr)
	b.WriteString("\n")
	b.WriteString("## 技术效果\n")
	b.WriteString(effectsStr)
	b.WriteString("\n")
	b.WriteString("## 技术特征\n")
	b.WriteString(featuresStr)
	b.WriteString("\n")
	b.WriteString("## 撰写要求\n")
	b.WriteString("1. 独立权利要求采用前序部分+特征部分的两段式写法\n")
	b.WriteString("2. 从属权利要求按技术方案层次递进布局\n")
	b.WriteString("3. 确保清楚、简要、得到说明书支持\n")
	b.WriteString("4. 避免使用约/大约/厚/薄等不确定用语\n")
	b.WriteString("5. 功能性限定需谨慎使用\n")
	b.WriteString("6. 从属权利要求只能引用在前的权利要求\n")
	b.WriteString("7. 多项从属只能择一引用（用或），不得用和\n")
	b.WriteString("8. 实用新型只能有产品权利要求\n\n")
	b.WriteString("请按以下格式输出完整的权利要求书，每条权利要求独占一行：\n\n")
	b.WriteString("权利要求书\n\n")
	b.WriteString("1. 【前序部分】，其特征在于，【特征部分】。\n")
	b.WriteString("2. 根据权利要求1所述的【限定部分】。\n")
	b.WriteString("3. 根据权利要求1或2所述的【限定部分】。\n")

	return b.String()
}
