package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/graph"
)

// ──────────────────────────────────────────────
// Disclosure API
// ──────────────────────────────────────────────

// DisclosureAnalyzeRequest 是提交交底书分析的请求。
type DisclosureAnalyzeRequest struct {
	Text string `json:"text"`
}

// DisclosureAnalyzeResponse 是提交分析的响应。
type DisclosureAnalyzeResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// DisclosureTaskStatus 是任务状态的轮询响应。
type DisclosureTaskStatus struct {
	TaskID   string                     `json:"task_id"`
	Status   string                     `json:"status"`
	Progress *DisclosureProgress        `json:"progress,omitempty"`
	Result   *disclosure.AnalysisReport `json:"result,omitempty"`
	Error    string                     `json:"error,omitempty"`
}

// DisclosureProgress 是 Pregel 图的执行进度。
type DisclosureProgress struct {
	CurrentNode string   `json:"current_node"`
	NodesDone   []string `json:"nodes_done"`
}

// ──────────────────────────────────────────────
// 任务管理（内嵌于 Server，非包级全局变量）
// ──────────────────────────────────────────────

const disclosureCleanupInterval = 10 * time.Minute
const disclosureTaskRetention = 30 * time.Minute
const disclosureExecTimeout = 10 * time.Minute

type disclosureTask struct {
	ID        string
	Text      string
	Status    string
	Progress  *DisclosureProgress
	Result    *disclosure.AnalysisReport
	Err       error
	CreatedAt time.Time
	DoneAt    time.Time

	mu     sync.RWMutex
	doneCh chan struct{}
}

type disclosureTaskManager struct {
	mu      sync.RWMutex
	tasks   map[string]*disclosureTask
	counter atomic.Int64
	stopCh  chan struct{}
}

// newDisclosureTaskManager 创建一个新的任务管理器并启动后台清理 goroutine。
func newDisclosureTaskManager() *disclosureTaskManager {
	m := &disclosureTaskManager{
		tasks:  make(map[string]*disclosureTask),
		stopCh: make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// close 停止后台清理 goroutine。
func (m *disclosureTaskManager) close() {
	close(m.stopCh)
}

func (m *disclosureTaskManager) submitTask(provider agentcore.Provider, text string) string {
	seq := m.counter.Add(1)
	id := fmt.Sprintf("disclosure-%s-%04d", formatTimestamp(time.Now()), seq)
	task := &disclosureTask{
		ID:        id,
		Text:      text,
		Status:    "pending",
		Progress:  &DisclosureProgress{},
		CreatedAt: time.Now(),
		doneCh:    make(chan struct{}),
	}

	m.mu.Lock()
	m.tasks[id] = task
	m.mu.Unlock()

	// 使用带超时的 context，避免图执行无限挂起
	ctx, cancel := context.WithTimeout(context.Background(), disclosureExecTimeout)
	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("disclosure: task panicked",
					"task_id", task.ID,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				task.mu.Lock()
				task.Status = "failed"
				task.Err = fmt.Errorf("task panicked: %v", r)
				task.DoneAt = time.Now()
				task.mu.Unlock()
				close(task.doneCh)
			}
		}()
		m.executeTask(ctx, task, provider)
	}()
	return id
}

func (m *disclosureTaskManager) executeTask(ctx context.Context, task *disclosureTask, provider agentcore.Provider) {
	task.mu.Lock()
	task.Status = "running"
	task.Progress = &DisclosureProgress{CurrentNode: "preprocess"}
	task.mu.Unlock()

	compiled, err := disclosure.BuildDisclosureAnalysisGraph(provider)
	if err != nil {
		task.mu.Lock()
		task.Status = "failed"
		task.Err = fmt.Errorf("构建分析图失败: %w", err)
		task.DoneAt = time.Now()
		task.mu.Unlock()
		close(task.doneCh)
		return
	}

	state := graph.PregelState{
		disclosure.StateKeyInput: task.Text,
	}
	// 使用传入的 ctx（带超时），而非 context.Background()
	state, runErr := compiled.Run(ctx, state)

	task.mu.Lock()
	if runErr != nil {
		task.Status = "failed"
		task.Err = runErr
	} else {
		report := disclosure.ExtractReportFromState(state)
		if report != nil {
			task.Status = "completed"
			task.Result = report
		} else {
			task.Status = "completed"
			if output := state.GetString(disclosure.StateKeyOutput); output != "" {
				task.Result = &disclosure.AnalysisReport{
					ReportText:  output,
					GeneratedAt: time.Now(),
				}
			}
		}
	}
	task.Progress.NodesDone = append(task.Progress.NodesDone, "done")
	task.DoneAt = time.Now()
	task.mu.Unlock()
	close(task.doneCh)
}

