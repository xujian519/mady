/**
 * Composer 的会话草稿与发送历史持久化（localStorage）。
 *
 * 从 Composer.tsx（M61）拆出：纯 localStorage 读写，无 React 依赖，
 * 可直接在 jsdom 下单测；key 按会话隔离，切换会话互不干扰。
 */

/** 发送历史最大条数（阶段 2.5，↑↓ 导航） */
const MAX_SEND_HISTORY = 50

/** 草稿 localStorage key（按会话隔离）。 */
export function draftKey(threadId: string | null): string {
  return `mady-composer-draft-${threadId || 'default'}`
}

/** 发送历史 localStorage key（按会话隔离）。 */
function historyKey(threadId: string | null): string {
  return `mady-composer-history-${threadId || 'default'}`
}

/** 读取发送历史（旧 → 新，Index 0 为最早）。 */
export function loadSendHistory(threadId: string | null): string[] {
  try {
    const raw = localStorage.getItem(historyKey(threadId))
    const parsed = raw ? (JSON.parse(raw) as unknown) : []
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return []
  }
}

/** 写入发送历史（新消息在前，去重相邻，上限 MAX_SEND_HISTORY）。 */
export function saveSendHistory(threadId: string | null, history: string[]): void {
  try {
    localStorage.setItem(historyKey(threadId), JSON.stringify(history.slice(0, MAX_SEND_HISTORY)))
  } catch {
    // localStorage 不可用（隐私模式等）时静默
  }
}
