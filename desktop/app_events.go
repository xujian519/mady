//go:build darwin

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
)

// emitAguiEvent 将 agentcore.Event 通过 AGUI converter 转换为 AGUI 事件，
// 再映射为 Wails 事件名并通过 Wails Events 推送到前端。
func (a *App) emitAguiEvent(ctx context.Context, converter *agui.Converter, e agentcore.Event) {
	aguiEvents := converter.Convert(e)
	for _, ev := range aguiEvents {
		name := mapAguiEventToWailsName(ev)
		runtime.EventsEmit(ctx, name, ev)
	}
}

// mapAguiEventToWailsName 将 AGUI 事件映射为前端订阅的 Wails 事件名。
// 标准事件：agui: + kebab-case(EventType)
// 自定义事件：agui: + kebab-case(CustomEvent.Name)
func mapAguiEventToWailsName(ev any) string {
	switch e := ev.(type) {
	case agui.RunStartedEvent:
		return "agui:agent-start"
	case agui.TextMessageContentEvent:
		return "agui:message-delta"
	case agui.ThinkingTextMessageContentEvent:
		return "agui:thinking-delta"
	case agui.ToolCallStartEvent:
		return "agui:tool-call-start"
	case agui.ToolCallEndEvent:
		return "agui:tool-call-end"
	case agui.RunErrorEvent:
		return "agui:error"
	case agui.CustomEvent:
		return "agui:" + toKebabCase(e.Name)
	default:
		return "agui:" + toKebabCase(string(eventTypeOf(ev)))
	}
}

// eventTypeOf 从 AGUI 事件中提取 EventType 字符串。
// 所有 AGUI 事件都内嵌 BaseEvent，支持 GetType() 方法。
func eventTypeOf(ev any) agui.EventType {
	if typed, ok := ev.(interface{ GetType() agui.EventType }); ok {
		return typed.GetType()
	}
	return agui.EventRaw
}

// generateRunID 生成唯一的 run 标识符。
func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

// toKebabCase 将 SCREAMING_SNAKE_CASE 或 camelCase 转换为 kebab-case。
// "RUN_STARTED" → "run-started", "handoff_start" → "handoff-start"
func toKebabCase(s string) string {
	if s == "" {
		return ""
	}
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			if i > 0 && s[i-1] >= 'a' && s[i-1] <= 'z' {
				result = append(result, '-')
			}
			result = append(result, c+('a'-'A'))
		case c == '_':
			result = append(result, '-')
		default:
			result = append(result, c)
		}
	}
	return string(result)
}
