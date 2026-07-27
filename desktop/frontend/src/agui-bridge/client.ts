/**
 * AGUI → Wails Events 桥接层。
 *
 * 在 Wails OnStartup 之后调用 subscribeAguiEvents()，
 * 订阅后端通过 `runtime.EventsEmit` 推送的所有 `agui:*` 事件，
 * 转换后投递给 Zustand store。
 *
 * 事件名映射规则（与 Go 端 mapAguiEventToWailsName 保持一致）：
 *   agui:agent-start      → RunStarted 事件
 *   agui:message-delta    → TextMessageContent 事件
 *   agui:thinking-delta   → ThinkingTextMessageContent 事件
 *   agui:tool-call-start  → ToolCallStart 事件
 *   agui:tool-call-end    → ToolCallEnd 事件
 *   agui:error            → RunError 事件
 *   agui:done             → 流式结束事件（Go 端自行 emit，非 AGUI 标准）
 *   agui:+kebab           → 其余类型（含 CustomEvent）
 */

import { listenToWailsEvent } from '@/lib/wails'
import { aguiReducer, type AguiEventPayload } from './reducer'

/** 需要订阅的 agui:* 事件名称列表（不含 'agui:' 前缀）。 */
const SUBSCRIBED_EVENTS = [
  'message-delta',
  'thinking-delta',
  'agent-start',
  'tool-call-start',
  'tool-call-end',
  'error',
  'a2ui',
  'approval-prompt',
  'done',
] as const

/**
 * 订阅所有 agui:* Wails 事件并分发到 store。
 * 返回 unsubscribe 函数，组件卸载时调用。
 */
export function subscribeAguiEvents(): () => void {
  const unsubscribers = SUBSCRIBED_EVENTS.map((name) =>
    listenToWailsEvent(`agui:${name}`, (payload: AguiEventPayload) => {
      aguiReducer(name, payload)
    }),
  )
  return () => {
    unsubscribers.forEach((fn) => fn())
  }
}
