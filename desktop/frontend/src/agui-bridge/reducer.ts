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
    // Go 端 ToolCallStartEvent 的 JSON 字段为 toolCallName
    name: payload.toolCallName ?? payload.toolName ?? payload.name ?? '',
    args: '',
    status: 'running',
    invisible: payload.invisible === true,
  }
  const store = useChatStore.getState()
  store.setToolCallBuffer(tc)
  store.addToolCall(tc)
}

/** tool-call-args：流式填充 ToolCard 参数（增量追加）。 */
function handleToolCallArgs(payload: AguiEventPayload) {
  const id = payload.toolCallId ?? ''
  const delta = payload.delta ?? ''
  if (!id || !delta) return
  const store = useChatStore.getState()
  store.appendToolCallArgs(id, delta)
  // 同步缓冲区，保证 tool-call-end 合并时参数完整
  const buf = useChatStore.getState().toolCallBuffer
  if (buf && buf.id === id) {
    store.setToolCallBuffer({ ...buf, args: buf.args + delta })
  }
}

/** tool-call-result：显示工具调用结果（Go 端在 tool-call-end 后立即发出）。 */
function handleToolCallResult(payload: AguiEventPayload) {
  const id = payload.toolCallId ?? ''
  if (!id) return
  useChatStore.getState().updateToolCall(id, { result: payload.content ?? '' })
}

/** step-started：进度指示器（多步 turn 场景）。 */
function handleStepStarted(payload: AguiEventPayload) {
  useChatStore.getState().setStep(payload.stepName ?? '')
}

/** step-finished：收起进度指示器。 */
function handleStepFinished() {
  useChatStore.getState().finishStep()
}

/** compaction-start：上下文压缩提示（CustomEvent，payload.value 携带数据）。 */
function handleCompactionStart(payload: AguiEventPayload) {
  const v = payload.value ?? {}
  useChatStore.getState().setCompaction({
    active: true,
    tokensBefore: v.tokens_before,
  })
}

/** compaction-end：压缩完成摘要（保留展示至下一轮）。 */
function handleCompactionEnd(payload: AguiEventPayload) {
  const v = payload.value ?? {}
  useChatStore.getState().setCompaction({
    active: false,
    tokensBefore: v.tokens_before,
    tokensAfter: v.tokens_after,
    messagesCut: v.messages_cut,
    durationMs: v.duration_ms,
  })
}

/** auto-retry：自动重试提示（CustomEvent，收到后续 token 时自动清除）。 */
function handleAutoRetry(payload: AguiEventPayload) {
  const v = payload.value ?? {}
  useChatStore.getState().setRetryNotice({
    attempt: v.attempt ?? 0,
    maxRetries: v.max_retries ?? 0,
    delayMs: v.delay_ms ?? 0,
  })
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
  'tool-call-args': handleToolCallArgs,
  'tool-call-end': handleToolCallEnd,
  'tool-call-result': handleToolCallResult,
  'step-started': handleStepStarted,
  'step-finished': handleStepFinished,
  'compaction-start': handleCompactionStart,
  'compaction-end': handleCompactionEnd,
  'auto-retry': handleAutoRetry,
  error: handleError,
  a2ui: handleA2UI,
  'approval-prompt': handleApprovalPrompt,
  done: handleDone,
  'context-usage': handleContextUsage,
}

/**
 * AGUI 事件类型 → Store action 映射。
 *
 * 有意不订阅的事件（当前累加器模型已覆盖，订阅后无消费者）：
 * - text-message-start/end：消息边界由 finishTurn 统一提交
 * - thinking-start/end：思考区块由 thinking 累加器 + running 状态驱动
 */
export function aguiReducer(eventName: string, payload: AguiEventPayload) {
  const handler = HANDLERS[eventName]
  if (handler) {
    handler(payload)
  }
  // 未注册的事件类型静默忽略（如 handoff-start/end 等 Invisible Handoff 事件）
}
