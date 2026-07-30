package agentcore

import (
	"maps"
	"strings"
	"sync"
)

// =============================================================================
// ReflectionEngine — 输出质量自动评估
//
// 参考 BCIP 的 ReflectionEngine (codex-patent-agents/reflection.rs)，
// 在 AfterModelCall 或 AfterTurn 中运行，自动检查 Agent 输出质量。
// 不依赖 LLM，纯启发式规则，零额外成本。
//
// 质量维度：
//   - 输出长度（过短→可能不完整）
//   - 结构完整性（章节标题是否存在）
//   - 领域关键词覆盖率（核心术语是否出现）
//   - 错误标记检测（错误/失败/无法完成等）
// =============================================================================

// ReflectionConfig 配置 ReflectionEngine 的行为。
type ReflectionConfig struct {
	// MinLength 是最小期望输出字符数（默认 100）。
	MinLength int

	// MaxLength 是最长期望输出字符数（0=不限制）。
	MaxLength int

	// RequiredSections 是期望出现的章节标题列表。
	RequiredSections []string

	// DomainKeywords 是领域关键词到角色 ID 的映射。
	// 用于检查特定角色的输出是否包含核心术语。
	DomainKeywords map[string][]string

	// ErrorPatterns 是错误标记的关键词列表。
	// 默认值：[ "错误", "无法完成", "失败", "failed", "error" ]
	ErrorPatterns []string
}

// DefaultReflectionConfig 返回适用于专利/法律领域的默认配置。
func DefaultReflectionConfig() ReflectionConfig {
	return ReflectionConfig{
		MinLength: 100,
		MaxLength: 0,
		RequiredSections: []string{
			"结论", "分析",
		},
		DomainKeywords: map[string][]string{
			"patent": {"专利", "权利要求", "技术方案", "实施例", "审查"},
			"legal":  {"法律", "法规", "第", "条", "规定", "法院"},
		},
		ErrorPatterns: []string{
			"错误", "无法完成", "失败", "failed", "error",
			"exception", "unavailable", "timeout",
		},
	}
}

// ReflectionResult 是 ReflectionEngine 的输出。
type ReflectionResult struct {
	// QualityScore 综合质量分 [0.0, 1.0]。
	QualityScore float64 `json:"quality_score"`

	// Issues 是发现的质量问题列表。
	Issues []ReflectionIssue `json:"issues,omitempty"`

	// Passed 表示是否通过所有检查。
	Passed bool `json:"passed"`
}

// ReflectionIssue 描述一个具体的质量问题。
type ReflectionIssue struct {
	// Severity 表示严重程度："error", "warning", "info"。
	Severity string `json:"severity"`

	// Category 表示问题类别："length", "structure", "keyword", "error_signal"。
	Category string `json:"category"`

	// Message 是问题描述。
	Message string `json:"message"`

	// Score 是该项的扣分 [0.0, 1.0]。
	Score float64 `json:"score,omitempty"`
}

// ReflectionEngine 提供轻量级输出质量自动评估。
// 设计为在 Agent 的 AfterTurn 生命周期钩子中调用。
type ReflectionEngine struct {
	config ReflectionConfig
	mu     sync.RWMutex
}

// NewReflectionEngine 创建反射引擎。
func NewReflectionEngine(config ReflectionConfig) *ReflectionEngine {
	if config.MinLength <= 0 {
		config.MinLength = 100
	}
	if len(config.ErrorPatterns) == 0 {
		config.ErrorPatterns = DefaultReflectionConfig().ErrorPatterns
	}
	if config.DomainKeywords == nil {
		config.DomainKeywords = maps.Clone(DefaultReflectionConfig().DomainKeywords)
		if config.DomainKeywords == nil {
			config.DomainKeywords = make(map[string][]string)
		}
	}
	return &ReflectionEngine{config: config}
}

