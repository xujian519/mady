package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// healthResponse 是 /health 和 /ready 端点的标准响应结构。
type healthResponse struct {
	Status string         `json:"status"`
	Time   string         `json:"time"`
	Checks map[string]any `json:"checks,omitempty"`
}

// handleReady 检查服务器及其依赖是否就绪。
//
//	GET /ready → {"status":"ok","time":"2026-07-21T...","checks":{"agents":4,"disclosure_tasks":0}}
//
// 当前检查项：
//   - agents: Agent 池中可用（refs==0 即空闲）的 entry 数量
//   - disclosure_tasks: 任务管理器中正在运行的任务数量
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]any)

	// Agent 池健康检查：统计空闲 entry
	s.poolMu.Lock()
	idleAgents := 0
	totalAgents := 0
	s.agentPool.Range(func(_, value any) bool {
		totalAgents++
		entry := value.(*poolEntry)
		if entry.refs == 0 {
			idleAgents++
		}
		return true
	})
	s.poolMu.Unlock()
	checks["agents"] = totalAgents
	checks["agents_idle"] = idleAgents

	// Disclosure 任务管理器健康检查：统计运行中的任务
	if dm := s.disclosure.Load(); dm != nil {
		runningTasks := 0
		for _, pair := range dm.tasks.Copy() {
			pair.mu.RLock()
			if pair.Status == "running" || pair.Status == "pending" {
				runningTasks++
			}
			pair.mu.RUnlock()
		}
		checks["disclosure_tasks"] = runningTasks
	} else {
		checks["disclosure_tasks"] = 0
	}

	// 将 checks map 序列化为 JSON 并作为独立字段输出
	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
		Checks: checks,
	})
}

// handleHealthFast 返回服务器存活状态（性能优化的 /health 快捷路径）。
func handleHealthFast(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(healthResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}
