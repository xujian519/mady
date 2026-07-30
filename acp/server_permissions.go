package acp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xujian519/mady/domains"
)

// ---------------------------------------------------------------------------
// Permission operations: RequestPermission, DefaultPermissionOptions,
// permissionDecisionFor, recordPermissionDecision

// RequestPermission asks the client (editor) to authorize a tool call and
// returns the user's outcome. Mirrors ACP's session/request_permission.
func (s *Server) RequestPermission(sessionID string, tc PermissionToolCall, options []PermissionOption) (*PermissionOutcome, error) {
	raw, err := s.sendRequest("session/request_permission", RequestPermissionParams{
		SessionID: sessionID,
		ToolCall:  tc,
		Options:   options,
	}, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	var res RequestPermissionResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res.Outcome, nil
}

// DefaultPermissionOptions are the standard allow/reject choices presented to
// the user for a tool-call permission request.
func DefaultPermissionOptions() []PermissionOption {
	return []PermissionOption{
		{OptionID: "allow_once", Name: "Allow", Kind: "allow_once"},
		{OptionID: "allow_always", Name: "Always allow", Kind: "allow_always"},
		{OptionID: "reject_once", Name: "Reject", Kind: "reject_once"},
	}
}

// permissionDecisionFor 把工具授权的 allow/deny 布尔结果映射为审批决策枚举：
// allow → adopted（人工放行），deny → rejected（人工拒绝）。
func permissionDecisionFor(allow bool) domains.ApprovalDecision {
	if allow {
		return domains.DecisionAdopted
	}
	return domains.DecisionRejected
}

// recordPermissionDecision 将编辑器端的人工工具授权结论留痕到 ApprovalStore，
// 与 TUI /approve /reject、Server /review 端点共用同一 RecordDecision 模式。
// 未配置 store 时为 no-op；记录失败仅记日志，绝不阻断授权主流程。
func (s *Server) recordPermissionDecision(ctx context.Context, sessionID, toolName string, rawInput any, decision domains.ApprovalDecision, feedback string) {
	if s.approvalStore == nil {
		return
	}
	original := ""
	if rawInput != nil {
		if data, err := json.Marshal(rawInput); err == nil {
			original = string(data)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := domains.RecordApprovalDecision(
		ctx, s.approvalStore,
		sessionID, "", "tool_permission:"+toolName, original,
		decision, "", feedback,
	); err != nil {
		s.logger.Warn("acp: 记录工具授权决策失败", "session_id", sessionID, "tool", toolName, "error", err)
	}
}
