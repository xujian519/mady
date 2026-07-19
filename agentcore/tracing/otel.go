// Package tracing provides a concrete OpenTelemetry implementation of the
// agentcore.Tracer interface, bridging Mady's lightweight tracing abstraction
// to the OpenTelemetry SDK.
//
// agentcore keeps tracing abstract (the Tracer/Span interfaces in
// agentcore/tracer.go) so the core package stays free of heavy dependencies.
// This package is the opt-in bridge: import it, configure a tracer, and pass it
// to agentcore.Config.Tracer (and agentcore.TracingMiddleware) to emit real
// spans to any OTel-compatible backend (Jaeger, Tempo, stdout, OTLP, …).
package tracing

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Compile-time interface checks.
var (
	_ agentcore.Tracer = (*otelTracer)(nil)
	_ agentcore.Span   = (*otelSpan)(nil)
)

// otelTracer adapts an OpenTelemetry trace.Tracer to agentcore.Tracer.
type otelTracer struct {
	tracer trace.Tracer
}

// Start begins a new span, returning a context carrying it and the span.
func (t *otelTracer) Start(ctx context.Context, name string, attrs ...agentcore.SpanAttribute) (context.Context, agentcore.Span) {
	ctx, span := t.tracer.Start(ctx, name, trace.WithAttributes(toOTelAttrs(attrs)...))
	return ctx, &otelSpan{span: span}
}

// otelSpan adapts an OpenTelemetry trace.Span to agentcore.Span.
type otelSpan struct {
	span trace.Span
}

// End completes the span.
func (s *otelSpan) End() { s.span.End() }

// SetAttributes attaches key-value attributes to the span.
func (s *otelSpan) SetAttributes(attrs ...agentcore.SpanAttribute) {
	s.span.SetAttributes(toOTelAttrs(attrs)...)
}

// RecordError records an error as a span event.
func (s *otelSpan) RecordError(err error) {
	if err != nil {
		s.span.RecordError(err)
	}
}

// AddEvent records a point-in-time event on the span.
func (s *otelSpan) AddEvent(name string, attrs ...agentcore.SpanAttribute) {
	s.span.AddEvent(name, trace.WithAttributes(toOTelAttrs(attrs)...))
}

// toOTelAttrs converts agentcore span attributes to OTel key-values.
func toOTelAttrs(attrs []agentcore.SpanAttribute) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, toKV(a))
	}
	return out
}

// toKV converts a single agentcore.SpanAttribute to an OTel KeyValue,
// inferring the value type.
func toKV(a agentcore.SpanAttribute) attribute.KeyValue {
	k := attribute.Key(a.Key)
	switch v := a.Value.(type) {
	case string:
		return k.String(v)
	case int:
		return k.Int64(int64(v))
	case int64:
		return k.Int64(v)
	case float64:
		return k.Float64(v)
	case bool:
		return k.Bool(v)
	case []string:
		return k.StringSlice(v)
	default:
		return k.String(fmt.Sprintf("%v", v))
	}
}

// NewFromTracerProvider builds an agentcore.Tracer from an existing OTel
// TracerProvider. Use this when you manage your own provider/exporter
// (e.g. OTLP exporter to a collector).
func NewFromTracerProvider(tp trace.TracerProvider, name string) agentcore.Tracer {
	return &otelTracer{tracer: tp.Tracer(name)}
}

// NewStdoutTracer creates a fully-configured agentcore.Tracer that pretty-prints
// spans to stdout. It returns the tracer and a shutdown function that must be
// called (typically via defer) to flush pending spans before exit.
//
// This is the simplest way to get started with tracing in Mady. For production,
// prefer NewFromTracerProvider with an OTLP exporter.
func NewStdoutTracer(name string) (agentcore.Tracer, func(context.Context) error, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, nil, fmt.Errorf("tracing: create stdout exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", name),
		)),
	)
	return &otelTracer{tracer: tp.Tracer(name)}, tp.Shutdown, nil
}
