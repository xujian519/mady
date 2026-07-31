/**
 * 待办任务 slice（TasksSlice）。
 *
 * 管理来自 agentcore/tasklist 的任务列表（agui 事件驱动），
 * 供底部 TodoDock 渲染。与 chatSlice / threadsSlice 组合为
 * stores/chat.ts 的 AppState。
 */
import type { StateCreator } from 'zustand'
import type { AppState } from '../chat'
import type { SliceState } from './types'

/** 任务项（来自 agentcore/tasklist）。 */
export interface TaskItem {
  id: string
  subject: string
  status: 'pending' | 'in_progress' | 'completed'
  priority: 'low' | 'normal' | 'high' | 'urgent'
  activeForm?: string
}

/** 待办任务 slice 状态与动作。 */
export interface TasksSlice {
  /** 渲染中的待办任务列表（来自 agentcore/tasklist）。 */
  tasks: TaskItem[]
  /** 添加或更新待办任务（来自 agentcore/tasklist 事件）。 */
  upsertTask: (task: TaskItem) => void
  /** 批量设置任务列表（用于初始加载）。 */
  setTasks: (tasks: TaskItem[]) => void
  /** 清除所有任务（会话切换时）。 */
  clearTasks: () => void
}

/** 待办任务默认值。 */
export const tasksInitialState = {
  tasks: [],
} satisfies SliceState<TasksSlice>

/** 创建 Tasks slice。 */
export const createTasksSlice: StateCreator<AppState, [], [], TasksSlice> = (set) => ({
  ...tasksInitialState,

  upsertTask: (task) =>
    set((s) => {
      const idx = s.tasks.findIndex((t) => t.id === task.id)
      if (idx >= 0) {
        const updated = [...s.tasks]
        updated[idx] = { ...updated[idx], ...task }
        return { tasks: updated }
      }
      return { tasks: [...s.tasks, task] }
    }),

  setTasks: (tasks) => set({ tasks }),

  clearTasks: () => set({ tasks: [] }),
})
