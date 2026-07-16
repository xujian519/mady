package domains

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
)

// handoffProvider 模拟 LLM：第一次返回 transfer_to_<name> 工具调用，之后返回 content。
// 在 integration_test.go（集成测试）和 fallback_test.go（单元测试）中共享。
type handoffProvider struct {
	called  atomic.Int64
	tool    string
	content string
}

func (p *handoffProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	call := p.called.Add(1) - 1
	if call == 0 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{
				{ID: "call_handoff", Name: "transfer_to_" + p.tool, Arguments: `{"message":"test input"}`},
			},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: p.content}, nil
}

func (p *handoffProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("streaming not implemented")
}
