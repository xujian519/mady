package main

import (
	"context"
	"fmt"
	"log"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/agentadapter"
)

// getCurrentAgent 返回当前 Agent 实例，线程安全。
func (s *tuiSession) getCurrentAgent() *agentcore.Agent {
	s.agentMu.RLock()
	defer s.agentMu.RUnlock()
	return s.currentAgent
}

// agentStatus 返回当前 Agent、初始化状态和错误信息，线程安全。
func (s *tuiSession) agentStatus() (*agentcore.Agent, bool, string) {
	s.agentMu.RLock()
	defer s.agentMu.RUnlock()
	return s.currentAgent, s.agentInitInFlight, s.agentInitErr
}

// markAgentInitializing 标记 Agent 正在初始化。shuttingDown 时不修改状态。
func (s *tuiSession) markAgentInitializing() {
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	if s.shuttingDown {
		return
	}
	s.agentInitInFlight = true
	s.agentInitErr = ""
}

// setAgentInitError 记录初始化错误并清除 in-flight 标记。
// 返回 false 表示 session 已在关闭过程中，调用方应停止后续操作。
func (s *tuiSession) setAgentInitError(err error) bool {
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	s.agentInitInFlight = false
	if s.shuttingDown {
		return false
	}
	s.agentInitErr = err.Error()
	return true
}

// swapCurrentAgent 原子替换当前 Agent 实例。返回旧实例和成功标志。
// 返回 false 表示 session 已在关闭过程中，调用方应清理新 Agent。
func (s *tuiSession) swapCurrentAgent(agent *agentcore.Agent) (*agentcore.Agent, bool) {
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	s.agentInitInFlight = false
	if s.shuttingDown {
		return nil, false
	}
	prev := s.currentAgent
	s.currentAgent = agent
	s.agentInitErr = ""
	return prev, true
}

// shutdownAgent 标记关闭并返回当前 Agent。调用方负责关闭返回的 Agent。
func (s *tuiSession) shutdownAgent() *agentcore.Agent {
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	s.shuttingDown = true
	s.agentInitInFlight = false
	prev := s.currentAgent
	s.currentAgent = nil
	return prev
}

// agentUnavailableMessage 返回 Agent 不可用时的用户提示文案，为空表示可用。
func (s *tuiSession) agentUnavailableMessage() string {
	agent, initializing, initErr := s.agentStatus()
	if agent != nil {
		return ""
	}
	if initializing {
		return "Agent 正在初始化，请稍候片刻再发送消息…"
	}
	if initErr != "" {
		return "Agent 初始化失败，请查看日志后重试当前操作。"
	}
	return "Agent 尚未就绪，请稍候…"
}

// initializeAgentAsync 在后台 goroutine 中初始化 Agent，不阻塞 TUI 启动。
func (s *tuiSession) initializeAgentAsync() {
	s.markAgentInitializing()
	go func() {
		s.runMu.Lock()
		defer s.runMu.Unlock()

		var newAgent *agentcore.Agent
		defer func() {
			if r := recover(); r != nil {
				if newAgent != nil {
					newAgent.Close()
				}
				err := fmt.Errorf("agent initialization failed: %v", r)
				log.Printf("[mady] %v", err)
				if s.setAgentInitError(err) {
					s.app.PrintSystem("Agent 初始化失败，请查看日志后重试当前操作。")
				}
			}
		}()

		newAgent = agentcore.New(s.buildAgentConfig())
		prev, ok := s.swapCurrentAgent(newAgent)
		if !ok {
			newAgent.Close()
			return
		}
		if prev != nil {
			prev.Close()
		}
		agentadapter.BindAgent(s.app, newAgent)
		s.app.PrintSystem("Agent 就绪，可以开始对话")
	}()
}

// rebuildAgent recreates the current agent from the latest config and rebinds it to the UI.
func (s *tuiSession) rebuildAgent() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	newAgent := agentcore.New(s.buildAgentConfig())
	prev, ok := s.swapCurrentAgent(newAgent)
	if !ok {
		newAgent.Close()
		return
	}
	if prev != nil {
		prev.Close()
	}
	agentadapter.BindAgent(s.app, newAgent)
}

// submitInput sends user input to the current agent asynchronously.
// The agent runs in a separate goroutine to avoid blocking the TUI event loop.
//
// 启动序列中 app.Start() 先于 Agent 初始化完成（见 tui.go），因此需要先检查
// Agent 状态；真正执行时再在 runMu 临界区内重读当前 Agent，避免运行中的
// rebuild/close 让 goroutine 持有已失效实例。
func (s *tuiSession) submitInput(input string) {
	if msg := s.agentUnavailableMessage(); msg != "" {
		s.app.PrintSystem(msg)
		return
	}
	store := s.agentStore
	threadID := s.currentThreadID
	go func() {
		s.runMu.Lock()
		defer s.runMu.Unlock()
		agent := s.getCurrentAgent()
		if agent == nil {
			s.app.PrintSystem(s.agentUnavailableMessage())
			return
		}

		runCtx, cancel := context.WithCancel(s.ctx)
		s.cancelMu.Lock()
		s.runCancel = cancel
		s.cancelMu.Unlock()
		defer func() {
			s.cancelMu.Lock()
			s.runCancel = nil
			s.cancelMu.Unlock()
		}()

		if _, err := agent.Run(runCtx, input); err != nil {
			log.Printf("[mady] agent run failed: %v", err)
			return
		}
		if store == nil {
			return
		}
		if err := agent.SaveState(context.Background(), threadID); err != nil {
			log.Printf("[mady] save state: %v", err)
		}
	}()
}

// resumeIfInterrupted continues the agent from an interrupt point (e.g. the
// disclosure review_gate) by calling agent.Resume, which preserves the
// interrupted runLoop's state. Returns true when a resume was initiated.
//
// This is the hard-interrupt recovery path, distinct from submitInput: when a
// Pregel tool node returns InterruptError, the agent loop exits and only
// Resume() can pick it up — submitInput would instead start a fresh turn and
// lose the in-flight tool context. Callers (/approve) should try this first
// and fall back to submitInput only when the agent is not interrupted (the
// ApprovalGate keyword-triggered soft-interrupt case).
func (s *tuiSession) resumeIfInterrupted() bool {
	agent := s.getCurrentAgent()
	if agent == nil || agent.Interrupted() == nil {
		return false
	}
	store := s.agentStore
	threadID := s.currentThreadID
	go func() {
		s.runMu.Lock()
		defer s.runMu.Unlock()
		agent := s.getCurrentAgent()
		if agent == nil || agent.Interrupted() == nil {
			return
		}

		runCtx, cancel := context.WithCancel(s.ctx)
		s.cancelMu.Lock()
		s.runCancel = cancel
		s.cancelMu.Unlock()
		defer func() {
			s.cancelMu.Lock()
			s.runCancel = nil
			s.cancelMu.Unlock()
		}()

		if _, err := agent.Resume(runCtx); err != nil {
			log.Printf("[mady] agent resume failed: %v", err)
			return
		}
		if store == nil {
			return
		}
		if err := agent.SaveState(context.Background(), threadID); err != nil {
			log.Printf("[mady] save state: %v", err)
		}
	}()
	return true
}
