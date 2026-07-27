package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/domains/evidence"
)

// evidenceTask is the internal representation of an async evidence judgment task.
type evidenceTask struct {
	ID      string
	Status  string // pending / running / completed / failed
	Result  *evidence.EvidenceJudgment
	Results []*evidence.EvidenceJudgment
	Err     error
	mu      sync.RWMutex
	doneCh  chan struct{}
}

func (t *evidenceTask) markCompletedSingle(result *evidence.EvidenceJudgment) {
	t.mu.Lock()
	t.Status = "completed"
	t.Result = result
	t.mu.Unlock()
	close(t.doneCh)
}

func (t *evidenceTask) markCompletedBatch(results []*evidence.EvidenceJudgment) {
	t.mu.Lock()
	t.Status = "completed"
	t.Results = results
	t.mu.Unlock()
	close(t.doneCh)
}

func (t *evidenceTask) markFailed(err error) {
	t.mu.Lock()
	t.Status = "failed"
	t.Err = err
	t.mu.Unlock()
	close(t.doneCh)
}

func (t *evidenceTask) snapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := map[string]any{"task_id": t.ID, "status": t.Status}
	if t.Err != nil {
		m["error"] = t.Err.Error()
	}
	if t.Result != nil {
		m["result"] = judgmentToAPIMap(t.Result)
	}
	if t.Results != nil {
		results := make([]map[string]any, len(t.Results))
		for i, r := range t.Results {
			results[i] = judgmentToAPIMap(r)
		}
		m["results"] = results
	}
	return m
}

var evidenceTaskCounter int

func newEvidenceTaskID() string {
	evidenceTaskCounter++
	return fmt.Sprintf("evj_%d", evidenceTaskCounter)
}

// handleEvidenceJudge handles POST /v1/evidence/judge
func (s *Server) handleEvidenceJudge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceURI string `json:"source_uri"`
		Snippet   string `json:"snippet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体 JSON 无效: " + err.Error()})
		return
	}
	if req.Snippet == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少必填字段 'snippet'"})
		return
	}

	task := &evidenceTask{ID: newEvidenceTaskID(), Status: "pending", doneCh: make(chan struct{})}
	s.evidenceTasksMu.Lock()
	s.evidenceTasks[task.ID] = task
	s.evidenceTasksMu.Unlock()

	go func() {
		task.mu.Lock()
		task.Status = "running"
		task.mu.Unlock()

		engine := evidence.NewEngine(nil)
		span := agentcore_evidence.EvidenceSpan{
			ID: task.ID, SourceURI: req.SourceURI, Snippet: req.Snippet,
		}
		judgment, err := engine.Judge(span)
		if err != nil {
			task.markFailed(err)
			slog.Error("evidence judge failed", "task_id", task.ID, "err", err)
			return
		}
		task.markCompletedSingle(judgment)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": task.ID, "status": "pending"})
}

// handleEvidenceJudgeStatus handles GET /v1/evidence/judge/{task_id}
func (s *Server) handleEvidenceJudgeStatus(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	s.evidenceTasksMu.Lock()
	task, ok := s.evidenceTasks[taskID]
	s.evidenceTasksMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "任务不存在: " + taskID})
		return
	}
	select {
	case <-task.doneCh:
	case <-r.Context().Done():
		return
	case <-time.After(30 * time.Second):
	}
	writeJSON(w, http.StatusOK, task.snapshot())
}

// handleEvidenceJudgeBatch handles POST /v1/evidence/judge/batch
func (s *Server) handleEvidenceJudgeBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			SourceURI string `json:"source_uri"`
			Snippet   string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效: " + err.Error()})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items 不能为空"})
		return
	}

	task := &evidenceTask{ID: "evb_" + newEvidenceTaskID()[4:], Status: "pending", doneCh: make(chan struct{})}
	s.evidenceTasksMu.Lock()
	s.evidenceTasks[task.ID] = task
	s.evidenceTasksMu.Unlock()

	go func() {
		task.mu.Lock()
		task.Status = "running"
		task.mu.Unlock()

		engine := evidence.NewEngine(nil)
		results := make([]*evidence.EvidenceJudgment, len(req.Items))
		for i, item := range req.Items {
			span := agentcore_evidence.EvidenceSpan{
				ID: fmt.Sprintf("%s_%d", task.ID, i), SourceURI: item.SourceURI, Snippet: item.Snippet,
			}
			judgment, err := engine.Judge(span)
			if err != nil {
				task.markFailed(fmt.Errorf("item %d: %w", i, err))
				return
			}
			results[i] = judgment
		}
		task.markCompletedBatch(results)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": task.ID, "status": "pending"})
}

// handleEvidenceBurden handles GET /v1/evidence/burden/{scenario}
func (s *Server) handleEvidenceBurden(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	if scenario == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 scenario"})
		return
	}
	result := evidence.DetermineBurden(evidence.BurdenScenario(scenario), nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"holder": result.BurdenHolder, "standard": result.Standard,
		"has_shifted": result.HasShifted, "shift_reason": result.ShiftReason,
		"reasoning": result.Reasoning,
	})
}

// handleEvidenceStandard handles GET /v1/evidence/standard/{standard}
func (s *Server) handleEvidenceStandard(w http.ResponseWriter, r *http.Request) {
	standardParam := r.PathValue("standard")
	if standardParam == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 standard"})
		return
	}
	result := evidence.AssessProofStandard(evidence.StandardOfProof(standardParam), 0, 0, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"met": result.Met, "standard": result.Standard, "confidence": result.Confidence,
		"reasoning": result.Reasoning,
	})
}

// handleEvidenceConflict handles POST /v1/evidence/conflict
func (s *Server) handleEvidenceConflict(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Claims []struct {
			ClaimID       string   `json:"claim_id"`
			Supporting    []string `json:"supporting"`
			Contradicting []string `json:"contradicting"`
		} `json:"claims"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效: " + err.Error()})
		return
	}
	cb := agentcore_evidence.NewClaimBinding()
	for _, c := range req.Claims {
		for _, sid := range c.Supporting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionSupporting, ClaimRefs: []string{c.ClaimID},
			})
		}
		for _, sid := range c.Contradicting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionContradicting, ClaimRefs: []string{c.ClaimID},
			})
		}
	}
	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()
	var out []map[string]any
	for _, c := range conflicts {
		out = append(out, map[string]any{"type": string(c.Type), "description": c.Description, "span_ids": c.SpanIDs})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": out})
}

