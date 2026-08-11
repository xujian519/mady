/**
 * Sidebar — 左侧导航栏（对齐设计规范第7.2章）。
 *
 * 布局：
 *   SidebarHeader (40px) — 收起按钮
 *   Tab 导航（竖向）— 会话 / 项目 / 记忆
 *   Tab Content Area — 会话列表 / 项目树 / 记忆面板
 *   底部 — 设置入口
 */

import React, { useState, useMemo } from 'react'
import { useChatStore } from '@/stores/chat'
import { useSettingsStore } from '@/stores/settings'
import { useThreads, useDeleteThread, useRenameThread } from '@/stores/threads'
import { useTabsStore } from '@/stores/tabs'
import { ThreadItem } from './ThreadItem'
import { TrashPanel } from './TrashPanel'
import { MemoryPanel } from './MemoryPanel'
import { ProjectTree } from './ProjectTree'
import { bindThreadToSession } from '@/lib/backend'
import { Search, Settings, FolderTree, MessageSquare, PanelLeftClose, Trash2, Brain } from 'lucide-react'

type SidebarTab = 'threads' | 'project' | 'memory'

interface SidebarProps {
  onNewChat: () => void
  onSettings: () => void
}

export const Sidebar: React.FC<SidebarProps> = ({ onNewChat: _onNewChat, onSettings }) => {
  // 会话列表真相源：TanStack Query（App.tsx 常驻挂载，F-B3）
  const { data: threadList = [] } = useThreads()
  const threads = threadList
  const deleteMutation = useDeleteThread()
  const renameMutation = useRenameThread()
  const threadId = useChatStore((s) => s.threadId)
  // 折叠状态持久化于 settings store（W4-T13），⌘B / 收起按钮 / 窄窗口均可切换
  const sidebarCollapsed = useSettingsStore((s) => s.sidebarCollapsed)
  const [activeTab, setActiveTab] = useState<SidebarTab>('threads')
  const [searchQuery, setSearchQuery] = useState('')
  // 阶段 2.2：历史面板 — 会话列表 ↔ 回收站视图切换
  const [showTrash, setShowTrash] = useState(false)

  const filtered = useMemo(() => {
    if (!searchQuery.trim()) return threads
    const q = searchQuery.toLowerCase()
    return threads.filter((t) => t.title?.toLowerCase().includes(q))
  }, [threads, searchQuery])

  const handleSelect = async (key: string) => {
    // 决策 5（标签联动）：侧栏点击会话 = 切换到绑定该会话的标签。
    // 已存在绑定该会话的标签 → 激活它；否则新建标签并绑定到该会话。
    // 历史加载统一由 ChatView 的激活标签 effect 处理（依赖含活跃标签 threadId，
    // bind 完成后自动重跑）——单一 owner、单一消息形状（B-1）。
    const tabState = useTabsStore.getState()
    const threadTitle = threads.find((t) => t.key === key)?.title ?? ''
    try {
      const existing = tabState.tabs.find((t) => t.threadId === key)
      if (existing) {
        if (existing.id !== tabState.activeTabId) {
          await tabState.activateTab(existing.id)
        }
        return
      }
      const created = await tabState.createTab()
      if (!created) return
      await bindThreadToSession(created.id, key, threadTitle)
    } catch {
      // 后端未就绪时静默失败
    }
  }

  const handleDelete = async (key: string) => {
    try {
      await deleteMutation.mutateAsync(key)
      // 删除的是当前会话时清空 threadId（S3）
      if (useChatStore.getState().threadId === key) {
        useChatStore.setState({ threadId: '' })
      }
    } catch {
      // 静默失败
    }
  }

  // icon 仅折叠态（48px）显示；展开态只渲染 label
  const tabs: { id: SidebarTab; label: string; icon: React.ReactNode }[] = [
    { id: 'threads', label: '会话', icon: <MessageSquare size={14} /> },
    { id: 'project', label: '项目', icon: <FolderTree size={14} /> },
    { id: 'memory', label: '记忆', icon: <Brain size={14} /> },
  ]

  return (
    <aside className={`${sidebarCollapsed ? 'w-12' : 'w-[var(--mady-sidebar-width)]'} h-full flex flex-col mady-material border-r border-mady-separator select-none transition-[width] duration-150`}>
      {/* SidebarHeader: 40px — 对齐规范 §7.2.2；仅保留收起按钮（折叠态隐藏，由 ChatView 标题栏/⌘B 恢复） */}
      {!sidebarCollapsed && (
        <div className="h-10 flex items-center justify-end px-3 border-b border-mady-separator">
          <button
            onClick={() => useSettingsStore.setState({ sidebarCollapsed: true })}
            className="p-1 rounded-md text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors duration-150"
            title="收起侧栏"
          >
            <PanelLeftClose size={14} />
          </button>
        </div>
      )}

      {/* 竖向 Tab 导航 — 会话/项目/记忆 按功能竖排，位于搜索框上方 */}
      <nav className={`flex ${sidebarCollapsed ? 'flex-col items-center gap-1 py-2' : 'flex-col gap-0.5 p-2 border-b border-mady-separator'}`}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            title={tab.label}
            aria-label={tab.label}
            className={`flex items-center gap-1.5 rounded-md transition-colors duration-150 ${
              sidebarCollapsed ? 'justify-center p-1.5' : 'px-2.5 py-1.5 text-mady-small font-medium'
            } ${
              activeTab === tab.id
                ? 'bg-mady-bg-hover text-mady-text-primary'
                : 'text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover/50'
            }`}
          >
            {sidebarCollapsed ? tab.icon : tab.label}
          </button>
        ))}
      </nav>

      {/* 折叠态：仅保留图标导航，内容区隐藏 */}
      {sidebarCollapsed ? (
        <div className="flex-1" />
      ) : (
        <>
      {/* Tab: 会话列表 */}
      {activeTab === 'threads' && (
        <>
          {/* 搜索框 + 回收站切换 */}
          <div className="p-3 pb-0 flex items-center gap-2">
            <div className="relative flex-1">
              <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-mady-text-tertiary" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索会话…"
                className="w-full pl-7 pr-3 py-1.5 rounded-md bg-mady-bg-primary border border-mady-border text-mady-ui text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-1 focus:ring-mady-accent/30 transition-all duration-150"
              />
            </div>
            <button
              onClick={() => setShowTrash((v) => !v)}
              title={showTrash ? '返回会话列表' : '回收站'}
              aria-label={showTrash ? '返回会话列表' : '回收站'}
              aria-pressed={showTrash}
              className={`p-1.5 rounded-md transition-colors duration-150 ${
                showTrash ? 'text-mady-accent bg-mady-bg-hover' : 'text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover'
              }`}
            >
              <Trash2 size={14} />
            </button>
          </div>

          {showTrash ? (
            <TrashPanel />
          ) : (
            /* 会话列表 */
            <nav className="flex-1 overflow-y-auto p-2 space-y-0.5">
              {filtered.length === 0 ? (
                <p className="text-mady-text-tertiary text-mady-caption text-center pt-8">
                  {searchQuery ? '无匹配会话' : '暂无会话'}
                </p>
              ) : (
                filtered.map((t) => (
                  <ThreadItem
                    key={t.key}
                    thread={t}
                    active={t.key === threadId}
                    onClick={() => handleSelect(t.key)}
                    onDelete={() => handleDelete(t.key)}
                    onRename={(name) => renameMutation.mutate({ key: t.key, name })}
                  />
                ))
              )}
            </nav>
          )}
        </>
      )}

      {/* Tab: 项目文件树 */}
      {activeTab === 'project' && (
        <div className="flex-1 overflow-y-auto p-2">
          <ProjectTree />
        </div>
      )}

      {/* Tab: 记忆（阶段 4：MemoryPanel） */}
      {activeTab === 'memory' && (
        <MemoryPanel />
      )}
        </>
      )}

      {/* 底部：设置入口 */}
      <div className="p-2 border-t border-mady-separator">
        <button
          onClick={onSettings}
          title="设置"
          aria-label="设置"
          className="w-full flex items-center gap-2 px-3 py-2 rounded-md text-mady-text-secondary text-mady-ui hover:bg-mady-bg-hover transition-colors duration-150"
        >
          <Settings size={14} />
          {!sidebarCollapsed && '设置'}
        </button>
      </div>
    </aside>
  )
}
