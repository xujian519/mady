package reasoning

import (
	"fmt"
	"strings"
)

// CaseSummary is a compact, human-readable distillation of a StageCheckpoint's
// FactBlackboard. It is used by the Tier 1 case-memory preheating pipeline (B2)
// to inject prior case context into the Agent's memory when resuming a case.
//
// The summary deliberately avoids embedding the full blackboard JSON — it
// extracts only the fields an Agent needs to "remember" a case at a glance:
// identity, technical domain, current workflow position, and fact count.
type CaseSummary struct {
	CaseID         string `json:"case_id"`
	CaseType       string `json:"case_type"`
	TechnicalField string `json:"technical_field"`
	CurrentStage   int    `json:"current_stage"`
	FactCount      int    `json:"fact_count"`
	WorkflowID     string `json:"workflow_id,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ExtractCaseSummary builds a CaseSummary from a StageCheckpoint. If the
// checkpoint or its blackboard is nil, a minimal summary with just the
// checkpoint-level fields is returned.
func ExtractCaseSummary(cp *StageCheckpoint) CaseSummary {
	s := CaseSummary{
		CaseID:       cp.CaseID,
		CaseType:     string(cp.CaseType),
		CurrentStage: cp.CurrentStage,
	}
	if cp.Blackboard != nil {
		s.TechnicalField = cp.Blackboard.TechnicalField
		s.CreatedAt = cp.Blackboard.CreatedAt
		s.UpdatedAt = cp.Blackboard.UpdatedAt
		s.FactCount = len(cp.Blackboard.ActiveFacts())
		s.WorkflowID = cp.Blackboard.WorkflowID()
	}
	return s
}

// String formats the summary as a compact text block suitable for storage as
// a memory entry. The format is designed to be keyword-rich for retrieval.
func (s CaseSummary) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("案件记忆 | 案件编号: %s", s.CaseID))
	b.WriteString(fmt.Sprintf(" | 案件类型: %s", s.CaseType))
	if s.TechnicalField != "" {
		b.WriteString(fmt.Sprintf(" | 技术领域: %s", s.TechnicalField))
	}
	b.WriteString(fmt.Sprintf(" | 当前阶段: %d", s.CurrentStage))
	b.WriteString(fmt.Sprintf(" | 已记录事实: %d条", s.FactCount))
	if s.WorkflowID != "" {
		b.WriteString(fmt.Sprintf(" | 工作流: %s", s.WorkflowID))
	}
	if s.CreatedAt != "" {
		b.WriteString(fmt.Sprintf(" | 创建: %s", s.CreatedAt))
	}
	if s.UpdatedAt != "" {
		b.WriteString(fmt.Sprintf(" | 更新: %s", s.UpdatedAt))
	}
	return b.String()
}
