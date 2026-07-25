package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleEvidenceJudge_SubmitAndPoll(t *testing.T) {
	srv := &Server{evidenceTasks: make(map[string]*evidenceTask)}
	body := `{"source_uri":"patent:CN12345678A","snippet":"test snippet"}`
	req := httptest.NewRequest("POST", "/v1/evidence/judge", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvidenceJudge(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var submitResp struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	json.Unmarshal(w.Body.Bytes(), &submitResp)
	if submitResp.TaskID == "" {
		t.Fatal("expected task_id in response")
	}

	req2 := httptest.NewRequest("GET", "/v1/evidence/judge/"+submitResp.TaskID, nil)
	req2.SetPathValue("task_id", submitResp.TaskID)
	w2 := httptest.NewRecorder()
	srv.handleEvidenceJudgeStatus(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleEvidenceJudge_InvalidBody(t *testing.T) {
	srv := &Server{evidenceTasks: make(map[string]*evidenceTask)}
	req := httptest.NewRequest("POST", "/v1/evidence/judge", bytes.NewReader([]byte(`bad json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvidenceJudge(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEvidenceBurden(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest("GET", "/v1/evidence/burden/patent_infringement", nil)
	req.SetPathValue("scenario", "patent_infringement")
	w := httptest.NewRecorder()
	srv.handleEvidenceBurden(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["holder"] != "专利权人" {
		t.Errorf("expected 专利权人, got %v", result["holder"])
	}
}

func TestHandleEvidenceConflict(t *testing.T) {
	srv := &Server{}
	body := `{"claims":[{"claim_id":"A","supporting":["e1"],"contradicting":["e2"]}]}`
	req := httptest.NewRequest("POST", "/v1/evidence/conflict", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvidenceConflict(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if _, ok := result["conflicts"]; !ok {
		t.Error("missing conflicts in response")
	}
}
