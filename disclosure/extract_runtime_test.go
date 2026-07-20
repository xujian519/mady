package disclosure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xujian519/mady/provider/chatcompat"
)

func TestExtractionNode_DefaultDeepSeekOmitsResponseFormatAtRuntime(t *testing.T) {
	t.Setenv("PROVIDER", "")

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_extract",
			"choices":[{"message":{"role":"assistant","content":"{\"problems\":[{\"id\":\"p1\",\"text\":\"功耗高\",\"confidence\":0.9}]}"}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer srv.Close()

	provider := chatcompat.New(chatcompat.Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Client:  srv.Client(),
	})

	node := newExtractionAgent(provider, extractProblems, StateKeyExtractProblem)
	state := map[string]any{
		StateKeyDoc: &DisclosureDoc{RawText: "背景技术：现有方案功耗高。"},
	}
	if _, err := node(context.Background(), state); err != nil {
		t.Fatalf("node run: %v", err)
	}
	if _, ok := gotBody["response_format"]; ok {
		t.Fatalf("response_format should be omitted, got %#v", gotBody["response_format"])
	}
}
