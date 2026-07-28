/**
 * ThreadItem — 会话列表单项。
 *
 * 展示会话标题、消息数、更新时间。
 * 点击选中，右键菜单预留。
 */

import React from 'react'
import type { ThreadSummary } from '@/stores/chat'
import { MessageSquare, Trash2 } from 'lucide-react'

interface ThreadItemProps {
  thread: ThreadSummary
  active?: boolean
  onClick: () => void
  onDelete?: () => void
}

export const ThreadItem: React.FC<ThreadItemProps> = ({
  thread,
  active = false,
  onClick,
  onDelete,
}) => {
  return (
    <button
      onClick={onClick}
      className={`
        w-full text-left px-3 py-2 rounded-lg text-mady-ui transition-colors duration-100
        flex items-center gap-2 group
        ${active
          ? 'bg-mady-accent-soft text-mady-accent'
          : 'text-mady-text-primary hover:bg-mady-bg-secondary'
        }
      `}
    >
      <MessageSquare size={14} className="shrink-0 text-mady-text-tertiary" />
      <span className="truncate flex-1">{thread.title || '新会话'}</span>
      <span className="text-mady-text-tertiary text-mady-caption shrink-0">
        {thread.messageN}
      </span>
      {onDelete && (
        <button
          onClick={(e) => {
            e.stopPropagation()
            onDelete()
          }}
          className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 rounded hover:bg-mady-danger/10 hover:text-mady-danger"
          title="删除会话"
        >
          <Trash2 size={12} />
        </button>
      )}
    </button>
  )
}
