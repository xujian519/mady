package specdrafting

import (
	"context"
	"log"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// LLMDrafter 使用 LLM 增强撰写说明书。
// Provider 可用时执行 Pregel 图（逐节点 LLM Agent），
// 否则降级为 SpecBuilder 模板填充。
type LLMDrafter struct {
	provider agentcore.Provider
	builder  *SpecBuilder
	compiled *graph.CompiledPregelGraph // 缓存的编译图
}

// NewLLMDrafter 创建 LLM 撰写器。
func NewLLMDrafter(provider agentcore.Provider, builder *SpecBuilder) *LLMDrafter {
	d := &LLMDrafter{provider: provider, builder: builder}
	if provider != nil {
		// 预编译 Pregel 图
		compiled, err := BuildSpecificationGraph(provider, nil, nil)
		if err == nil {
			d.compiled = compiled
		} else {
			log.Printf("specdrafting: 预编译 Pregel 图失败（将降级为 Builder）: %v", err)
		}
	}
	return d
}

// Draft 基于输入生成说明书。
//   - LLM 模式：provider 可用且图编译成功时，执行 Pregel 图
//   - 降级模式：回退到 Builder 模板填充
func (d *LLMDrafter) Draft(input SpecInput) *SpecOutput {
	if d.compiled != nil {
		state, err := d.compiled.Run(context.Background(), graph.PregelState{
			StateKeyInput: &input,
		})
		if err == nil {
			if output, ok := state[StateKeyOutput].(*SpecOutput); ok && output != nil {
				return output
			}
		}
		log.Printf("specdrafting: Pregel 图执行失败（将降级为 Builder）: %v", err)
	}

	// 降级路径
	if d.builder != nil {
		return d.builder.Build(input)
	}
	return NewSpecBuilder(nil).Build(input)
}

// DraftAvailable 返回是否可以执行 LLM 撰写。
func (d *LLMDrafter) DraftAvailable() bool {
	return d.compiled != nil
}

// Provider 返回底层 LLM provider。
func (d *LLMDrafter) Provider() agentcore.Provider {
	return d.provider
}

// Builder 返回底层模板构建器。
func (d *LLMDrafter) Builder() *SpecBuilder {
	return d.builder
}
