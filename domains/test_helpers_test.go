package domains

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tools"
)

// testToolExt 为 domains 包内的测试创建一个最小工具扩展。
// 测试不实际使用工具，仅需要一个合法的 Extension 实例通过参数校验。
func testToolExt() agentcore.Extension {
	return tools.NewExtension(tools.ExtensionConfig{
		WorkingDir: "/tmp/mady-test",
	})
}

// testToolExtTuple 返回三个工具扩展（unified/patent/legal），供 UnifiedAgentConfig 测试使用。
func testToolExtTuple() (agentcore.Extension, agentcore.Extension, agentcore.Extension) {
	base := testToolExt()
	return base, base, base
}

// testRecord 创建一个最小 ProjectRecord 用于测试。
func testRecord() ProjectRecord {
	return ProjectRecord{
		ProjectID: "test-case-001",
		RootPath:  "/tmp/test-case",
		Status:    StatusActive,
	}
}

// handoffProvider 模拟 LLM Provider，支持指定首次返回工具调用（handoff）、
// 后续返回固定内容的模式。用于 Handoff 流程的单元/集成测试。
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
				{ID: "call_handoff", Name: "transfer_to_" + p.tool, Arguments: `{"message":"test"}`},
			},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: p.content}, nil
}

func (p *handoffProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("streaming not implemented")
}
