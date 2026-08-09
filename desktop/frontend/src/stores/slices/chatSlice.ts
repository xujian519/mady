/**
 * Chat 运行状态 slice。
 *
 * 覆盖：应用就绪、会话运行（running/runId/threadId/error）、
 * 消息流（messages/output/thinking/toolCalls）、审批提示、
 * 上下文使用率、步骤指示、压缩与重试提示。
 *
 * 与 threadsSlice / tasksSlice 组合为 stores/chat.ts 的 AppState
 * （Zustand Slices 模式，见 mady-desktop-standards.md M-DSK-ST-005）。
 */
import type { StateCreator } from 'zustand'
import { chat as backendChat, chatInTab as backendChatInTab } from '@/lib/backend'
import { useTabsStore } from '@/stores/tabs'
import type { AppState } from '../chat'
import type { SliceState } from './types'

// ── Types ─────────────────────────────────────────

export interface ToolCall {
  id: string
  name: string
  args: string
  result?: string
  status: 'running' | 'completed' | 'error'
  /** 是否隐式交接（Invisible Handoff），前端不渲染。 */
  invisible?: boolean
}

/** 一条消息（用户或 Agent）。 */
export interface Message {
  id: string
  role: 'user' | 'agent'
  content: string
  timestamp: number
  /** Agent 消息的思考过程（结构化字段，渲染端折叠展示；F-B4 不再拼 HTML 进 content）。 */
  thinking?: string
}

/** 上下文压缩提示（agui:compaction-start/end）。 */
export interface CompactionNotice {
  /** 压缩是否正在进行中。 */
  active: boolean
  tokensBefore?: number
  tokensAfter?: number
  messagesCut?: number
  durationMs?: number
}

/** 自动重试提示（agui:auto-retry）。 */
export interface RetryNotice {
  attempt: number
  maxRetries: number
  delayMs: number
}

/** 审批提示负载（来自 agui:approval-prompt）。 */
export interface ApprovalPrompt {
  id: string
  title: string
  description?: string
  options?: ApprovalOption[]
  surfaceId?: string
}

export interface ApprovalOption {
  label: string
  value: string
}

/** Chat slice 状态与动作。 */
export interface ChatSlice {
  /** 应用是否已完成初始化并准备就绪 */
  ready: boolean
  /** 当前有没有正在运行的 chat */
  running: boolean
  /** 当前 runId（由 App.Chat 返回） */
  runId: string | null
  /** 当前会话 threadId */
  threadId: string | null
  /** 最新错误信息 */
  error: string | null
  /** 消息历史（跨轮次持久）。 */
  messages: Message[]
  /** 流式输出的已完成文本（当前轮）。 */
  output: string
  /** 当前正在累积的思考过程 */
  thinking: string
  /** 当前轮的工具调用列表 */
  toolCalls: ToolCall[]
  /** tool-call-start → tool-call-end 之间暂存的缓冲区 */
  toolCallBuffer: ToolCall | null
  /** 审批提示（T3.3）。 */
  approvalPrompt: ApprovalPrompt | null
  /** 上下文窗口使用率（0-100）。 */
  contextUsagePercent: number | null
  /** 累计 Token 使用量。 */
  contextTotalTokens: number | null
  /** 模型上下文窗口大小（Token 数）。 */
  contextWindow: number | null
  /** 当前执行的步骤名（agui:step-started，形如 turn_N）。 */
  currentStep: string | null
  /** 当前轮已开始的步骤计数。 */
  stepCount: number
  /** 上下文压缩提示（进行中或最近一次完成）。 */
  compaction: CompactionNotice | null
  /** 自动重试提示（收到后续 token 或轮次结束时清除）。 */
  retryNotice: RetryNotice | null
  /** 输出完整性提示（agui:run-finished 携带 finishReason="length"/"error" 时设置，
   *  新轮次开始时清除）。告知用户输出可能不完整。 */
  truncationNotice: string | null

  setReady: (v: boolean) => void
  setRunning: (runId: string | null) => void
  setThreadId: (id: string) => void
  setError: (err: string | null) => void
  /** 发送用户消息（乐观添加 + 调用后端）。 */
  sendMessage: (text: string) => Promise<void>
  /** 追加 Agent 流式 token。 */
  appendToken: (delta: string) => void
  appendThinking: (delta: string) => void
  addToolCall: (tc: ToolCall) => void
  updateToolCall: (id: string, tc: Partial<ToolCall>) => void
  setToolCallBuffer: (tc: ToolCall | null) => void
  /** 结束当前轮：将 output 提交为 agent message，重置运行时状态。 */
  finishTurn: () => void
  /** 设置审批提示（T3.3）。 */
  setApprovalPrompt: (p: ApprovalPrompt | null) => void
  /** 更新上下文使用率。 */
  setContextUsage: (percent: number, totalTokens: number, contextWindow: number) => void
  /** 步骤开始（进度指示器）。 */
  setStep: (name: string) => void
  /** 步骤结束（收起进度指示器）。 */
  finishStep: () => void
  /** 设置上下文压缩提示。 */
  setCompaction: (n: CompactionNotice) => void
  /** 设置/清除自动重试提示。 */
  setRetryNotice: (n: RetryNotice | null) => void
  /** 设置/清除输出完整性提示。 */
  setTruncationNotice: (n: string | null) => void
  /** 追加工具调用参数（agui:tool-call-args 流式增量）。 */
  appendToolCallArgs: (id: string, delta: string) => void
}

