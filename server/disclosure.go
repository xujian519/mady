package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/inventiveness"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/csync"
	"github.com/xujian519/mady/retrieval/domain"
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
	// ReviewDecision 记录人工复核结论（adopted/modified/rejected），
	// 仅在复核端点受理后非空。
	ReviewDecision string `json:"review_decision,omitempty"`
	// Inventiveness 承载创造性三步法评估结果（异步完成后非空）。
	// 由 InventivenessTrigger 在 disclosure 管线结束后自动触发并填充。
	Inventiveness *inventiveness.InventivenessResult `json:"inventiveness,omitempty"`
}

// DisclosureReviewRequest 是人工复核结论的提交请求。
// Decision 取值为 adopted（采纳）/ modified（修改后采纳）/ rejected（拒绝），
// 与 domains.ApprovalDecision 对齐；modified 时 ModifiedOutput 承载改后内容。
type DisclosureReviewRequest struct {
	Decision       string `json:"decision"`
	ModifiedOutput string `json:"modified_output,omitempty"`
	Feedback       string `json:"feedback,omitempty"`
	CaseID         string `json:"case_id,omitempty"`
}

// DisclosureReviewResponse 是复核受理的响应。
type DisclosureReviewResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
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
	// ReviewDecision 记录人工复核结论（adopted/modified/rejected），
	// 由 handleDisclosureReview 在留痕成功后写入。
	ReviewDecision string

	mu     sync.RWMutex
	doneCh chan struct{}
}

type disclosureTaskManager struct {
	tasks    *csync.Map[string, *disclosureTask]
	counter  atomic.Int64
	stopCh   chan struct{}
	eventBus iface.EventBus
}

func cloneDisclosureProgress(src *DisclosureProgress) *DisclosureProgress {
	if src == nil {
		return nil
	}
	dst := &DisclosureProgress{
		CurrentNode: src.CurrentNode,
	}
	if len(src.NodesDone) > 0 {
		dst.NodesDone = append([]string(nil), src.NodesDone...)
	}
	return dst
}

func cloneAnalysisReport(src *disclosure.AnalysisReport) *disclosure.AnalysisReport {
	if src == nil {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		cp := *src
		return &cp
	}
	var dst disclosure.AnalysisReport
	if err := json.Unmarshal(data, &dst); err != nil {
		cp := *src
		return &cp
	}
	return &dst
}

// newDisclosureTaskManager 创建一个新的任务管理器并启动后台清理 goroutine。
// eventBus 为可选参数，非 nil 时任务完成后发射 DisclosureCompletedEvent。
func newDisclosureTaskManager(eventBus iface.EventBus) *disclosureTaskManager {
	m := &disclosureTaskManager{
		tasks:    csync.NewMap[string, *disclosureTask](),
		stopCh:   make(chan struct{}),
		eventBus: eventBus,
	}
	go m.cleanupLoop()
	return m
}

// close 停止后台清理 goroutine。
func (m *disclosureTaskManager) close() {
	close(m.stopCh)
}

func (m *disclosureTaskManager) submitTask(provider agentcore.Provider, retriever domain.DomainRetriever, text string) string {
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

	m.tasks.Set(id, task)

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
		m.executeTask(ctx, task, provider, retriever)
	}()
	return id
}

