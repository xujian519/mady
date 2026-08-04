/**
 * AGUI 事件 Reducer。
 *
 * 将 Wails Events 推送的 AGUI 事件负载转换为 Zustand store 的动作。
 *
 * 事件负载的格式由 Go 端 `agui.Converter` 决定。每一条 `agui:*` 事件
 * 的 payload 就是 AGUI 事件结构本身（如 RunStartedEvent 的 JSON）。
 */

import { useChatStore, type ToolCall, type TaskItem } from '@/stores/chat'
import { useA2UIStore } from '@/a2ui-renderer/a2ui-store'

/**
 * AGUI 事件负载（通用）。具体结构取决于事件类型。
 */
export type AguiEventPayload = any

// ── 流式 delta 批处理（M-DSK-PRF-001/004，G-I5） ──
//
// Wails v2 事件经 ExecJS 全量推送到前端，逐 token 高频 setState 会让
// 每个 token 触发整轮 re-render。这里按 16ms 合并一批再写入 store：
//  - output 与 thinking 各自缓冲，同批内顺序保持；
//  - 非流式事件（tool-call/step 等）即时处理，与文本流解耦；
//  - run 结束（handleDone）前强制 flush，确保最后一批 token 不丢失。
const DELTA_BATCH_MS = 16
let deltaBuffer = { output: '', thinking: '' }
let deltaTimer: ReturnType<typeof setTimeout> | null = null

function flushDeltas() {
  if (deltaTimer) {
    clearTimeout(deltaTimer)
    deltaTimer = null
  }
  const { output, thinking } = deltaBuffer
  deltaBuffer = { output: '', thinking: '' }
  if (output) useChatStore.getState().appendToken(output)
  if (thinking) useChatStore.getState().appendThinking(thinking)
}

function scheduleDeltaFlush() {
  if (!deltaTimer) deltaTimer = setTimeout(flushDeltas, DELTA_BATCH_MS)
}

// ── 子处理器 ──────────────────────────────────────

function handleAgentStart(payload: AguiEventPayload) {
  const store = useChatStore.getState()
  if (payload.threadId) store.setThreadId(payload.threadId)
  if (payload.runId) store.setRunning(payload.runId)
  store.setToolCallBuffer(null)
}

function handleMessageDelta(payload: AguiEventPayload) {
  deltaBuffer.output += payload.delta ?? payload.content ?? ''
  // 收到新 token 即清除重试提示（不等批 flush）：
  // appendToken 内清 retryNotice 的逻辑随 16ms 批处理被延迟，
  // 此处立即生效，保持「收到 token → 重试提示消失」的既有语义（G-I5 回归修复）。
  useChatStore.setState({ retryNotice: null })
  scheduleDeltaFlush()
}

function handleThinkingDelta(payload: AguiEventPayload) {
  deltaBuffer.thinking += payload.delta ?? payload.content ?? ''
  scheduleDeltaFlush()
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
  // F-I5：A2UI 信封处理异常（如重复 createSurface 抛 SurfaceExistsError）
  // 不应中断 AGUI 事件流——捕获并记录，事件流继续。
  try {
    useA2UIStore.getState().applyEnvelope(payload)
  } catch (err) {
    console.error('[agui] applyEnvelope failed:', err)
  }
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

function handleDone(payload: AguiEventPayload) {
  const store = useChatStore.getState()
  // G-I5：run 结束前强制 flush 流式缓冲，避免最后一批 token 丢失
  flushDeltas()
  store.setToolCallBuffer(null)
  store.finishTurn()
  // 运行级错误（如线程加载失败）经 agui:done 的 error 字段传递（Go 侧 startChatRun 塞入），
  // 不额外发 agui:error —— 此处读取并反馈 UI，避免静默无反馈。
  const errMsg = payload.error
  if (errMsg) {
    store.setError(typeof errMsg === 'string' ? errMsg : 'Unknown error')
  }
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

/** task-created：新待办任务创建（来自 agentcore/tasklist extension）。 */
function handleTaskCreated(payload: AguiEventPayload) {
  // payload 是 CustomEvent.Value，内嵌 TaskCreatedEvent.
  // TaskCreatedEvent 结构：{ task: { id, subject, status, priority, activeForm } }
  const taskRaw = payload.task ?? payload
  if (!taskRaw?.id) return
  const task: TaskItem = {
    id: String(taskRaw.id),
    subject: String(taskRaw.subject ?? ''),
    status: (taskRaw.status ?? 'pending') as TaskItem['status'],
    priority: (taskRaw.priority ?? 'normal') as TaskItem['priority'],
    activeForm: taskRaw.activeForm ?? undefined,
  }
  useChatStore.getState().upsertTask(task)
}

/** task-updated：待办任务状态变更。 */
function handleTaskUpdated(payload: AguiEventPayload) {
  // payload 是 CustomEvent.Value，内嵌 TaskUpdatedEvent.
  // TaskUpdatedEvent 结构：{ task: { id, subject, status, ... }, newStatus }
  const taskRaw = payload.task ?? payload
  if (!taskRaw?.id) return
  const task: TaskItem = {
    id: String(taskRaw.id),
    subject: String(taskRaw.subject ?? ''),
    status: (taskRaw.status ?? 'pending') as TaskItem['status'],
    priority: (taskRaw.priority ?? 'normal') as TaskItem['priority'],
    activeForm: taskRaw.activeForm ?? undefined,
  }
  useChatStore.getState().upsertTask(task)
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
  'task-created': handleTaskCreated,
  'task-updated': handleTaskUpdated,
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