let msgCounter = 0
function nextMsgId(): string {
  return `msg-${Date.now()}-${++msgCounter}`
}

/** Chat slice 状态默认值（组合为 stores/chat.ts 的 initialState）。 */
export const chatInitialState = {
  ready: false,
  running: false,
  runId: null,
  threadId: null,
  error: null,
  messages: [],
  output: '',
  thinking: '',
  toolCalls: [],
  toolCallBuffer: null,
  approvalPrompt: null,
  contextUsagePercent: null,
  contextTotalTokens: null,
  contextWindow: null,
  currentStep: null,
  stepCount: 0,
  compaction: null,
  retryNotice: null,
  truncationNotice: null,
} satisfies SliceState<ChatSlice>

/** 创建 Chat slice（组合入口通过 StateCreator 注入 set/get）。 */
export const createChatSlice: StateCreator<AppState, [], [], ChatSlice> = (set, get) => ({
  ...chatInitialState,

  setReady: (v) => set({ ready: v }),

  setRunning: (runId) => set({ running: runId !== null, runId }),

  setThreadId: (id) => set({ threadId: id }),

  setError: (err) => set({ error: err }),

  sendMessage: async (text: string) => {
    const trimmed = text.trim()
    if (!trimmed) return

    const userMsg: Message = {
      id: nextMsgId(),
      role: 'user',
      content: trimmed,
      timestamp: Date.now(),
    }

    set((s) => ({
      messages: [...s.messages, userMsg],
      output: '',
      thinking: '',
      toolCalls: [],
      error: null,
      running: true,
      currentStep: null,
      stepCount: 0,
      compaction: null,
      retryNotice: null,
      truncationNotice: null,
    }))

    try {
      const { threadId } = get()
      // 阶段 2.1c：激活标签存在时消息发往该标签的会话（ChatInTab），
      // 标签未关联会话时后端自动创建并写回 tab.ThreadID。
      const activeTabId = useTabsStore.getState().activeTabId
      const runId = activeTabId
        ? await backendChatInTab(activeTabId, {
            message: trimmed,
            thread_id: threadId ?? undefined,
          })
        : await backendChat({
            message: trimmed,
            thread_id: threadId ?? undefined,
          })
      set({ runId, running: true })
      if (activeTabId) {
        // 阶段 2.1c：ChatInTab 首次发消息时后端会创建会话并写回 tab.ThreadID，
        // 刷新 tabs store 让前端标签立即带上 threadId，避免切走再切回时误判为「新标签」。
        await useTabsStore.getState().loadTabs()
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg, running: false })
    }
  },

  appendToken: (delta) =>
    set((s) => ({
      output: s.output + delta,
      // 收到正常 token 说明重试已恢复，清除重试提示
      retryNotice: null,
    })),

  appendThinking: (delta) => set((s) => ({ thinking: s.thinking + delta })),

  addToolCall: (tc) => set((s) => ({ toolCalls: [...s.toolCalls, tc] })),

  updateToolCall: (id, tc) =>
    set((s) => ({
      toolCalls: s.toolCalls.map((t) => (t.id === id ? { ...t, ...tc } : t)),
    })),

  setToolCallBuffer: (tc) => set({ toolCallBuffer: tc }),

  finishTurn: () => {
    const { output, thinking } = get()
    const agentMsgs: Message[] = []

    // F-B4：thinking 存结构化字段（渲染端用 ReasoningBlock 折叠展示），
    // content 只含正文输出，不再注入 <details> HTML。
    if (output || thinking) {
      agentMsgs.push({
        id: nextMsgId(),
        role: 'agent',
        content: output,
        thinking: thinking || undefined,
        timestamp: Date.now(),
      })
    }

    set((s) => ({
      running: false,
      runId: null,
      messages: [...s.messages, ...agentMsgs],
      output: '',
      thinking: '',
      toolCalls: [],
      toolCallBuffer: null,
      currentStep: null,
      retryNotice: null,
    }))
  },

  setApprovalPrompt: (p) => set({ approvalPrompt: p }),

  setContextUsage: (percent, totalTokens, contextWindow) =>
    set({
      contextUsagePercent: percent,
      contextTotalTokens: totalTokens,
      contextWindow,
    }),

  setStep: (name) =>
    set((s) => ({ currentStep: name, stepCount: s.stepCount + 1 })),

  finishStep: () => set({ currentStep: null }),

  setCompaction: (n) => set({ compaction: n }),

  setRetryNotice: (n) => set({ retryNotice: n }),

  setTruncationNotice: (n) => set({ truncationNotice: n }),

  appendToolCallArgs: (id, delta) =>
    set((s) => ({
      toolCalls: s.toolCalls.map((t) =>
        t.id === id ? { ...t, args: t.args + delta } : t,
      ),
    })),
})
