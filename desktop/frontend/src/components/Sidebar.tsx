/**
 * Sidebar — 左侧导航栏（对齐设计规范第7.2章）。
 *
 * 布局：
 *   SidebarHeader (40px) — 汉堡 + Tab 切换 + 收起按钮
 *   Tab Content Area — 会话列表 / 项目树 / 文件浏览
 *   底部 — 设置入口
 */

import React, { useState, useMemo } from 'react'
import { useChatStore } from '@/stores/chat'
import { ThreadItem } from './ThreadItem'
import { ProjectTree } from './ProjectTree'
import { deleteThread, getThread } from '@/lib/backend'
import { Search, Settings, FolderTree, FileText, MessageSquare, PanelLeftClose } from 'lucide-react'

type SidebarTab = 'threads' | 'project' | 'files'

interface SidebarProps {
  onNewChat: () => void
  onSettings: () => void
}

export const Sidebar: React.FC<SidebarProps> = ({ onNewChat: _onNewChat, onSettings }) => {
  const threads = useChatStore((s) => s.threads)
  const threadId = useChatStore((s) => s.threadId)
  const [activeTab, setActiveTab] = useState<SidebarTab>('threads')
  const [searchQuery, setSearchQuery] = useState('')

  const filtered = useMemo(() => {
    if (!searchQuery.trim()) return threads
    const q = searchQuery.toLowerCase()
    return threads.filter((t) => t.title?.toLowerCase().includes(q))
  }, [threads, searchQuery])

  const handleSelect = async (key: string) => {
    useChatStore.setState({ threadId: key })
    try {
      const snapshot = await getThread(key)
      if (snapshot?.messages) {
        useChatStore.setState({
          messages: snapshot.messages as any[],
        })
      }
    } catch {
      // 后端未就绪时静默失败
    }
  }

  const handleDelete = async (key: string) => {
    try {
      await deleteThread(key)
      const store = useChatStore.getState()
      store.setThreads(store.threads.filter((t) => t.key !== key))
    } catch {
      // 静默失败
    }
  }

  const tabs: { id: SidebarTab; label: string; icon: React.ReactNode }[] = [
    { id: 'threads', label: '会话', icon: <MessageSquare size={14} /> },
    { id: 'project', label: '项目', icon: <FolderTree size={14} /> },
    { id: 'files', label: '文件', icon: <FileText size={14} /> },
  ]

  return (
    <aside className="w-[var(--mady-sidebar-width)] h-full flex flex-col mady-material border-r border-mady-separator select-none">
      {/* SidebarHeader: 40px — 对齐规范 §7.2.2 */}
      <div className="h-10 flex items-center justify-between px-3 border-b border-mady-separator">
        {/* Tab 切换按钮组 */}
        <div className="flex items-center gap-0.5">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-mady-small font-medium transition-colors duration-150 ${
                activeTab === tab.id
                  ? 'bg-mady-bg-hover text-mady-text-primary'
                  : 'text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover/50'
              }`}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>

        {/* 收起按钮 */}
        <button
          className="p-1 rounded-md text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors duration-150"
          title="收起侧栏"
        >
          <PanelLeftClose size={14} />
        </button>
      </div>

      {/* Tab: 会话列表 */}
      {activeTab === 'threads' && (
        <>
          {/* 搜索框 */}
          <div className="p-3 pb-0">
            <div className="relative">
              <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-mady-text-tertiary" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索会话…"
                className="w-full pl-7 pr-3 py-1.5 rounded-md bg-mady-bg-primary border border-mady-border text-mady-ui text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-1 focus:ring-mady-accent/30 transition-all duration-150"
              />
            </div>
          </div>

          {/* 会话列表 */}
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
                />
              ))
            )}
          </nav>
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

      {/* 底部：设置入口 */}
      <div className="p-2 border-t border-mady-separator">
        <button
          onClick={onSettings}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-md text-mady-text-secondary text-mady-ui hover:bg-mady-bg-hover transition-colors duration-150"
        >
          <Settings size={14} />
          设置
        </button>
      </div>
    </aside>
  )
}