func (m *disclosureTaskManager) getTask(id string) (*disclosureTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return task, ok
}

// cleanupLoop 定期清理已过期的任务，防止内存泄漏。
func (m *disclosureTaskManager) cleanupLoop() {
	ticker := time.NewTicker(disclosureCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanup(disclosureTaskRetention)
		case <-m.stopCh:
			return
		}
	}
}

func (m *disclosureTaskManager) cleanup(olderThan time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, task := range m.tasks {
		if !task.DoneAt.IsZero() && now.Sub(task.DoneAt) > olderThan {
			delete(m.tasks, id)
		}
	}
}

// ──────────────────────────────────────────────
// HTTP 处理器（*Server 方法，内嵌 manager）
// ──────────────────────────────────────────────

// initDisclosureManager 确保 disclosure 管理器已初始化。
func (s *Server) initDisclosureManager() *disclosureTaskManager {
	s.mu.RLock()
	dm := s.disclosure
	s.mu.RUnlock()
	if dm != nil {
		return dm
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disclosure == nil {
		s.disclosure = newDisclosureTaskManager()
	}
	return s.disclosure
}

// handleDisclosureAnalyze 接收交底书文本，启动异步分析。
func (s *Server) handleDisclosureAnalyze(w http.ResponseWriter, r *http.Request) {
	var req DisclosureAnalyzeRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, DisclosureAnalyzeResponse{Status: "failed"})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, DisclosureAnalyzeResponse{Status: "failed"})
		return
	}

	cfg := s.snapshotConfig()
	provider := cfg.Provider
	if provider == nil {
		writeJSON(w, http.StatusInternalServerError, DisclosureAnalyzeResponse{Status: "failed"})
		return
	}

	dm := s.initDisclosureManager()
	taskID := dm.submitTask(provider, req.Text)
	writeJSON(w, http.StatusAccepted, DisclosureAnalyzeResponse{
		TaskID: taskID,
		Status: "pending",
	})
}

// handleDisclosureStatus 返回指定任务的状态/结果。
func (s *Server) handleDisclosureStatus(w http.ResponseWriter, r *http.Request) {
	dm := s.initDisclosureManager()
	taskID := r.PathValue("task_id")
	task, ok := dm.getTask(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	task.mu.RLock()
	resp := DisclosureTaskStatus{
		TaskID:   task.ID,
		Status:   task.Status,
		Progress: task.Progress,
		Result:   task.Result,
	}
	if task.Err != nil {
		resp.Error = task.Err.Error()
	}
	task.mu.RUnlock()

	writeJSON(w, http.StatusOK, resp)
}

// handleDisclosureStream 使用 SSE 实时推送执行进度。
func (s *Server) handleDisclosureStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	dm := s.initDisclosureManager()
	taskID := r.PathValue("task_id")
	task, ok := dm.getTask(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var writeMu sync.Mutex

	writeSSE := func(format string, args ...any) {
		writeMu.Lock()
		fmt.Fprintf(w, format, args...)
		flusher.Flush()
		writeMu.Unlock()
	}

	writeSSE(": connected\n\n")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-task.doneCh:
			task.mu.RLock()
			resp := DisclosureTaskStatus{
				TaskID: task.ID,
				Status: task.Status,
				Result: task.Result,
			}
			if task.Err != nil {
				resp.Error = task.Err.Error()
			}
			task.mu.RUnlock()

			payload, _ := json.Marshal(resp)
			writeSSE("event: done\ndata: %s\n\n", payload)
			return
		case <-ticker.C:
			task.mu.RLock()
			resp := DisclosureTaskStatus{
				TaskID:   task.ID,
				Status:   task.Status,
				Progress: task.Progress,
			}
			task.mu.RUnlock()

			payload, _ := json.Marshal(resp)
			writeSSE("event: progress\ndata: %s\n\n", payload)
		}
	}
}

// ──────────────────────────────────────────────
// 工具函数
// ──────────────────────────────────────────────

func formatTimestamp(t time.Time) string {
	return t.Format("20060102-150405")
}
