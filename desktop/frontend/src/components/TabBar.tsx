/**
 * TabBar — 会话标签栏（阶段 2.1c）。
 *
 * 多标签并行会话入口：标签列表来自 Go 侧 tabStore（ListTabs），
 * 支持新建 / 切换 / 关闭。激活标签变化时由 ChatView 同步聊天上下文。
 */

import React from 'react'
import { useTabsStore } from '@/stores/tabs'
import { Plus, X } from 'lucide-react'

export const TabBar: React.FC = () => {
  const tabs = useTabsStore((s) => s.tabs)
  const activeTabId = useTabsStore((s) => s.activeTabId)
  const createTab = useTabsStore((s) => s.createTab)
  const closeTab = useTabsStore((s) => s.closeTab)
  const activateTab = useTabsStore((s) => s.activateTab)

  const handleCreate = async () => {
    await createTab()
  }

  const handleClose = (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    void closeTab(id)
  }

  return (
    <div className="flex items-center gap-1 overflow-x-auto no-scrollbar">
      {tabs.map((tab) => {
        const active = tab.id === activeTabId
        return (
          <div
            key={tab.id}
            role="tab"
            aria-selected={active}
            className={`
              group flex items-center gap-1.5 max-w-[180px] pl-3 pr-1.5 h-7 rounded-lg text-mady-small
              border transition-colors duration-150 cursor-pointer select-none
              ${active
                ? 'bg-mady-bg-secondary text-mady-text-primary border-mady-border'
                : 'text-mady-text-secondary border-transparent hover:bg-mady-bg-hover/60 hover:text-mady-text-primary'}
            `}
            onClick={() => void activateTab(tab.id)}
          >
            <span className="truncate">{tab.title}</span>
            <button
              onClick={(e) => handleClose(e, tab.id)}
              title="关闭标签"
              aria-label={`关闭 ${tab.title}`}
              className="shrink-0 p-0.5 rounded text-mady-text-tertiary opacity-0 group-hover:opacity-100 hover:text-mady-danger hover:bg-mady-bg-hover transition-opacity duration-150"
            >
              <X size={11} />
            </button>
          </div>
        )
      })}

      <button
        onClick={handleCreate}
        title="新建标签"
        aria-label="新建标签"
        className="shrink-0 p-1 rounded-md text-mady-text-tertiary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors duration-150"
      >
        <Plus size={13} />
      </button>
    </div>
  )
}