// judgmentToAPIMap converts EvidenceJudgment to API response map.
func judgmentToAPIMap(j *evidence.EvidenceJudgment) map[string]any {
	m := map[string]any{"overall_score": j.OverallScore, "confidence": j.Confidence, "reasoning": j.Reasoning}
	if j.RelevanceJudgment != nil {
		m["relevance"] = map[string]any{"score": j.RelevanceJudgment.Score, "level": j.RelevanceJudgment.Level, "reasoning": j.RelevanceJudgment.Reasoning}
	}
	if j.LegalityJudgment != nil {
		m["legality"] = map[string]any{"score": j.LegalityJudgment.Score, "level": j.LegalityJudgment.Level, "reasoning": j.LegalityJudgment.Reasoning}
	}
	if j.AuthenticityJudgment != nil {
		m["authenticity"] = map[string]any{"score": j.AuthenticityJudgment.Score, "level": j.AuthenticityJudgment.Level, "reasoning": j.AuthenticityJudgment.Reasoning}
	}
	if j.TypeSpecificJudgment != nil {
		ts := j.TypeSpecificJudgment
		tsMap := map[string]any{"evidence_type": string(ts.EvidenceType)}
		if ts.PlatformCredibility != nil {
			tsMap["platform_credibility"] = string(*ts.PlatformCredibility)
		}
		if ts.ContentIntegrity != "" {
			tsMap["content_integrity"] = string(ts.ContentIntegrity)
		}
		if ts.PublicIntent != "" {
			tsMap["public_intent"] = string(ts.PublicIntent)
		}
		if ts.FourElementsCheck != nil {
			fec := ts.FourElementsCheck
			tsMap["four_elements_check"] = map[string]any{
				"time":          map[string]any{"met": fec.TimeElement.Met, "score": fec.TimeElement.Score, "detail": fec.TimeElement.Detail},
				"place":         map[string]any{"met": fec.PlaceElement.Met, "score": fec.PlaceElement.Score, "detail": fec.PlaceElement.Detail},
				"method":        map[string]any{"met": fec.MethodElement.Met, "score": fec.MethodElement.Score, "detail": fec.MethodElement.Detail},
				"accessibility": map[string]any{"met": fec.Accessibility.Met, "score": fec.Accessibility.Score, "detail": fec.Accessibility.Detail},
			}
		}
		m["type_specific"] = tsMap
	}
	issues := make([]map[string]string, 0, len(j.FlaggedIssues))
	for _, issue := range j.FlaggedIssues {
		issues = append(issues, map[string]string{"type": issue.Type, "description": issue.Description, "severity": issue.Severity})
	}
	if len(issues) > 0 {
		m["issues"] = issues
	}
	return m
}
