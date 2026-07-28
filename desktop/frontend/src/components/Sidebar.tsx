/**
 * Sidebar — 左侧导航栏。
 *
 * 功能：
 * 1. 搜索/过滤会话
 * 2. 新建会话按钮
 * 3. 会话列表（从 chatStore.threads 读取）
 * 4. 会话删除
 * 5. 设置入口
 */

import React, { useState, useMemo } from 'react'
import { useChatStore } from '@/stores/chat'
import { ThreadItem } from './ThreadItem'
import { ProjectTree } from './ProjectTree'
import { deleteThread } from '@/lib/backend'
import { Plus, Search, Settings, FolderTree } from 'lucide-react'

interface SidebarProps {
  onNewChat: () => void
  onSettings: () => void
}

export const Sidebar: React.FC<SidebarProps> = ({ onNewChat, onSettings }) => {
  const threads = useChatStore((s) => s.threads)
  const threadId = useChatStore((s) => s.threadId)
  const [searchQuery, setSearchQuery] = useState('')
  const [showProjectTree, setShowProjectTree] = useState(false)

  const filtered = useMemo(() => {
    if (!searchQuery.trim()) return threads
    const q = searchQuery.toLowerCase()
    return threads.filter((t) => t.title?.toLowerCase().includes(q))
  }, [threads, searchQuery])

  const handleSelect = async (key: string) => {
    useChatStore.setState({ threadId: key })
    // TODO: 加载会话消息
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

  return (
    <aside className="w-[var(--mady-sidebar-width)] h-full flex flex-col mady-material border-r border-mady-separator select-none">
      {/* 顶部：新建 + 搜索 + 项目树切换 */}
      <div className="p-3 space-y-2 border-b border-mady-separator">
        <button
          onClick={onNewChat}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-lg bg-mady-accent text-white text-mady-ui font-medium hover:bg-mady-accent-hover transition-colors"
        >
          <Plus size={14} />
          新对话
        </button>

        <div className="relative">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-mady-text-tertiary" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="搜索会话…"
            className="w-full pl-7 pr-3 py-1.5 rounded-lg bg-mady-bg-primary border border-mady-border text-mady-ui text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent transition-colors"
          />
        </div>

        <button
          onClick={() => setShowProjectTree(!showProjectTree)}
          className={`w-full flex items-center gap-2 px-3 py-1.5 rounded-lg text-mady-ui transition-colors ${
            showProjectTree
              ? 'bg-mady-accent-soft text-mady-accent'
              : 'text-mady-text-secondary hover:bg-mady-bg-primary'
          }`}
        >
          <FolderTree size={14} />
          项目文件
        </button>
      </div>

      {/* 项目树 */}
      {showProjectTree && (
        <div className="border-b border-mady-separator max-h-48 overflow-y-auto">
          <ProjectTree />
        </div>
      )}

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

      {/* 底部：设置入口 */}
      <div className="p-2 border-t border-mady-separator">
        <button
          onClick={onSettings}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-mady-text-secondary text-mady-ui hover:bg-mady-bg-primary transition-colors"
        >
          <Settings size={14} />
          设置
        </button>
      </div>
    </aside>
  )
}
