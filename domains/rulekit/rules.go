// Package rulekit 提供规则引擎的公共骨架，供领域规则模块（claimdrafting /
// specdrafting 等）复用。
//
// 背景：claimdrafting 与 specdrafting 曾各自实现一份 RuleEngine（注册/批量
// 校验/按严重度分组）与 baseRule 骨架，scorer 的扣分权重与等级划分也同构。
// 本包收敛这些真正同构的机制；各领域的规则数据（具体规则类型、词表）与
// Violation 的领域语义差异（claim 的 ClaimNumber vs spec 的 SectionName）
// 作为实质差异保留在各模块。
//
// 使用方式（以 claimdrafting 为例）：
//
//	type ClaimRule = rulekit.Rule[[]Claim, DraftInput]
//	type RuleEngine = rulekit.Engine[[]Claim, DraftInput]
//
// 各模块通过类型别名接入，现有构造 Violation / 引用 Severity 常量的代码
// 无需改动。
package rulekit

import (
	"strings"
	"sync"
)

// Severity 表示违规的严重程度。
type Severity string

// Severity level constants.
const (
	SeverityError   Severity = "error"   // 严重违法（如多项从属互引）
	SeverityWarning Severity = "warning" // 潜在风险（如使用不确定用语）
	SeverityInfo    Severity = "info"    // 建议改进（如保护范围可优化）
)

// Violation 记录一条规则违规信息。
// 两领域通用字段 + 领域可选字段：claimdrafting 使用 ClaimNumber，
// specdrafting 使用 SectionName；未使用方因 omitempty 不参与序列化。
type Violation struct {
	RuleName    string   `json:"rule_name"`
	RuleBasis   string   `json:"rule_basis,omitempty"` // 法律依据
	Severity    Severity `json:"severity"`
	Message     string   `json:"message"`                // 违规描述
	Suggestion  string   `json:"suggestion"`             // 修改建议
	ClaimNumber int      `json:"claim_number,omitempty"` // 关联的权利要求编号（0 表示整体问题，claimdrafting 用）
	SectionName string   `json:"section_name,omitempty"` // 关联的章节名（specdrafting 用）
}

// Rule 是一条验证规则：对 item（检查对象）在 ctx（上下文）下执行检查，
// 返回违规列表。
type Rule[T, C any] interface {
	// Name 返回规则唯一标识符（kebab-case，如 "clarity-wording"）。
	Name() string
	// Description 返回规则的人类可读描述。
	Description() string
	// LegalBasis 返回法律依据（如"专利法第26条第4款"）。
	LegalBasis() string
	// Check 执行检查，返回违规列表。
	Check(item T, ctx C) []Violation
}

// Engine 管理一组验证规则，提供批量验证能力（线程安全）。
type Engine[T, C any] struct {
	mu    sync.RWMutex
	rules []Rule[T, C]
}

// NewEngine 创建一个空的规则引擎。
func NewEngine[T, C any]() *Engine[T, C] {
	return &Engine[T, C]{rules: make([]Rule[T, C], 0)}
}

// Register 注册一条规则（线程安全）。
func (e *Engine[T, C]) Register(rule Rule[T, C]) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// RegisterAll 批量注册规则（线程安全）。
func (e *Engine[T, C]) RegisterAll(rules ...Rule[T, C]) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rules...)
}

// Rules 返回当前注册的所有规则。
func (e *Engine[T, C]) Rules() []Rule[T, C] {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := make([]Rule[T, C], len(e.rules))
	copy(cp, e.rules)
	return cp
}

// Validate 执行所有注册规则的检查，返回违规列表。
func (e *Engine[T, C]) Validate(item T, ctx C) []Violation {
	e.mu.RLock()
	defer e.mu.RUnlock()
	all := make([]Violation, 0, len(e.rules))
	for _, rule := range e.rules {
		all = append(all, rule.Check(item, ctx)...)
	}
	return all
}

// ValidateAndGroup 执行检查并按严重程度分组。
func (e *Engine[T, C]) ValidateAndGroup(item T, ctx C) (errors, warnings, infos []Violation) {
	for _, v := range e.Validate(item, ctx) {
		switch v.Severity {
		case SeverityError:
			errors = append(errors, v)
		case SeverityWarning:
			warnings = append(warnings, v)
		case SeverityInfo:
			infos = append(infos, v)
		}
	}
	return
}

// BaseRule 提供规则实现的公用字段（嵌入 Rule 实现）。
type BaseRule struct {
	name        string
	description string
	legalBasis  string
}

// Name 返回规则唯一标识符。
func (r *BaseRule) Name() string { return r.name }

// Description 返回规则的人类可读描述。
func (r *BaseRule) Description() string { return r.description }

// LegalBasis 返回法律依据。
func (r *BaseRule) LegalBasis() string { return r.legalBasis }

// NewBaseRule 创建基础规则字段。
func NewBaseRule(name, desc, basis string) BaseRule {
	return BaseRule{name: name, description: desc, legalBasis: basis}
}

// ContainsAny 检查字符串 s 是否包含 words 中任一词汇，返回命中的词。
func ContainsAny(s string, words []string) (string, bool) {
	for _, w := range words {
		if strings.Contains(s, w) {
			return w, true
		}
	}
	return "", false
}
