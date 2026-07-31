/**
 * 会话列表 slice（ThreadsSlice）。
 *
 * 管理侧栏展示的会话列表（来自 App.ListThreads）。
 * 与 chatSlice / tasksSlice 组合为 stores/chat.ts 的 AppState。
 */
import type { StateCreator } from 'zustand'
import type { AppState } from '../chat'
import type { SliceState } from './types'

/** 会话摘要（供侧栏渲染）。 */
export interface ThreadSummary {
  key: string
  title: string
  updatedAt: string
  messageN: number
}

/** 会话列表 slice 状态与动作。 */
export interface ThreadsSlice {
  /** 会话列表（供侧栏使用） */
  threads: ThreadSummary[]
  setThreads: (threads: ThreadSummary[]) => void
}

/** 会话列表默认值。 */
export const threadsInitialState = {
  threads: [],
} satisfies SliceState<ThreadsSlice>

/** 创建 Threads slice。 */
export const createThreadsSlice: StateCreator<AppState, [], [], ThreadsSlice> = (set) => ({
  ...threadsInitialState,

  setThreads: (threads) => set({ threads }),
})
