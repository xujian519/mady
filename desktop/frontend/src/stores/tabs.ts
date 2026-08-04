/**
 * Tabs store — 会话标签（阶段 2.1c）。
 *
 * 真相源在 Go 侧（desktop/tabs.go，持久化于 ~/.mady/desktop-tabs.json）；
 * 本 store 为前端缓存 + 动作分发。动作完成后重新拉取列表，
 * 保证与后端状态一致（数据量小，简单可靠）。
 */

import { create } from 'zustand'
import * as backend from '@/lib/backend'

export interface DesktopTab {
  id: string
  threadId?: string
  title: string
  createdAt: string
  activeAt: string
}

interface TabsState {
  /** 全部标签（后端顺序，首个为最近激活）。 */
  tabs: DesktopTab[]
  /** 当前激活标签 ID（"" = 无）。 */
  activeTabId: string | null
  /** 是否已从后端加载过。 */
  loaded: boolean

  loadTabs: () => Promise<void>
  createTab: () => Promise<DesktopTab | null>
  closeTab: (id: string) => Promise<void>
  activateTab: (id: string) => Promise<void>
}

export const useTabsStore = create<TabsState>()((set, get) => ({
  tabs: [],
  activeTabId: null,
  loaded: false,

  loadTabs: async () => {
    try {
      const tabs = await backend.listTabs()
      const activeTabId = await backend.activeTabId()
      set({ tabs, activeTabId: activeTabId || null, loaded: true })
    } catch {
      // 后端未就绪（启动早期）：保持空列表，由下次动作重试
    }
  },

  createTab: async () => {
    try {
      const created = await backend.createTab()
      await get().loadTabs()
      return created
    } catch {
      return null
    }
  },

  closeTab: async (id) => {
    try {
      await backend.closeTab(id)
    } catch {
      // 最后一个标签不可关闭等：保持现状
    }
    await get().loadTabs()
  },

  activateTab: async (id) => {
    try {
      await backend.activateTab(id)
      set({ activeTabId: id })
    } catch {
      // 忽略（后端启动未就绪等）
    }
  },
}))
