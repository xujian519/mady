package claimdrafting

import (
	"fmt"
	"strings"
)

// =============================================================================
// LLMDrafter LLM 撰写器
// =============================================================================

// LLMDrafter 通过 LLM 增强权利要求撰写质量。
// 当 LLM provider 可用时，提供优化表述、从零撰写、措辞优化等能力。
// 当 provider 不可用时，降级为纯规则引擎模式。
type LLMDrafter struct {
	provider   Provider // LLM provider 接口
	builder    *ClaimBuilder
	promptTmpl string // 提示词模板
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
func NewLLMDrafter(provider Provider, builder *ClaimBuilder) *LLMDrafter {
	return &LLMDrafter{
		provider: provider,
		builder:  builder,
	}
}

// DraftFromScratch 使用 LLM 从头撰写权利要求书。
// 当 provider 不可用时，降级为 builder 的规则引擎生成。
func (d *LLMDrafter) DraftFromScratch(input DraftInput) (*DraftOutput, error) {
	// interface nil guard
	if d == nil || d.provider == nil || !d.provider.Available() {
		return d.builder.Build(input)
	}

	// 构建提示词
	prompt := d.buildPrompt(input)

	// 调用 LLM
	result, err := d.provider.Complete(prompt)
	if err != nil {
		// LLM 失败时降级
		return d.builder.Build(input)
	}

	// TODO(下一阶段): 实现 LLM 返回结果的精确解析（当前降级到规则引擎）
	_ = result
	return d.builder.Build(input)
}

// buildPrompt 构建 LLM 提示词。
func (d *LLMDrafter) buildPrompt(input DraftInput) string {
	var fb, pb, eb strings.Builder
	for _, f := range input.Features {
		fb.WriteString(fmt.Sprintf("- %s（类别：%s，重要性：%s）\n", f.Description, f.Category, f.Importance))
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
	b.WriteString("请输出完整的权利要求书（以权利要求书为标题），包含独立权利要求和从属权利要求。")

	return b.String()
}

// parseClaimResult 解析 LLM 返回的权利要求文本。
// 这是一个基础实现，后续可通过更精确的 NLP 解析增强。
func parseClaimResult(_ string) ([]Claim, []string) {
	// 基础实现：返回空列表和简单的提示信息
	return nil, []string{"LLM 生成了权利要求，请人工审核格式"}
}

// filterClaimsByKind 按类型过滤权利要求。
func filterClaimsByKind(claims []Claim, kind string) []Claim {
	var filtered []Claim
	for _, c := range claims {
		if c.Kind == kind {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// extractSuggestionMessages 从违规列表中提取建议消息。
func extractSuggestionMessages(violations []Violation) []string {
	var messages []string
	seen := make(map[string]bool)
	for _, v := range violations {
		if v.Suggestion != "" && !seen[v.Suggestion] {
			messages = append(messages, "["+v.RuleName+"] "+v.Suggestion)
			seen[v.Suggestion] = true
		}
	}
	return messages
}
