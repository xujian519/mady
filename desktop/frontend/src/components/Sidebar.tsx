/**
 * Sidebar — 左侧导航栏（对齐设计规范第7.2章）。
 *
 * 布局：
 *   SidebarHeader (40px) — 汉堡 + Tab 切换 + 收起按钮
 *   Tab Content Area — 会话列表 / 项目树 / 文件浏览
 *   底部 — 设置入口
 */

import React, { useState, useMemo, useRef } from 'react'
import { useChatStore } from '@/stores/chat'
import { useSettingsStore } from '@/stores/settings'
import { useThreads, useDeleteThread, useRenameThread } from '@/stores/threads'
import { useTabsStore } from '@/stores/tabs'
import { ThreadItem } from './ThreadItem'
import { TrashPanel } from './TrashPanel'
import { MemoryPanel } from './MemoryPanel'
import { ProjectTree } from './ProjectTree'
import { getThread, bindThreadToSession } from '@/lib/backend'
import { Search, Settings, FolderTree, FileText, MessageSquare, PanelLeftClose, Trash2, Brain } from 'lucide-react'

type SidebarTab = 'threads' | 'project' | 'files' | 'memory'

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
  // 会话切换竞态守卫（S3）：快速切换 A→B 时丢弃 A 的过期响应
  const selectReqRef = useRef(0)

  const filtered = useMemo(() => {
    if (!searchQuery.trim()) return threads
    const q = searchQuery.toLowerCase()
    return threads.filter((t) => t.title?.toLowerCase().includes(q))
  }, [threads, searchQuery])

  const handleSelect = async (key: string) => {
    // 决策 5（标签联动）：侧栏点击会话 = 切换到绑定该会话的标签。
    // 已存在绑定该会话的标签 → 激活它；否则新建标签并绑定到该会话。
    // 历史加载由 ChatView 的激活标签 effect 统一处理（避免双真相源撕裂）。
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
      await tabState.loadTabs()
      // 新标签绑定完成：主动加载该会话历史（ChatView effect 可能已在 bind 前
      // 按空 threadId 走了清空分支，这里补一次加载保证最终一致）。
      const reqId = ++selectReqRef.current
      try {
        const snapshot = await getThread(key)
        if (reqId !== selectReqRef.current) return // 已切换到其他会话，丢弃过期响应
        if (snapshot?.messages) {
          useChatStore.setState({
            threadId: key,
            messages: snapshot.messages as any[],
          })
        }
      } catch {
        /* 后端未就绪时静默失败 */
      }
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

  const tabs: { id: SidebarTab; label: string; icon: React.ReactNode }[] = [
    { id: 'threads', label: '会话', icon: <MessageSquare size={14} /> },
    { id: 'project', label: '项目', icon: <FolderTree size={14} /> },
    { id: 'files', label: '文件', icon: <FileText size={14} /> },
    { id: 'memory', label: '记忆', icon: <Brain size={14} /> },
  ]

  return (
    <aside className={`${sidebarCollapsed ? 'w-12' : 'w-[var(--mady-sidebar-width)]'} h-full flex flex-col mady-material border-r border-mady-separator select-none transition-[width] duration-150`}>
      {/* SidebarHeader: 40px — 对齐规范 §7.2.2；折叠态仅显示 Tab 图标 */}
      <div className="h-10 flex items-center justify-between px-3 border-b border-mady-separator">
        {/* Tab 切换按钮组 */}
        <div className={`flex ${sidebarCollapsed ? 'flex-col items-center gap-1 w-full' : 'items-center gap-0.5'}`}>
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              title={tab.label}
              aria-label={tab.label}
              className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-mady-small font-medium transition-colors duration-150 ${
                activeTab === tab.id
                  ? 'bg-mady-bg-hover text-mady-text-primary'
                  : 'text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover/50'
              }`}
            >
              {tab.icon}
              {!sidebarCollapsed && tab.label}
            </button>
          ))}
        </div>

        {/* 收起按钮（W4-T13：折叠状态持久化到 settings store）——折叠态时隐藏，由 ChatView 标题栏展开按钮恢复 */}
        {!sidebarCollapsed && (
          <button
            onClick={() => useSettingsStore.setState({ sidebarCollapsed: true })}
            className="p-1 rounded-md text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors duration-150"
            title="收起侧栏"
          >
            <PanelLeftClose size={14} />
          </button>
        )}
      </div>

      {/* 折叠态：仅保留图标 Tab 切换，内容区隐藏 */}
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

      {/* Tab: 文件浏览（预留） */}
      {activeTab === 'files' && (
        <div className="flex-1 flex items-center justify-center text-mady-text-tertiary">
          <div className="text-center px-4">
            <FileText size={24} className="mx-auto mb-2 opacity-50" />
            <p className="text-mady-caption">文件浏览</p>
          </div>
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
