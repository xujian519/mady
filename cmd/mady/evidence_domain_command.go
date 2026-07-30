package main

// evidence_domain_command.go implements /evidence-domain slash command for
// viewing the status of evidence domain rules (triple-aspect, burden of proof,
// standard of proof, conflict detection, credibility).
//
// Data source: fc.EvidenceExt (evidence.Extension) and the domain evidence
// extension injected in bootstrap/setup.go.

import (
	"fmt"
	"strings"
)

// handleEvidenceDomainCommand shows the status of evidence domain rules.
func (s *tuiSession) handleEvidenceDomainCommand(sub string) {
	_ = sub // reserved for future sub-commands (e.g., /evidence-domain rules)

	var b strings.Builder
	b.WriteString("⚖ 证据判断规则引擎\n\n")

	// Evidence extension status.
	if s.fc.EvidenceExt != nil {
		ledger := s.fc.EvidenceExt.Ledger()
		if ledger != nil {
			fmt.Fprintf(&b, "  · 工具调用账本: 在线（%d 条记录）\n", ledger.Len())
		} else {
			b.WriteString("  · 工具调用账本: 在线\n")
		}
	} else {
		b.WriteString("  · 工具调用账本: 未加载\n")
	}

	// Domain evidence extension (triple-aspect/type/burden/standard/etc.).
	if s.fc.EvidenceExt != nil {
		b.WriteString("  · 证据类型判断: 已注入\n")
	} else {
		b.WriteString("  · 证据类型判断: 未注入\n")
	}

	// Knowledge backend status.
	if s.fc.KnowledgeBackend != nil {
		b.WriteString("  · 知识检索后端: 已加载\n")
	} else if s.fc.WikiStore != nil {
		b.WriteString("  · 知识检索后端: Wiki 存储可用\n")
	} else {
		b.WriteString("  · 知识检索后端: 未加载\n")
	}

	// Citation gate status.
	if s.fc.WikiRoot != "" {
		b.WriteString("  · 法条引用核验: 可用（S1 静态表 + S2 知识源）\n")
	} else {
		b.WriteString("  · 法条引用核验: S1 静态表可用\n")
	}

	b.WriteString("\n可用命令:\n")
	b.WriteString("  /evidence [关键词] — 查看引用证据详情\n")
	b.WriteString("  /ledger — 查看本轮工具调用证据\n")
	b.WriteString("  /knowledge [关键词] — 检索知识库\n")

	s.app.PrintSystem(b.String())
}