func (m *disclosureTaskManager) executeTask(ctx context.Context, task *disclosureTask, provider agentcore.Provider, retriever domain.DomainRetriever) {
	task.mu.Lock()
	task.Status = "running"
	task.Progress = &DisclosureProgress{CurrentNode: "preprocess"}
	task.mu.Unlock()

	var opts []disclosure.GraphOption
	if retriever != nil {
		opts = append(opts, disclosure.WithRetriever(retriever))
	}
	compiled, err := disclosure.BuildDisclosureAnalysisGraphWithOpts(provider, opts...)
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

	// 从 PregelState 提取证据片段和覆盖度，供下游消费者（如 InventivenessTrigger）。
	var evidenceChunks []disclosure.EvidenceChunk
	var evidenceCoverage string
	if raw, ok := state[disclosure.StateKeyEvidence]; ok {
		if chunks, ok := raw.([]disclosure.EvidenceChunk); ok {
			evidenceChunks = chunks
		}
	}
	if cov, ok := state[disclosure.StateKeyEvidenceCoverage].(string); ok {
		evidenceCoverage = cov
	}

	task.mu.Lock()
	switch {
	case runErr != nil && agentcore.IsInterrupt(runErr):
		// review_gate 在报告生成之后才中断（节点顺序：...→generate_report→
		// review_gate），所以 state 里已有完整 AnalysisReport。提取出来标记为
		// "awaiting_review"，让客户端拿到报告自行人工复核——而非当作失败。
		// Server 是异步任务模型，无交互式 Resume，故不保留中断态等待恢复。
		report := disclosure.ExtractReportFromState(state)
		if report != nil {
			task.Status = "awaiting_review"
			task.Result = report
		} else {
			task.Status = "failed"
			task.Err = fmt.Errorf("interrupted but no report in state: %w", runErr)
		}
	case runErr != nil:
		task.Status = "failed"
		task.Err = runErr
	default:
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

	// 在锁内构建事件（eventBus.Emit 是非阻塞的），消除时序间隙。
	if m.eventBus != nil {
		ev := DisclosureCompletedEvent{
			at:               task.DoneAt,
			TaskID:           task.ID,
			Report:           task.Result,
			EvidenceChunks:   evidenceChunks,
			EvidenceCoverage: evidenceCoverage,
		}
		if task.Err != nil {
			ev.Err = task.Err.Error()
		}
		m.eventBus.Emit(iface.NewEvent(iface.EventType(EventDisclosureCompleted), ev))
	}
	task.mu.Unlock()

	close(task.doneCh)
}

func (m *disclosureTaskManager) getTask(id string) (*disclosureTask, bool) {
	return m.tasks.Get(id)
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
	now := time.Now()
	tasks := m.tasks.Copy()
	for id, task := range tasks {
		if !task.DoneAt.IsZero() && now.Sub(task.DoneAt) > olderThan {
			m.tasks.Del(id)
		}
	}
}

// ──────────────────────────────────────────────
// HTTP 处理器（*Server 方法，内嵌 manager）
// ──────────────────────────────────────────────

// initDisclosureManager 确保 disclosure 管理器已初始化。
func (s *Server) initDisclosureManager() *disclosureTaskManager {
	dm := s.disclosure.Load()
	if dm != nil {
		return dm
	}
	s.discMu.Lock()
	defer s.discMu.Unlock()
	dm = s.disclosure.Load()
	if dm == nil {
		dm = newDisclosureTaskManager(s.eventBus)
		s.disclosure.Store(dm)
	}
	return dm
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
	retriever := s.getDisclosureRetriever()
	taskID := dm.submitTask(provider, retriever, req.Text)
	writeJSON(w, http.StatusAccepted, DisclosureAnalyzeResponse{
		TaskID: taskID,
		Status: "pending",
	})
}

// handleDisclosureStatus 返回指定任务的状态/结果（含创造性分析结果）。
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
		TaskID:         task.ID,
		Status:         task.Status,
		Progress:       cloneDisclosureProgress(task.Progress),
		Result:         cloneAnalysisReport(task.Result),
		ReviewDecision: task.ReviewDecision,
	}
	if task.Err != nil {
		resp.Error = task.Err.Error()
	}
	task.mu.RUnlock()

	// 附加创造性分析结果（异步完成）。
	if inv := s.GetInventivenessResult(taskID); inv != nil {
		resp.Inventiveness = inv
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDisclosureReview 受理 awaiting_review 任务的人工复核结论，并将决策
// 留痕到 ApprovalStore（P3 专家盲测数据收集链路的关键触点）。
//
// Server 是异步任务模型，review_gate 中断后图已终止、无 Resume 路径，因此
// 人工复核通过此端点显式提交：复核结论写入 SQLite（与 TUI /approve /reject
// 同一 RecordDecision 模式），报告标记 ReviewedByHuman。
func (s *Server) handleDisclosureReview(w http.ResponseWriter, r *http.Request) {
	var req DisclosureReviewRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	decision := domains.ApprovalDecision(req.Decision)
	switch decision {
	case domains.DecisionAdopted, domains.DecisionModified, domains.DecisionRejected:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision must be one of adopted/modified/rejected"})
		return
	}

	store := s.getApprovalStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval store not configured"})
		return
	}

	dm := s.initDisclosureManager()
	taskID := r.PathValue("task_id")
	task, ok := dm.getTask(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	// 在任务锁内校验状态、留痕并更新，避免并发重复复核产生脏记录。
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.Status != "awaiting_review" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("task is %q, not awaiting_review", task.Status),
		})
		return
	}

	// 留痕的 OriginalOutput 取被审报告正文；缺失时退化为整份报告的 JSON。
	original := ""
	if task.Result != nil {
		original = task.Result.ReportText
		if original == "" {
			if data, err := json.Marshal(task.Result); err == nil {
				original = string(data)
			}
		}
	}
	if err := domains.RecordApprovalDecision(
		r.Context(), store,
		taskID, req.CaseID, "disclosure_review", original,
		decision, req.ModifiedOutput, req.Feedback,
	); err != nil {
		slog.Default().Error("disclosure: record review decision failed",
			"task_id", taskID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record review decision"})
		return
	}

	task.Status = "reviewed"
	task.ReviewDecision = req.Decision
	if task.Result != nil {
		// 三种结论都算"已经人工复核"——rejected 同样是复核行为的结果。
		task.Result.ReviewedByHuman = true
	}
	writeJSON(w, http.StatusOK, DisclosureReviewResponse{TaskID: taskID, Status: "reviewed"})
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
				TaskID:         task.ID,
				Status:         task.Status,
				Progress:       task.Progress,
				Result:         task.Result,
				ReviewDecision: task.ReviewDecision,
			}
			if task.Err != nil {
				resp.Error = task.Err.Error()
			}
			task.mu.RUnlock()

			// SSE 流同样附加创造性分析结果（与 REST 状态端点一致）。
			if inv := s.GetInventivenessResult(taskID); inv != nil {
				resp.Inventiveness = inv
			}

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
