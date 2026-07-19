// Package writing provides pattern-based writing guidance for patent/legal drafting.
//
// The writing package implements a Skill Distillation system that extracts reusable
// writing patterns from high-quality patent/legal documents and makes them available
// to Agents as queryable resources.
//
// Architecture:
//
//	seed-patterns/*.yaml (hand-crafted seed patterns)
//	  → pattern_store.go (in-memory store + search)
//	    → skill_compiler.go (match + compile to XML)
//	      → writing_extension.go (query_writing_patterns tool)
//	    → quality_evaluator.go (4-dimension scoring + user feedback)
package writing

import (
	"fmt"
	"strings"
)

// WritingPattern 是一个可应用的写作模式/技巧。
// 它封装了从优秀案例中提取的写作方法论——不是"查什么"而是"怎么写"。
type WritingPattern struct {
	ID          string      `yaml:"id" json:"id"`
	Name        string      `yaml:"name" json:"name"`
	Category    string      `yaml:"category" json:"category"`
	SubCategory string      `yaml:"sub_category,omitempty" json:"sub_category,omitempty"`
	Summary     string      `yaml:"summary" json:"summary"`
	Context     string      `yaml:"context,omitempty" json:"context,omitempty"`
	Steps       []Step      `yaml:"steps,omitempty" json:"steps,omitempty"`
	Examples    []Example   `yaml:"examples,omitempty" json:"examples,omitempty"`
	Dos         []Principle `yaml:"dos,omitempty" json:"dos,omitempty"`
	Donts       []Principle `yaml:"donts,omitempty" json:"donts,omitempty"`
	SourceRef   string      `yaml:"source_ref,omitempty" json:"source_ref,omitempty"`
	Quality     float64     `yaml:"quality" json:"quality"`
	Version     int         `yaml:"version,omitempty" json:"version,omitempty"`
}

// Step 是写作模式中的一个步骤。
type Step struct {
	Order       int    `yaml:"order" json:"order"`
	Name        string `yaml:"name" json:"name"`
	Instruction string `yaml:"instruction" json:"instruction"`
	Example     string `yaml:"example,omitempty" json:"example,omitempty"`
}

// Example 是一个写作示例。
type Example struct {
	Context string `yaml:"context,omitempty" json:"context,omitempty"`
	Text    string `yaml:"text" json:"text"`
	Note    string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Principle 是一个写作原则。
type Principle struct {
	Rule    string `yaml:"rule" json:"rule"`
	Example string `yaml:"example,omitempty" json:"example,omitempty"`
}

// PatternCategory 定义了 WritingPattern 的预定义分类。
type PatternCategory string

const (
	CatOAInventiveness PatternCategory = "oa_inventiveness" // OA 答复-创造性
	CatOANovelty       PatternCategory = "oa_novelty"       // OA 答复-新颖性
	CatOAClarity       PatternCategory = "oa_clarity"       // OA 答复-不清楚
	CatClaimDrafting   PatternCategory = "claim_drafting"   // 权利要求撰写
	CatSpecDrafting    PatternCategory = "spec_drafting"    // 说明书撰写
	CatDisclosure      PatternCategory = "disclosure"       // 技术交底书
	CatInvalidation    PatternCategory = "invalidation"     // 无效请求
	CatIPCStrategy     PatternCategory = "ipc_strategy"     // IPC 策略
	CatEmbodiment      PatternCategory = "embodiment"       // 具体实施方式
)

// String returns a human-readable summary.
func (p *WritingPattern) String() string {
	return fmt.Sprintf("[%s] %s (%s)", p.Category, p.Name, p.Summary)
}

// ApplicableCategories returns category tags for this pattern.
// Used by MatchPatterns to filter.
func (p *WritingPattern) ApplicableCategories() []string {
	cats := []string{p.Category}
	if p.SubCategory != "" {
		cats = append(cats, p.SubCategory)
	}
	return cats
}

// Keywords returns searchable keywords extracted from the pattern.
func (p *WritingPattern) Keywords() []string {
	var kw []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if len([]rune(s)) >= 2 && !seen[s] {
			seen[s] = true
			kw = append(kw, s)
		}
	}
	add(p.Name)
	add(p.Category)
	if p.SubCategory != "" {
		add(p.SubCategory)
	}
	for _, s := range p.Steps {
		add(s.Name)
	}
	for _, d := range p.Dos {
		add(d.Rule)
	}
	for _, d := range p.Donts {
		add(d.Rule)
	}
	return kw
}
