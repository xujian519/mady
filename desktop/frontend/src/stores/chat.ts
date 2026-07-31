/**
 * 全局 Zustand store 组合入口（AppState）。
 *
 * 按 Zusatnd Slices 模式将状态域切分为三个 slice：
 * - chatSlice   — 会话运行与消息流状态（stores/slices/chatSlice.ts）
 * - threadsSlice — 会话列表（stores/slices/threadsSlice.ts）
 * - tasksSlice  — 待办任务（stores/slices/tasksSlice.ts）
 *
 * 对外 API 保持兼容：`useChatStore` / `initialState` / 各领域类型
 * 的导入路径不变（见 mady-desktop-standards.md M-DSK-ST-005）。
 * 组件订阅建议使用 selector（useStore(s => s.x)），复合值用 useShallow。
 */
import { create } from 'zustand'
import { createChatSlice, chatInitialState, type ChatSlice } from './slices/chatSlice'
import { createThreadsSlice, threadsInitialState, type ThreadsSlice } from './slices/threadsSlice'
import { createTasksSlice, tasksInitialState, type TasksSlice } from './slices/tasksSlice'
import type { SliceState } from './slices/types'

// ── 组合类型 ──────────────────────────────────────

/** 组合后的完整 store 类型（状态 + 动作）。 */
export type AppState = ChatSlice & ThreadsSlice & TasksSlice

/** 兼容别名：旧代码中 `ChatStore` 即组合后的完整 store。 */
export type ChatStore = AppState

/** 纯状态字段类型（不含 actions），供 reset / initialState 使用。 */
export type ChatState = SliceState<AppState>

// ── 组合 store ────────────────────────────────────

/**
 * 全局 store 实例。
 *
 * 三个 slice 共享同一 set/get（StateCreator 注入），
 * 因此 slice 之间的跨域读写在组合 store 上是天然一致的。
 */
export const useChatStore = create<AppState>()((...a) => ({
  ...createChatSlice(...a),
  ...createThreadsSlice(...a),
  ...createTasksSlice(...a),
}))

/** 全部状态字段的默认值（用于重置会话状态，如 `setState({ ...initialState, ready: true })`）。 */
export const initialState: ChatState = {
  ...chatInitialState,
  ...threadsInitialState,
  ...tasksInitialState,
}

// ── 类型 re-export（对外 API 兼容） ────────────────

export type {
  Message,
  ToolCall,
  CompactionNotice,
  RetryNotice,
  ApprovalPrompt,
  ApprovalOption,
} from './slices/chatSlice'
export type { ThreadSummary } from './slices/threadsSlice'
export type { TaskItem } from './slices/tasksSlice'