// Reflect 对 Agent 输出执行质量检查。
// content 是 Agent 生成的文本内容。
// domain 是可选的领域标识（如 "patent", "legal"），用于领域关键词检查。
// roleID 是可选的角色标识，用于指定关键词检查范围。
//
//nolint:gocognit // 原因：反思检查引擎，含多维度质量评估和关键词检查
func (e *ReflectionEngine) Reflect(content string, domain, roleID string) ReflectionResult {
	if strings.TrimSpace(content) == "" {
		return ReflectionResult{
			QualityScore: 0,
			Issues: []ReflectionIssue{{
				Severity: "error",
				Category: "length",
				Message:  "输出为空",
				Score:    1.0,
			}},
			Passed: false,
		}
	}

	var issues []ReflectionIssue
	var totalDeduction float64

	// 1. 长度检查。
	length := len([]rune(content))
	if length < e.config.MinLength {
		deduction := min(1.0, float64(e.config.MinLength-length)/float64(e.config.MinLength+1))
		issues = append(issues, ReflectionIssue{
			Severity: "warning",
			Category: "length",
			Message:  "输出过短",
			Score:    deduction,
		})
		totalDeduction += deduction
	} else if e.config.MaxLength > 0 && length > e.config.MaxLength {
		deduction := min(0.3, float64(length-e.config.MaxLength)/float64(length))
		issues = append(issues, ReflectionIssue{
			Severity: "warning",
			Category: "length",
			Message:  "输出过长",
			Score:    deduction,
		})
		totalDeduction += deduction
	}

	// 2. 结构完整性检查。
	if len(e.config.RequiredSections) > 0 {
		contentLower := strings.ToLower(content)
		var missing []string
		for _, section := range e.config.RequiredSections {
			if !strings.Contains(contentLower, strings.ToLower(section)) {
				missing = append(missing, section)
			}
		}
		if len(missing) > 0 {
			deduction := min(0.4, float64(len(missing))*0.15)
			issues = append(issues, ReflectionIssue{
				Severity: "warning",
				Category: "structure",
				Message:  "缺少必要章节: " + strings.Join(missing, ", "),
				Score:    deduction,
			})
			totalDeduction += deduction
		}
	}

	// 3. 领域关键词检查。
	if domain != "" {
		e.mu.RLock()
		keywords := e.config.DomainKeywords[domain]
		e.mu.RUnlock()
		if len(keywords) > 0 {
			contentLower := strings.ToLower(content)
			var missingKeywords []string
			for _, kw := range keywords {
				if !strings.Contains(contentLower, strings.ToLower(kw)) {
					missingKeywords = append(missingKeywords, kw)
				}
			}
			if len(missingKeywords) > 0 {
				// 只有大部分关键词缺失时才扣分。
				ratio := float64(len(missingKeywords)) / float64(len(keywords))
				if ratio > 0.5 {
					deduction := min(0.3, ratio*0.25)
					issues = append(issues, ReflectionIssue{
						Severity: "info",
						Category: "keyword",
						Message:  "缺少领域核心术语: " + strings.Join(missingKeywords, ", "),
						Score:    deduction,
					})
					totalDeduction += deduction
				}
			}
		}
	}

	// 4. 错误标记检测。
	contentLower := strings.ToLower(content)
	for _, pattern := range e.config.ErrorPatterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			issues = append(issues, ReflectionIssue{
				Severity: "error",
				Category: "error_signal",
				Message:  "检测到错误标记: " + pattern,
				Score:    0.5,
			})
			totalDeduction += 0.5
			break
		}
	}

	// 计算最终质量分。当发现任何问题时 passed=false。
	quality := 1.0 - min(1.0, totalDeduction)
	passed := len(issues) == 0

	return ReflectionResult{
		QualityScore: quality,
		Issues:       issues,
		Passed:       passed,
	}
}

// UpdateDomainKeywords 动态更新领域关键词映射。
func (e *ReflectionEngine) UpdateDomainKeywords(domain string, keywords []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.DomainKeywords[domain] = keywords
}
