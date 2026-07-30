package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
)

func (s *tuiSession) recordApprovalDecision(decision domains.ApprovalDecision, modifiedOutput, feedback string) {
	if s.approvalGate == nil {
		return
	}
	caseID := ""
	if s.currentProject != nil {
		caseID = s.currentProject.ProjectID
	}
	triggerKeyword := "review"
	originalOutput := ""
	if agent := s.getCurrentAgent(); agent != nil {
		if ir := agent.Interrupted(); ir != nil {
			if gate, ok := ir.Data["gate"].(string); ok && gate != "" {
				triggerKeyword = gate
			}
			originalOutput = ir.Reason
			if len(ir.Data) > 0 {
				if data, err := json.Marshal(ir.Data); err == nil {
					originalOutput += "\n" + string(data)
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.approvalGate.RecordDecision(ctx, s.currentThreadID, caseID, triggerKeyword, originalOutput, decision, modifiedOutput, feedback); err != nil {
		log.Printf("approval: record decision: %v", err)
	}
}

// persistSlashMessages 将斜杠命令的用户输入和 Pregel 输出写入 AgentStore JSONL，
// 确保分析结果不因 TUI 重启而丢失。
//
// 若持久化未启用（agentStore == nil），静默跳过；错误仅记录日志，不阻塞显示。
func (s *tuiSession) persistSlashMessages(inputLine, outputText string) {
	if s.agentStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// 加载当前线程已有消息准备追加。
	existing, err := s.agentStore.Load(ctx, s.currentThreadID)
	if err != nil {
		// Load 在首次使用空线程时返回 StatusIdle + 空 Messages，不会报错。
		log.Printf("[mady] load thread for slash persistence: %v", err)
		return
	}

	msgs := existing.Messages
	msgs = append(msgs,
		agentcore.Message{Role: agentcore.RoleUser, Content: inputLine},
		agentcore.Message{Role: agentcore.RoleAssistant, Content: outputText},
	)

	snap := agentcore.StateSnapshot{
		Status:     agentcore.StatusFinished,
		Messages:   msgs,
		Turn:       existing.Turn + 1,
		TotalUsage: existing.TotalUsage,
	}

	if err := s.agentStore.Save(ctx, s.currentThreadID, snap); err != nil {
		log.Printf("[mady] persist slash result: %v", err)
	}
}
