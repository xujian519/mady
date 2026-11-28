package agui

// 本文件收纳 Converter.Convert 按事件族拆出的组装函数（M68）。
//
// 帧结构与事件顺序都是对外契约（见 contract_test.go 的 golden fixture），
// 这里只做「形状搬迁」：不新增/删除事件、不调整字段、不改变收尾顺序。

import (
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// afterClose 先关闭进行中的消息/思考块，再追加 tail 事件。
// 各事件族共用「收尾在前、自身事件在后」的顺序约定。
func (c *Converter) afterClose(t time.Time, tail []any) []any {
	return append(c.closeAll(t), tail...)
}

// convertRunStarted 组装 RUN_STARTED（不触发收尾）。
func (c *Converter) convertRunStarted(t time.Time) []any {
	return []any{c.RunStarted(t)}
}

// convertAgentEnd 组装 AGENT_END：收尾进行中的块后发 success 的 RUN_FINISHED。
func (c *Converter) convertAgentEnd(t time.Time, output, finishReason string) []any {
	return c.afterClose(t, []any{c.RunFinishedWithSuccess(t, output, finishReason)})
}

// convertAgentError 组装 AGENT_ERROR：收尾进行中的块后发 RUN_ERROR。
func (c *Converter) convertAgentError(t time.Time, err error) []any {
	return c.afterClose(t, []any{c.RunError(t, err)})
}

// convertTurnStart 组装 STEP_STARTED（步名为 turn_<n>）。
func (c *Converter) convertTurnStart(t time.Time, turn int64) []any {
	return c.afterClose(t, []any{StepStartedEvent{
		BaseEvent: baseEvent(EventStepStarted, t),
		StepName:  fmt.Sprintf("turn_%d", turn),
	}})
}

// convertTurnEnd 组装 STEP_FINISHED，累加本轮流用量，并在有累计用量时补 CONTEXT_USAGE。
func (c *Converter) convertTurnEnd(t time.Time, turn int64, usage agentcore.TokenUsage) []any {
	events := c.afterClose(t, []any{StepFinishedEvent{
		BaseEvent: baseEvent(EventStepFinished, t),
		StepName:  fmt.Sprintf("turn_%d", turn),
	}})
	c.accumulateUsage(usage, t)
	if c.totalTokens > 0 {
		events = append(events, c.buildContextUsage(t))
	}
	return events
}
