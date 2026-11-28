/**
 * ChatView 虚拟列表的纯逻辑：历史消息映射、步骤名/Token 格式化、
 * 行数与高度估算、活动内容判定。
 *
 * 从 ChatView.tsx（M56）拆出：无 React/pdf.js 依赖，可直接在 jsdom 下单测。
 */

import type { Message, ToolCall, CompactionNotice, RetryNotice } from '@/stores/chat'

/**
 * 将 Go 侧历史消息（agentcore.Message：role=user/assistant、无 timestamp 字段）
 * 映射为前端 Message（role 归一 + 时间戳缺省为 0），避免渲染出 NaN:NaN 与角色错位（M-14）。
 */
export function mapHistoryMessage(m: Record<string, unknown>, index: number): Message {
  // B-9：fallback id 用下标而非随机值——随机 id 会让同一条历史每次加载都重挂载
  // 消息气泡（滚动位置/折叠状态丢失）。
  const id = typeof m.id === 'string' && m.id ? m.id : `hist-${index}`
  return {
    id,
    role: m.role === 'user' ? 'user' : 'agent',
    content: typeof m.content === 'string' ? m.content : '',
    timestamp: typeof m.timestamp === 'number' && Number.isFinite(m.timestamp) ? m.timestamp : 0,
    thinking: typeof m.thinking === 'string' ? m.thinking : undefined,
  }
}

// ── 虚拟列表项类型 ────────────────────────────────

/** 步骤名美化：turn_N → 推理第 N 步。 */
export function formatStepName(step: string): string {
  const m = /^turn_(\d+)$/.exec(step)
  if (m) return `推理与工具调用进行中`
  return step
}

/** Token 数量格式化：12500 → 12.5k。 */
export function formatTokens(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

export type TranscriptItem =
  | { kind: 'message'; message: Message; index: number }
  // 阶段 2.4：回合组头（可折叠）。startIndex 为回合首条消息在 messages 中的索引，
  // roundId 为该回合首条消息 id，作为折叠状态的身份标识（索引漂移时状态不串位）。
  | { kind: 'round-header'; roundId: string; startIndex: number; count: number; collapsed: boolean }
  | { kind: 'streaming-output'; output: string }
  | { kind: 'streaming-thinking'; thinking: string }
  | { kind: 'tool-calls'; toolCalls: ToolCall[] }
  | { kind: 'step-indicator'; step: string; count: number }
  | { kind: 'retry-notice'; notice: RetryNotice }
  | { kind: 'truncation-notice'; notice: string }
  | { kind: 'compaction-notice'; notice: CompactionNotice }
  | { kind: 'error'; error: string }

/** 根据消息内容估算行数，用于动态高度预估。 */
export function estimateLines(text: string): number {
  if (!text) return 1
  // 按 \n + 按 80 字符折行
  const newlines = text.split('\n').length
  const wrapLines = Math.ceil(text.length / 80)
  return Math.max(newlines, wrapLines)
}

/** 单项预估高度（px），用于虚拟化器的 estimateSize。 */
export function itemHeight(item: TranscriptItem): number {
  switch (item.kind) {
    case 'round-header':
      return 32
    case 'message':
      // 头像 32px + 气泡 padding ~48px + 时间戳 ~20px + 内容
      return 80 + estimateLines(item.message.content) * 22
    case 'streaming-output':
      return 60 + estimateLines(item.output) * 22
    case 'streaming-thinking':
      return Math.min(200, 40 + estimateLines(item.thinking) * 22)
    case 'tool-calls':
      return item.toolCalls.length * 80 + 24
    case 'step-indicator':
    case 'retry-notice':
    case 'truncation-notice':
    case 'compaction-notice':
      return 36
    case 'error':
      return 60
  }
}

/** 是否有活动内容（非空列表）。 */
export function hasActiveContent(
  messages: Message[], output: string, thinking: string,
  toolCalls: ToolCall[], error: string | null,
): boolean {
  return messages.length > 0 || output.length > 0 || thinking.length > 0 || toolCalls.length > 0 || !!error
}
