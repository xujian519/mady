package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains"
)

type disclosureSmokeProvider struct {
	mu        sync.Mutex
	responses map[string]string
	fallback  string
}

func (p *disclosureSmokeProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var content strings.Builder
	for _, msg := range req.Messages {
		content.WriteString(msg.Content)
	}
	joined := content.String()
	for key, resp := range p.responses {
		if strings.Contains(joined, key) {
			return &agentcore.ProviderResponse{Content: resp}, nil
		}
	}
	return &agentcore.ProviderResponse{Content: p.fallback}, nil
}

func (p *disclosureSmokeProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Content: resp.Content, Done: true}
	close(ch)
	return ch, nil
}

func newDisclosureSmokeProvider() agentcore.Provider {
	return &disclosureSmokeProvider{
		responses: map[string]string{
			"提取所有需要解决的技术问题": mustSmokeJSON(map[string]any{
				"problems": []map[string]any{
					{"id": "P1", "text": "现有运动检测方案功耗较高", "confidence": 0.9},
				},
			}),
			"提取所有技术特征": mustSmokeJSON(map[string]any{
				"features": []map[string]any{
					{
						"id":               "F1",
						"description":      "自适应采样率算法",
						"category":         "method",
						"function":         "动态调整采样频率",
						"prior_art_status": "partial",
						"importance":       "high",
						"confidence":       0.88,
						"solves":           []string{"P1"},
					},
					{
						"id":               "F2",
						"description":      "低功耗休眠模式",
						"category":         "method",
						"function":         "降低待机功耗",
						"prior_art_status": "known",
						"importance":       "medium",
						"confidence":       0.82,
						"solves":           []string{"P1"},
					},
				},
			}),
			"提取所有有益技术效果": mustSmokeJSON(map[string]any{
				"effects": []map[string]any{
					{"id": "E1", "text": "待机功耗降低 70%", "from": []string{"F1", "F2"}, "confidence": 0.86},
				},
			}),
			"你是一名资深专利审查员，负责对技术交底书进行新颖性预评估。": mustSmokeJSON(map[string]any{
				"feature_assessments": []map[string]any{
					{
						"feature_id":         "F1",
						"novelty_status":     "likely_novel",
						"confidence":         "medium",
						"reasoning":          "现有技术中未见相同的动态采样策略组合。",
						"similar_prior_art":  "常规固定频率采样方案",
						"cited_evidence_ids": []string{},
					},
					{
						"feature_id":         "F2",
						"novelty_status":     "possibly_known",
						"confidence":         "medium",
						"reasoning":          "低功耗休眠在可穿戴设备中较常见。",
						"similar_prior_art":  "传统节能待机模式",
						"cited_evidence_ids": []string{},
					},
				},
				"overall_conclusion": "核心改进点集中在自适应采样率策略，建议继续检索验证。",
				"overall_confidence": "medium",
				"search_advice":      []string{"自适应采样", "低功耗运动检测"},
			}),
			"生成技术交底书分析报告": `## 技术交底书分析报告

### 一、文档概况
- 标题：一种低功耗运动检测传感器

### 二、技术问题
- 现有运动检测方案功耗较高

### 三、技术特征
- 自适应采样率算法
- 低功耗休眠模式

### 四、技术效果
- 待机功耗降低 70%

### 五、结论
建议进入人工复核。

### 免责声明
本报告由 AI 辅助生成，不构成正式法律意见。`,
		},
		fallback: `{}`,
	}
}

func mustSmokeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func waitForDisclosureTask(t *testing.T, srv *Server, taskID string, want string) DisclosureTaskStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/v1/disclosure/analyze/"+taskID, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d: %s", rec.Code, rec.Body.String())
		}
		var status DisclosureTaskStatus
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if status.Status == want {
			return status
		}
		if status.Status == "failed" {
			t.Fatalf("task failed unexpectedly: %s", status.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for disclosure task to reach %q", want)
	return DisclosureTaskStatus{}
}

func TestDisclosureHappyPathSmoke(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "smoke",
			Model:    "stub",
			Provider: newDisclosureSmokeProvider(),
		},
	})
	defer srv.Close()
	store := domains.NewMemoryApprovalStore()
	srv.SetApprovalStore(store)

	analyzeReq := DisclosureAnalyzeRequest{
		Text: `发明名称：一种低功耗运动检测传感器

背景技术
现有运动检测方案功耗高，影响可穿戴设备续航。

发明内容
本发明通过自适应采样率算法和低功耗休眠模式降低待机功耗。`,
	}
	payload, err := json.Marshal(analyzeReq)
	if err != nil {
		t.Fatalf("marshal analyze request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/disclosure/analyze", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("analyze: got %d: %s", rec.Code, rec.Body.String())
	}
	var accepted DisclosureAnalyzeResponse
	if decodeErr := json.NewDecoder(rec.Body).Decode(&accepted); decodeErr != nil {
		t.Fatalf("decode analyze response: %v", decodeErr)
	}
	if accepted.TaskID == "" {
		t.Fatal("expected non-empty task id")
	}

	status := waitForDisclosureTask(t, srv, accepted.TaskID, "awaiting_review")
	if status.Result == nil {
		t.Fatal("expected analysis report before review")
	}
	if status.Result.ReportText == "" {
		t.Fatal("expected non-empty report text")
	}
	if status.Result.ReviewedByHuman {
		t.Fatal("report should not be marked reviewed before review endpoint")
	}

	reviewRec := doReviewRequest(t, srv, accepted.TaskID, DisclosureReviewRequest{
		Decision:       "adopted",
		Feedback:       "结构完整，可进入下一步处理",
		ModifiedOutput: "",
		CaseID:         "smoke-case-1",
	})
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("review: got %d: %s", reviewRec.Code, reviewRec.Body.String())
	}

	reviewed := waitForDisclosureTask(t, srv, accepted.TaskID, "reviewed")
	if reviewed.ReviewDecision != "adopted" {
		t.Fatalf("review decision = %q, want adopted", reviewed.ReviewDecision)
	}
	if reviewed.Result == nil || !reviewed.Result.ReviewedByHuman {
		t.Fatal("report should be marked ReviewedByHuman after review")
	}

	records, err := store.List(context.Background(), accepted.TaskID)
	if err != nil {
		t.Fatalf("approval store list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 approval record, got %d", len(records))
	}
	if records[0].Decision != domains.DecisionAdopted {
		t.Fatalf("approval decision = %q, want adopted", records[0].Decision)
	}

	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "disclosure-smoke.md")
	if saveErr := disclosure.SaveReport(reviewed.Result, mdPath); saveErr != nil {
		t.Fatalf("SaveReport markdown: %v", saveErr)
	}
	mdData, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read markdown export: %v", err)
	}
	md := string(mdData)
	if !strings.Contains(md, "技术交底书分析报告") {
		t.Fatal("expected markdown export to contain report title")
	}
	if strings.Contains(md, "尚未经人工复核") {
		t.Fatal("reviewed report should not contain unreviewed warning")
	}
}
