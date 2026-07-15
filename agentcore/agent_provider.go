package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// --- provider call with retry ---

func (a *Agent) callProviderWithRetry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	resp, err := a.callProvider(ctx, req)
	if err == nil {
		return resp, nil
	}

	cfg := a.config.RetryConfig
	if cfg == nil || !IsRetryableError(err) {
		return nil, err
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	for attempt := int64(1); attempt <= maxRetries; attempt++ {
		delay := applyFullJitter(retryDelay(attempt, cfg))
		a.emit(&AutoRetryEvent{
			baseEvent:  newBase(EventAutoRetry),
			Attempt:    attempt,
			MaxRetries: maxRetries,
			Delay:      delay,
			Err:        err,
		})

		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		resp, err = a.callProvider(ctx, req)
		if err == nil {
			return resp, nil
		}
		if !IsRetryableError(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("重试 %d 次后仍然失败: %w", maxRetries, err)
}

// --- internal helpers ---

func (a *Agent) callProvider(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	if a.config.Provider == nil {
		return nil, NewFatalError("provider", "agent provider is nil", nil)
	}
	ctx, span := a.tracer().Start(ctx, "agent.llm",
		Attr("model", req.Model),
		Attr("streaming", a.config.Streaming),
		Attr("tool_count", len(req.Tools)),
	)
	defer span.End()

	var resp *ProviderResponse
	var err error
	if a.config.Streaming {
		resp, err = a.runStreaming(ctx, req)
	} else {
		resp, err = a.config.Provider.Complete(ctx, req)
	}
	if err != nil {
		span.RecordError(err)
		err = NewRetryableError("provider_call", err.Error(), err)
	}
	return resp, err
}

func (a *Agent) runStreaming(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	ch, err := a.config.Provider.Stream(ctx, req)
	if err != nil {
		return nil, NewRetryableError("provider_stream", err.Error(), err)
	}

	var content strings.Builder
	var blocks []ContentBlock
	toolCallMap := make(map[int64]*ToolCall)
	var usage TokenUsage
	var finishReason string

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case delta, ok := <-ch:
			if !ok {
				goto buildResponse
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Default().Error("runStreaming panic recovered", "panic", r, "agent", a.config.Name)
					}
				}()
				if delta.Content != "" {
					content.WriteString(delta.Content)
					kind := BlockKindText
					for _, bl := range delta.Blocks {
						if bl.Kind == BlockKindThinking {
							kind = BlockKindThinking
							break
						}
					}
					a.emit(&MessageDeltaEvent{
						baseEvent: newBase(EventMessageDelta),
						Delta:     delta.Content,
						Kind:      kind,
					})
				}
				if len(delta.Blocks) > 0 {
					blocks = MergeContentBlocks(blocks, delta.Blocks...)
				} else if delta.Content != "" {
					blocks = MergeContentBlocks(blocks, ContentBlock{
						Kind: BlockKindText,
						Text: delta.Content,
					})
				}

				for _, tcd := range delta.ToolCalls {
					tc, ok := toolCallMap[tcd.Index]
					if !ok {
						tc = &ToolCall{}
						toolCallMap[tcd.Index] = tc
					}
					if tcd.ID != "" {
						tc.ID = tcd.ID
					}
					if tcd.Name != "" {
						tc.Name = tcd.Name
					}
					tc.Arguments += tcd.Arguments
				}

				if delta.Usage != nil {
					usage = *delta.Usage
				}
				if delta.FinishReason != "" {
					finishReason = delta.FinishReason
				}
			}()
		}
	}

buildResponse:
	var indices []int64
	for idx := range toolCallMap {
		indices = append(indices, idx)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })

	toolCalls := make([]ToolCall, 0, len(indices))
	for _, idx := range indices {
		toolCalls = append(toolCalls, *toolCallMap[idx])
	}

	return &ProviderResponse{
		Content:      content.String(),
		Blocks:       blocks,
		ToolCalls:    toolCalls,
		Usage:        usage,
		FinishReason: finishReason,
	}, nil
}

// hasInvalidToolCallArgs checks if any tool call in the batch has arguments
// that are not valid JSON. Empty arguments are considered valid (some tools
// take no arguments).
func hasInvalidToolCallArgs(calls []ToolCall) bool {
	for _, tc := range calls {
		if tc.Arguments != "" && !json.Valid([]byte(tc.Arguments)) {
			return true
		}
	}
	return false
}
