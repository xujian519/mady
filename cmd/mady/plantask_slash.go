package main

// plantask_slash.go 定义 PlanTask HCL 的 TUI 交互命令：
//
//	/interrupt [session_id] — 请求暂停当前执行（会话 → AwaitingFeedback）
//	/resume    [session_id] — 无改动直接恢复执行（→ Executing）
//	/feedback <文本> [session_id] — 注入反馈并重新规划（→ Replanning → Executing）
//
// 命令直接调用 plantask 扩展的工具 Func（复用状态机校验与事件发射），
// 并通过既有 runCancel / resumeIfInterrupted 机制联动 Agent 运行循环。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/plantask"
)

// plantaskExt 返回当前会话的 plantask 扩展（未启用时 nil）。
func (s *tuiSession) plantaskExt() *plantask.Extension {
	if s == nil || s.fc == nil {
		return nil
	}
	return s.fc.PlantaskExt
}

// resolvePlantaskSessionID 解析会话 ID：命令参数优先，否则取最近活动会话。
func (s *tuiSession) resolvePlantaskSessionID(arg string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	ext := s.plantaskExt()
	if ext == nil {
		return "", fmt.Errorf("PlanTask 扩展未启用")
	}
	sess, err := ext.LatestSession(context.Background())
	if err != nil {
		return "", fmt.Errorf("无活动 HCL 会话（先提交计划：plan_submit）")
	}
	return sess.ID, nil
}

// callPlantaskTool 按名称调用 plantask 工具（复用状态机校验与事件）。
func (s *tuiSession) callPlantaskTool(toolName string, args any) (any, error) {
	ext := s.plantaskExt()
	if ext == nil {
		return nil, fmt.Errorf("PlanTask 扩展未启用")
	}
	for _, t := range ext.Tools() {
		if t.Name != toolName {
			continue
		}
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		return t.Func(context.Background(), raw)
	}
	return nil, fmt.Errorf("plantask 工具 %s 未找到", toolName)
}

// interruptCurrentRun 取消正在运行的 Agent（与 Ctrl+C 的 OnInterrupt 一致）。
func (s *tuiSession) interruptCurrentRun() {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.runCancel != nil {
		s.runCancel()
		s.runCancel = nil
	}
}

// handlePlantaskInterrupt 实现 /interrupt。
func (s *tuiSession) handlePlantaskInterrupt(input string) {
	arg := strings.TrimSpace(strings.TrimPrefix(input, "/interrupt"))
	sid, err := s.resolvePlantaskSessionID(arg)
	if err != nil {
		s.app.PrintSystem("⏸️  " + err.Error())
		return
	}
	// 先暂停运行中的 Agent，再迁移会话状态。
	s.interruptCurrentRun()
	_, err = s.callPlantaskTool("workflow_interrupt", plantask.WorkflowInterruptArgs{
		SessionID: sid,
		Reason:    "用户通过 /interrupt 请求暂停",
	})
	if err != nil && !errors.Is(err, agentcore.ErrInterrupt) {
		s.app.PrintSystem("⏸️  暂停失败: " + err.Error())
		return
	}
	s.app.PrintSystem("⏸️  已暂停执行，等待反馈。可用 /feedback 注入改进意见，或 /resume 直接续跑。")
}

// handlePlantaskResume 实现 /resume。
func (s *tuiSession) handlePlantaskResume(input string) {
	arg := strings.TrimSpace(strings.TrimPrefix(input, "/resume"))
	sid, err := s.resolvePlantaskSessionID(arg)
	if err != nil {
		s.app.PrintSystem("▶️  " + err.Error())
		return
	}
	res, err := s.callPlantaskTool("workflow_resume", plantask.WorkflowResumeArgs{SessionID: sid})
	if err != nil {
		s.app.PrintSystem("▶️  恢复失败: " + err.Error())
		return
	}
	s.app.PrintSystem(fmt.Sprintf("▶️  %s", resultMessage(res)))
	// 联动 Agent：优先从中断点恢复，否则续发执行指令。
	s.resumeIfInterrupted()
}

// handlePlantaskFeedback 实现 /feedback <文本> [session_id]。
func (s *tuiSession) handlePlantaskFeedback(input string) {
	rest := strings.TrimSpace(strings.TrimPrefix(input, "/feedback"))
	if rest == "" {
		s.app.PrintSystem("💬  用法: /feedback <改进意见> [session_id]，例如 /feedback 检索范围应含美国同族")
		return
	}
	// 文本与可选 session_id 分离：最后一个以 _ 分隔的 token 形如会话 ID 时视为 ID。
	sid, text := "", rest
	if fields := strings.Fields(rest); len(fields) >= 2 {
		last := fields[len(fields)-1]
		if strings.Contains(last, "_") {
			sid, text = last, strings.TrimSpace(strings.TrimSuffix(rest, last))
		}
	}
	resolved, err := s.resolvePlantaskSessionID(sid)
	if err != nil {
		s.app.PrintSystem("💬  " + err.Error())
		return
	}
	res, err := s.callPlantaskTool("workflow_feedback", plantask.WorkflowFeedbackArgs{
		SessionID: resolved,
		Feedback:  text,
	})
	if err != nil {
		s.app.PrintSystem("💬  反馈注入失败: " + err.Error())
		return
	}
	s.app.PrintSystem(fmt.Sprintf("💬  %s", resultMessage(res)))
	// replan 完成（Executing）→ 联动续跑。
	s.resumeIfInterrupted()
}

// resultMessage 从工具结果中提取可读消息。
func resultMessage(res any) string {
	switch v := res.(type) {
	case string:
		var m struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		}
		if json.Unmarshal([]byte(v), &m) == nil && (m.Message != "" || m.Status != "") {
			if m.Message != "" {
				return m.Message
			}
			return "状态: " + m.Status
		}
		return v
	case error:
		return v.Error()
	default:
		return fmt.Sprintf("%v", res)
	}
}
