package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestNewStdoutTracer(t *testing.T) {
	tracer, shutdown, err := NewStdoutTracer("test-svc")
	if err != nil {
		t.Fatalf("NewStdoutTracer: %v", err)
	}
	if tracer == nil {
		t.Fatal("tracer is nil")
	}

	ctx, span := tracer.Start(context.Background(), "op.test",
		agentcore.Attr("kind", "unit-test"),
		agentcore.Attr("count", 42),
	)
	if span == nil {
		t.Fatal("span is nil")
	}

	span.SetAttributes(agentcore.Attr("extra", true))
	span.AddEvent("doing work", agentcore.Attr("step", 1))
	span.RecordError(errors.New("simulated"))
	span.End()

	if ctx == nil {
		t.Fatal("context is nil")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestToKV(t *testing.T) {
	cases := []struct {
		attr agentcore.SpanAttribute
	}{
		{agentcore.Attr("s", "hello")},
		{agentcore.Attr("i", 7)},
		{agentcore.Attr("i64", int64(700))},
		{agentcore.Attr("f", 3.14)},
		{agentcore.Attr("b", true)},
		{agentcore.Attr("slice", []string{"a", "b"})},
		{agentcore.Attr("other", struct{ X int }{X: 1})}, // falls back to %v
	}
	for _, c := range cases {
		kv := toKV(c.attr)
		if string(kv.Key) != c.attr.Key {
			t.Fatalf("key mismatch: %s vs %s", kv.Key, c.attr.Key)
		}
	// just ensure no panic and a valid type is produced.
	_ = kv.Value.String()
	}
}

func TestTracingMiddleware_WithOTel(t *testing.T) {
	tracer, shutdown, err := NewStdoutTracer("middleware-test")
	if err != nil {
		t.Fatalf("NewStdoutTracer: %v", err)
	}
	defer shutdown(context.Background())

	mw := agentcore.TracingMiddleware(tracer)
	called := false
	wrapped := mw(func(ctx context.Context, tc agentcore.ToolCall) (string, error) {
		called = true
		return "ok", nil
	})

	out, err := wrapped(context.Background(), agentcore.ToolCall{ID: "tc1", Name: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" || !called {
		t.Fatalf("middleware did not pass through correctly: out=%q called=%v", out, called)
	}
}

func TestOTelTracer_RecordErrorNilSafe(t *testing.T) {
	tracer, shutdown, err := NewStdoutTracer("nilsafe")
	if err != nil {
		t.Fatalf("NewStdoutTracer: %v", err)
	}
	defer shutdown(context.Background())

	_, span := tracer.Start(context.Background(), "op.nilsafe")
	span.RecordError(nil) // must not panic
	span.End()
}
