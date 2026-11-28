/**
 * AGUI 事件载荷类型表（M72）。
 *
 * 每个接口对应 Go 侧 `agui/*Event` 的 JSON 形态（字段名与 `agui/types.go` 的
 * `json` 标签一致，均为可选——`omitempty` 字段在传输中可能缺失）。
 * 这是前端唯一的「事件名 → 载荷」权威映射：新增事件时同时补这里与 `reducer.ts`，
 * 否则 `AguiEventPayloads` 的键集合校验会失败。
 */

import type { Envelope } from '@/a2ui-renderer/store'
import type { ApprovalOption } from '@/stores/slices/chatSlice'

/** `agent-start`：RUN_STARTED。 */
export interface AgentStartPayload {
  threadId?: string
  runId?: string
}

/** `message-delta` / `thinking-delta`：TEXT/THINKING_MESSAGE_CONTENT。 */
export interface MessageDeltaPayload {
  delta?: string
  content?: string
}

/** `tool-call-start`：TOOL_CALL_START（Go 字段为 `toolCallName`）。 */
export interface ToolCallStartPayload {
  toolCallId?: string
  id?: string
  toolCallName?: string
  toolName?: string
  name?: string
  invisible?: boolean
}

/** `tool-call-args`：TOOL_CALL_ARGS（增量参数）。 */
export interface ToolCallArgsPayload {
  toolCallId?: string
  delta?: string
}

/** `tool-call-result`：TOOL_CALL_RESULT。 */
export interface ToolCallResultPayload {
  toolCallId?: string
  content?: string
}

/** `tool-call-end`：TOOL_CALL_END（带最终结果或错误）。 */
export interface ToolCallEndPayload {
  result?: string
  error?: unknown
}

/** `step-started`：STEP_STARTED。 */
export interface StepStartedPayload {
  stepName?: string
}

/** CustomEvent 通用外壳：`{ value: <业务载荷> }`。 */
export interface CustomValuePayload<T> {
  value?: T
}

/** `compaction-*` 的 `value`（snake_case，与 agentcore 事件一致）。 */
export interface CompactionValue {
  tokens_before?: number
  tokens_after?: number
  messages_cut?: number
  duration_ms?: number
}

/** `auto-retry` 的 `value`。 */
export interface AutoRetryValue {
  attempt?: number
  max_retries?: number
  delay_ms?: number
}

/** `error`：RUN_ERROR。 */
export interface ErrorPayload {
  error?: string
  message?: string
}

/** `done`：流式结束（非 AGUI 标准，Go 端自行 emit）。 */
export interface DonePayload {
  error?: unknown
}

/** `run-finished`：RUN_FINISHED。 */
export interface RunFinishedPayload {
  finishReason?: string
  threadId?: string
  runId?: string
  result?: string
}

/** `context-usage`：CONTEXT_USAGE。 */
export interface ContextUsagePayload {
  usagePercent?: number
  contextWindow?: number
  tokenUsage?: { totalTokens?: number }
}

/** agentcore 任务快照（`task-created` / `task-updated` 共用）。 */
export interface TaskSnapshot {
  id?: string | number
  subject?: string
  status?: string
  priority?: string
  activeForm?: string
}

/** 任务事件载荷：可能带 `task` 外壳，也可能直接把任务字段铺在顶层。 */
export type TaskPayload = TaskSnapshot & { task?: TaskSnapshot }

/** `approval-prompt`：审批提示。 */
export interface ApprovalPromptPayload {
  id?: string
  title?: string
  description?: string
  options?: ApprovalOption[]
}

/** 事件名 → 载荷类型。键集合必须与 `reducer.ts` 的 `HANDLERS` 一致。 */
export interface AguiEventPayloads {
  'agent-start': AgentStartPayload
  'message-delta': MessageDeltaPayload
  'thinking-delta': MessageDeltaPayload
  'tool-call-start': ToolCallStartPayload
  'tool-call-args': ToolCallArgsPayload
  'tool-call-end': ToolCallEndPayload
  'tool-call-result': ToolCallResultPayload
  'step-started': StepStartedPayload
  'step-finished': undefined
  'compaction-start': CustomValuePayload<CompactionValue>
  'compaction-end': CustomValuePayload<CompactionValue>
  'auto-retry': CustomValuePayload<AutoRetryValue>
  error: ErrorPayload
  a2ui: Envelope
  'approval-prompt': ApprovalPromptPayload
  done: DonePayload
  'run-finished': RunFinishedPayload
  'context-usage': ContextUsagePayload
  'task-created': TaskPayload
  'task-updated': TaskPayload
}

/** 已注册处理器的 AGUI 事件名。 */
export type AguiEventName = keyof AguiEventPayloads

/** 全部事件载荷的联合类型（用于 Wails 订阅回调的泛型参数）。 */
export type AguiEventPayload = AguiEventPayloads[AguiEventName]
