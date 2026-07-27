package domains

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

const auditExtName = "audit"

// AuditExtension 将审计功能封装为 agentcore.Extension。
// 通过 Observer 接口自动监听 Agent 运行事件并写入 JSONL 审计日志。
type AuditExtension struct {
	logger    *AuditLogger
	enc       *Encryptor
	retention DataRetentionConfig
	projectID string
}

var (
	_ agentcore.Extension              = (*AuditExtension)(nil)
	_ agentcore.AgentRunObserver       = (*AuditExtension)(nil)
	_ agentcore.ToolCallObserver       = (*AuditExtension)(nil)
	_ agentcore.MessagePersistObserver = (*AuditExtension)(nil)
)

// NewAuditExtension 创建审计扩展。
// baseDir 是审计日志存储目录的父目录（实际存储在 baseDir/audit/ 下）。
// projectID 是审计记录的项目标识。当 baseDir 为空时返回 nil（审计禁用）。
func NewAuditExtension(baseDir string, projectID string) (*AuditExtension, error) {
	if baseDir == "" {
		return nil, nil // audit disabled
	}
	auditDir := filepath.Join(baseDir, "audit")
	logger, err := NewAuditLogger(auditDir)
	if err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}
	return &AuditExtension{
		logger:    logger,
		enc:       NewEncryptor(),
		retention: DefaultRetentionConfig(),
		projectID: projectID,
	}, nil
}

// Name returns the extension identifier.
func (e *AuditExtension) Name() string { return auditExtName }

// Init initializes the audit extension with the given agent context.
func (e *AuditExtension) Init(_ context.Context, _ *agentcore.Agent) error {
	if e.logger != nil {
		slog.Info("审计扩展已初始化", "project", e.projectID)
	}
	return nil
}

// Dispose cleans up the audit logger, flushing and closing the underlying writer.
func (e *AuditExtension) Dispose() error {
	if e.logger != nil {
		return e.logger.Close()
	}
	return nil
}

// AuditEnabled 返回审计是否已启用。
func (e *AuditExtension) AuditEnabled() bool {
	return e.logger != nil
}

// ---------- AgentRunObserver ----------

// AfterAgentRun logs the completion of an agent run to the audit trail.
func (e *AuditExtension) AfterAgentRun(ctx context.Context, arc *agentcore.AgentRunContext, output string, err error) {
	if !e.AuditEnabled() {
		return
	}
	projectID := e.resolveProjectID(arc)
	description := "Agent 运行结束"
	details := truncateAuditDetail(fmt.Sprintf("turn=%d, input_len=%d, output_len=%d", arc.Turn, len(arc.Input), len(output)))
	success := err == nil
	e.logger.Log(AuditAccess, projectID, "agent", description, success, details)
}

// BeforeAgentRun is a no-op hook that satisfies the extension interface.
func (e *AuditExtension) BeforeAgentRun(_ context.Context, _ *agentcore.AgentRunContext) error {
	return nil
}

// ---------- ToolCallObserver ----------

// AfterToolExecution logs tool call details to the audit trail.
func (e *AuditExtension) AfterToolExecution(ctx context.Context, arc *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) {
	if !e.AuditEnabled() || len(tec.ToolCalls) == 0 {
		return
	}
	projectID := e.resolveProjectID(arc)
	for i, tc := range tec.ToolCalls {
		success := true
		if i < len(tec.Results) && tec.Results[i].Err != nil {
			success = false
		}
		// 记录工具名和参数名称列表（不记录参数值，保护敏感信息）
		argNames := extractArgNames(tc.Arguments)
		description := fmt.Sprintf("工具调用: %s", tc.Name)
		details := truncateAuditDetail(fmt.Sprintf("参数: [%s]", strings.Join(argNames, ", ")))
		e.logger.Log(AuditModify, projectID, "agent", description, success, details)
	}
}

// BeforeToolExecution is a no-op hook that satisfies the extension interface.
func (e *AuditExtension) BeforeToolExecution(_ context.Context, _ *agentcore.AgentRunContext, _ *agentcore.ToolExecutionContext) error {
	return nil
}

// ---------- MessagePersistObserver ----------

// AfterMessagePersist logs message persistence events to the audit trail.
func (e *AuditExtension) AfterMessagePersist(ctx context.Context, arc *agentcore.AgentRunContext, msg agentcore.Message) {
	if !e.AuditEnabled() {
		return
	}
	projectID := e.resolveProjectID(arc)
	role := string(msg.Role)
	description := fmt.Sprintf("消息持久化: %s", role)
	details := truncateAuditDetail(fmt.Sprintf("content_len=%d", len(msg.Content)))
	e.logger.Log(AuditModify, projectID, "agent", description, true, details)
}

// BeforeMessagePersist is a no-op hook that satisfies the extension interface.
func (e *AuditExtension) BeforeMessagePersist(_ context.Context, _ *agentcore.AgentRunContext, _ *agentcore.Message) error {
	return nil
}

// ---------- 辅助函数 ----------

// resolveProjectID 从 AgentRunContext 中提取案件 ID，回退到扩展级别的 projectID。
func (e *AuditExtension) resolveProjectID(arc *agentcore.AgentRunContext) string {
	if arc != nil && arc.CaseID != "" {
		return arc.CaseID
	}
	return e.projectID
}

// truncateAuditDetail 截断审计详情到 500 字符以内。
func truncateAuditDetail(detail string) string {
	if len(detail) <= 500 {
		return detail
	}
	return detail[:497] + "..."
}

// extractArgNames 从 JSON 参数字符串中提取键名列表，不包含值。
func extractArgNames(rawJSON string) []string {
	if rawJSON == "" || rawJSON == "null" || rawJSON == "{}" {
		return nil
	}
	cleaned := strings.TrimSpace(rawJSON)
	if !strings.HasPrefix(cleaned, "{") {
		return []string{"<non-json>"}
	}
	// 只匹配引号包裹的键名（后跟冒号的字符串）
	var names []string
	raw := []byte(cleaned)
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			start := i + 1
			j := start
			for j < len(raw) && raw[j] != '"' {
				if raw[j] == '\\' {
					j++ // skip escaped char
				}
				j++
			}
			key := string(raw[start:j])
			i = j
			// 检查后面是否跟冒号（是键名）
			next := strings.TrimLeft(string(raw[j+1:]), " \t\r\n")
			if strings.HasPrefix(next, ":") {
				names = append(names, key)
			}
		}
	}
	return names
}
