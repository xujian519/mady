package sanitizer

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// SanitizingProvider wraps an agentcore.Provider to automatically sanitize PII
// from outgoing LLM requests and restore original values in incoming responses.
//
// It implements the Provider interface transparently: callers that receive a
// SanitizingProvider use it exactly like the wrapped provider.
type SanitizingProvider struct {
	inner agentcore.Provider
	rules []Rule
}

// New wraps the given provider with PII sanitization. The returned Provider
// is safe to use anywhere the original provider was used.
func New(inner agentcore.Provider) *SanitizingProvider {
	return &SanitizingProvider{
		inner: inner,
		rules: defaultRules(),
	}
}

// WithRules returns a copy of the provider using the given rules instead of
// the defaults. This is primarily useful for testing.
func (p *SanitizingProvider) WithRules(rules []Rule) *SanitizingProvider {
	return &SanitizingProvider{
		inner: p.inner,
		rules: rules,
	}
}

// Complete sanitizes PII from the request, forwards to the inner provider,
// and restores original values in the response.
func (p *SanitizingProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	if req == nil {
		return p.inner.Complete(ctx, req)
	}

	// Clone the request so we don't mutate the caller's data.
	sanitizedReq, rm := p.sanitizeRequest(req)

	resp, err := p.inner.Complete(ctx, sanitizedReq)
	if err != nil {
		return resp, err
	}

	if resp != nil {
		resp = p.restoreResponse(resp, rm)
	}
	return resp, nil
}

// Stream sanitizes PII from the request, forwards to the inner provider,
// and restores original values in each stream delta.
func (p *SanitizingProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	if req == nil {
		return p.inner.Stream(ctx, req)
	}

	sanitizedReq, rm := p.sanitizeRequest(req)

	innerCh, err := p.inner.Stream(ctx, sanitizedReq)
	if err != nil {
		return nil, err
	}

	outCh := make(chan agentcore.StreamDelta, 64)
	go func() {
		defer close(outCh)
		for delta := range innerCh {
			delta.Content = rm.restore(delta.Content)
			if len(delta.Blocks) > 0 {
				blocks := make([]agentcore.ContentBlock, len(delta.Blocks))
				for i, b := range delta.Blocks {
					blocks[i] = b
					blocks[i].Text = rm.restore(b.Text)
				}
				delta.Blocks = blocks
			}
			select {
			case outCh <- delta:
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

// sanitizeRequest clones the request and replaces PII in all message content.
// It returns the cloned request and a replacement map for later restoration.
func (p *SanitizingProvider) sanitizeRequest(req *agentcore.ProviderRequest) (*agentcore.ProviderRequest, *replacementMap) {
	clone := *req
	rm := newReplacementMap()

	if len(req.Messages) > 0 {
		msgs := make([]agentcore.Message, len(req.Messages))
		for i, msg := range req.Messages {
			msgs[i] = msg.Clone()
			msgs[i].Content = sanitizeText(msgs[i].Content, p.rules, rm)
			if len(msgs[i].Blocks) > 0 {
				blocks := make([]agentcore.ContentBlock, len(msgs[i].Blocks))
				for j, b := range msgs[i].Blocks {
					blocks[j] = b
					blocks[j].Text = sanitizeText(b.Text, p.rules, rm)
				}
				msgs[i].Blocks = blocks
			}
		}
		clone.Messages = msgs
	}

	return &clone, rm
}

// restoreResponse replaces placeholders in the response with their original values.
func (p *SanitizingProvider) restoreResponse(resp *agentcore.ProviderResponse, rm *replacementMap) *agentcore.ProviderResponse {
	resp.Content = rm.restore(resp.Content)
	if len(resp.Blocks) > 0 {
		blocks := make([]agentcore.ContentBlock, len(resp.Blocks))
		for i, b := range resp.Blocks {
			blocks[i] = b
			blocks[i].Text = rm.restore(b.Text)
		}
		resp.Blocks = blocks
	}
	if len(resp.ToolCalls) > 0 {
		tcs := make([]agentcore.ToolCall, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			tcs[i] = tc
			tcs[i].Arguments = rm.restore(tc.Arguments)
		}
		resp.ToolCalls = tcs
	}
	return resp
}
