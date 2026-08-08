package chatcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// collectDeltas drains a stream channel and returns all deltas.
func collectDeltas(t *testing.T, ch <-chan agentcore.StreamDelta) []agentcore.StreamDelta {
	t.Helper()
	var out []agentcore.StreamDelta
	for d := range ch {
		out = append(out, d)
	}
	return out
}

func streamRequest() *agentcore.ProviderRequest {
	return &agentcore.ProviderRequest{
		Model:    "test-model",
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: "hi"}},
	}
}

// TestStream_LargeDataLine verifies that a single SSE data line larger than
// bufio.Scanner's default 64KB token limit is fully consumed. Before the fix
// the scanner stopped silently and the content was truncated.
func TestStream_LargeDataLine(t *testing.T) {
	const contentLen = 200 * 1024 // > 64KB default scanner limit
	big := strings.Repeat("a", contentLen)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk, _ := json.Marshal(map[string]any{
			"id":      "chunk_big",
			"choices": []map[string]any{{"delta": map[string]any{"content": big}}},
		})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		finish, _ := json.Marshal(map[string]any{
			"id":      "chunk_fin",
			"choices": []map[string]any{{"delta": map[string]any{}, "finish_reason": "stop"}},
		})
		fmt.Fprintf(w, "data: %s\n\n", finish)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	provider := New(Config{APIKey: "k", BaseURL: srv.URL})
	ch, err := provider.Stream(context.Background(), streamRequest())
	if err != nil {
		t.Fatal(err)
	}

	deltas := collectDeltas(t, ch)
	var content strings.Builder
	var finishReason string
	for _, d := range deltas {
		content.WriteString(d.Content)
		if d.FinishReason != "" {
			finishReason = d.FinishReason
		}
	}
	if content.Len() != contentLen {
		t.Fatalf("content length = %d, want %d (silent truncation?)", content.Len(), contentLen)
	}
	if finishReason != "stop" {
		t.Fatalf("finishReason = %q, want stop", finishReason)
	}
}

// TestStream_ServerClosesWithoutDone verifies that when the server drops the
// connection mid-stream without sending [DONE] or a finish_reason, the
// provider signals abnormal termination via a terminal delta with
// FinishReason "error" instead of closing the channel silently.
func TestStream_ServerClosesWithoutDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"c1","choices":[{"delta":{"content":"partial"}}]}` + "\n\n"))
		// Handler returns without [DONE]: connection closes mid-stream.
	}))
	defer srv.Close()

	provider := New(Config{APIKey: "k", BaseURL: srv.URL})
	ch, err := provider.Stream(context.Background(), streamRequest())
	if err != nil {
		t.Fatal(err)
	}

	deltas := collectDeltas(t, ch)
	if len(deltas) == 0 {
		t.Fatal("no deltas received")
	}
	last := deltas[len(deltas)-1]
	if last.FinishReason != streamFinishReasonError {
		t.Fatalf("last delta FinishReason = %q, want %q (abnormal end must be signalled)",
			last.FinishReason, streamFinishReasonError)
	}
}

// TestStream_MidStreamErrorPayload verifies that a `data: {"error":...}`
// payload mid-stream terminates the stream with FinishReason "error" rather
// than being silently parsed as an empty chunk.
func TestStream_MidStreamErrorPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"c1","choices":[{"delta":{"content":"before"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"error":{"message":"boom","type":"server_error","code":"500"}}` + "\n\n"))
	}))
	defer srv.Close()

	provider := New(Config{APIKey: "k", BaseURL: srv.URL})
	ch, err := provider.Stream(context.Background(), streamRequest())
	if err != nil {
		t.Fatal(err)
	}

	deltas := collectDeltas(t, ch)
	last := deltas[len(deltas)-1]
	if last.FinishReason != streamFinishReasonError {
		t.Fatalf("last delta FinishReason = %q, want %q", last.FinishReason, streamFinishReasonError)
	}
	var content string
	for _, d := range deltas {
		content += d.Content
	}
	if content != "before" {
		t.Fatalf("content = %q, want %q", content, "before")
	}
}

// TestStream_MalformedDataLineSkipped verifies that an unparseable data line
// is skipped (with a warning) and does not kill the stream.
func TestStream_MalformedDataLineSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {not-json\n\n"))
		_, _ = w.Write([]byte(`data: {"id":"c1","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	provider := New(Config{APIKey: "k", BaseURL: srv.URL})
	ch, err := provider.Stream(context.Background(), streamRequest())
	if err != nil {
		t.Fatal(err)
	}

	deltas := collectDeltas(t, ch)
	if len(deltas) != 1 || deltas[0].Content != "ok" || deltas[0].FinishReason != "stop" {
		t.Fatalf("deltas = %#v", deltas)
	}
}

// TestStream_DefaultClientsTimeoutConfig pins the timeout contract: the
// non-streaming client keeps the 5-minute overall timeout, while the
// streaming client has no overall Timeout (streams are bounded by context
// cancellation) and only bounds the connect / time-to-headers phase.
func TestStream_DefaultClientsTimeoutConfig(t *testing.T) {
	provider := New(Config{APIKey: "k", BaseURL: "http://127.0.0.1:1"})

	if provider.client.Timeout != 5*time.Minute {
		t.Fatalf("non-stream client Timeout = %v, want 5m", provider.client.Timeout)
	}
	if provider.streamClient.Timeout != 0 {
		t.Fatalf("stream client Timeout = %v, want 0 (context-driven cancellation)", provider.streamClient.Timeout)
	}
	tr, ok := provider.streamClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("stream client transport = %T, want *http.Transport", provider.streamClient.Transport)
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Fatalf("stream transport ResponseHeaderTimeout = %v, want > 0", tr.ResponseHeaderTimeout)
	}
}

// TestStream_NotBoundByOverallClientTimeout is the functional counterpart of
// TestStream_DefaultClientsTimeoutConfig: a streaming request served through
// the default stream client (no custom Client configured) completes even
// though the exchange would have exceeded any short overall client timeout.
func TestStream_NotBoundByOverallClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i, text := range []string{"one", "two", "three"} {
			chunk, _ := json.Marshal(map[string]any{
				"id":      fmt.Sprintf("c%d", i),
				"choices": []map[string]any{{"delta": map[string]any{"content": text}}},
			})
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
		_, _ = w.Write([]byte(`data: {"id":"cf","choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	// No custom Client: exercises the default streamClient path.
	provider := New(Config{APIKey: "k", BaseURL: srv.URL})
	ch, err := provider.Stream(context.Background(), streamRequest())
	if err != nil {
		t.Fatal(err)
	}

	var content string
	var finishReason string
	for d := range ch {
		content += d.Content
		if d.FinishReason != "" {
			finishReason = d.FinishReason
		}
	}
	if content != "onetwothree" {
		t.Fatalf("content = %q, want onetwothree", content)
	}
	if finishReason != "stop" {
		t.Fatalf("finishReason = %q, want stop", finishReason)
	}
}
