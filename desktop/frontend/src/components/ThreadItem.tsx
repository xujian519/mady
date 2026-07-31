/**
 * ThreadItem — 会话列表单项。
 *
 * 展示会话标题、消息数、更新时间。
 * 点击选中；删除采用两段式确认（F-I9，M-DSK-IX-009 破坏性操作须确认）。
 *
 * 无障碍（F-I9）：外层容器用 div role="button" 承载选中语义，
 * 内层删除按钮不再「按钮套按钮」（非法嵌套交互元素）。
 */

import React, { useState } from 'react'
import type { ThreadSummary } from '@/stores/chat'
import { MessageSquare, Trash2, Check } from 'lucide-react'

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
  // 两段式删除确认：第一次点击进入确认态，第二次才真正删除
  const [confirming, setConfirming] = useState(false)

  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={thread.title || '新会话'}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick()
        }
      }}
      className={`
        w-full text-left px-3 py-2 rounded-lg text-mady-ui transition-colors duration-100
        flex items-center gap-2 group cursor-pointer select-none
        focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mady-accent/40
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
            if (!confirming) {
              setConfirming(true)
              return
            }
            onDelete()
          }}
          onBlur={() => setConfirming(false)}
          aria-label={confirming ? '确认删除会话' : '删除会话'}
          className={`opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity p-0.5 rounded ${
            confirming
              ? 'bg-mady-danger/15 text-mady-danger'
              : 'hover:bg-mady-danger/10 hover:text-mady-danger text-mady-text-tertiary'
          }`}
        >
          {confirming ? <Check size={12} /> : <Trash2 size={12} />}
        </button>
      )}
    </div>
  )
}
