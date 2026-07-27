/**
 * AGUI 事件 Reducer。
 *
 * 将 Wails Events 推送的 AGUI 事件负载转换为 Zustand store 的动作。
 *
 * 事件负载的格式由 Go 端 `agui.Converter` 决定。每一条 `agui:*` 事件
 * 的 payload 就是 AGUI 事件结构本身（如 RunStartedEvent 的 JSON）。
 */

import { useChatStore, type ToolCall } from '@/stores/chat'
import { useA2UIStore } from '@/a2ui-renderer/a2ui-store'

/**
 * AGUI 事件负载（通用）。具体结构取决于事件类型。
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type AguiEventPayload = any

// ── 子处理器 ──────────────────────────────────────

function handleAgentStart(payload: AguiEventPayload) {
  const store = useChatStore.getState()
  if (payload.threadId) store.setThreadId(payload.threadId)
  if (payload.runId) store.setRunning(payload.runId)
  store.setToolCallBuffer(null)
}

function handleMessageDelta(payload: AguiEventPayload) {
  const delta = payload.delta ?? payload.content ?? ''
  useChatStore.getState().appendToken(delta)
}

function handleThinkingDelta(payload: AguiEventPayload) {
  const delta = payload.delta ?? payload.content ?? ''
  useChatStore.getState().appendThinking(delta)
}

function handleToolCallStart(payload: AguiEventPayload) {
  const tc: ToolCall = {
    id: payload.toolCallId ?? payload.id ?? '',
    name: payload.toolName ?? payload.name ?? '',
    args: '',
    status: 'running',
    invisible: payload.invisible === true,
  }
  const store = useChatStore.getState()
  store.setToolCallBuffer(tc)
  store.addToolCall(tc)
}

function handleToolCallEnd(payload: AguiEventPayload) {
  const buf = useChatStore.getState().toolCallBuffer
  if (buf) {
    const updated: ToolCall = {
      ...buf,
      status: payload.error ? 'error' : 'completed',
      result: payload.result ?? '',
    }
    useChatStore.getState().updateToolCall(updated.id, updated)
    useChatStore.getState().setToolCallBuffer(null)
  }
}

function handleError(payload: AguiEventPayload) {
  const errMsg = payload.error ?? payload.message ?? 'Unknown error'
  useChatStore.getState().setError(errMsg)
}

function handleA2UI(payload: AguiEventPayload) {
  useA2UIStore.getState().applyEnvelope(payload)
}

function handleApprovalPrompt(payload: AguiEventPayload) {
  const store = useChatStore.getState()
  store.setApprovalPrompt({
    id: payload.id ?? `approval-${Date.now()}`,
    title: payload.title ?? '审批请求',
    description: payload.description,
    options: payload.options,
  })
}

function handleDone() {
  const store = useChatStore.getState()
  store.setToolCallBuffer(null)
  store.finishTurn()
}

function handleContextUsage(payload: AguiEventPayload) {
  const store = useChatStore.getState()
  // ContextUsageEvent fields: usagePercent, tokenUsage.totalTokens, contextWindow
  const percent = payload.usagePercent ?? 0
  const tokenUsage = payload.tokenUsage ?? {}
  const totalTokens = tokenUsage.totalTokens ?? 0
  const contextWindow = payload.contextWindow ?? 128000
  store.setContextUsage(percent, totalTokens, contextWindow)
}

// ── 主分发器 ──────────────────────────────────────

const HANDLERS: Record<string, (payload: AguiEventPayload) => void> = {
  'agent-start': handleAgentStart,
  'message-delta': handleMessageDelta,
  'thinking-delta': handleThinkingDelta,
  'tool-call-start': handleToolCallStart,
  'tool-call-end': handleToolCallEnd,
  error: handleError,
  a2ui: handleA2UI,
  'approval-prompt': handleApprovalPrompt,
  done: handleDone,
  'context-usage': handleContextUsage,
}

/**
 * AGUI 事件类型 → Store action 映射。
 */
export function aguiReducer(eventName: string, payload: AguiEventPayload) {
  const handler = HANDLERS[eventName]
  if (handler) {
    handler(payload)
  }
  // 未注册的事件类型静默忽略（如 handoff-start/end 等 Invisible Handoff 事件）
}
