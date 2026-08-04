/**
 * ThreadItem — 会话列表单项。
 *
 * 展示会话标题、消息数、更新时间。
 * 点击选中；删除采用两段式确认（F-I9，M-DSK-IX-009 破坏性操作须确认）；
 * 重命名采用行内编辑（Pencil → 输入框，Enter 确认 / Esc 取消 / 失焦取消，
 * 2026-08-04 决策 7：补阶段 1.4「自定义标题」最后一块）。
 *
 * 无障碍（F-I9）：外层容器用 div role="button" 承载选中语义，
 * 内层删除按钮不再「按钮套按钮」（非法嵌套交互元素）。
 */

import React, { useEffect, useRef, useState } from 'react'
import type { ThreadSummary } from '@/stores/chat'
import { MessageSquare, Trash2, Check, Pencil } from 'lucide-react'

interface ThreadItemProps {
  thread: ThreadSummary
  active?: boolean
  onClick: () => void
  onDelete?: () => void
  onRename?: (name: string) => void
}

export const ThreadItem: React.FC<ThreadItemProps> = ({
  thread,
  active = false,
  onClick,
  onDelete,
  onRename,
}) => {
  // 两段式删除确认：第一次点击进入确认态，第二次才真正删除
  const [confirming, setConfirming] = useState(false)
  // 行内重命名编辑态
  const [renaming, setRenaming] = useState(false)
  const [draft, setDraft] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (renaming) inputRef.current?.focus()
  }, [renaming])

  const startRename = (e: React.MouseEvent) => {
    e.stopPropagation()
    setDraft(thread.title || '')
    setRenaming(true)
  }

  const commitRename = () => {
    // B-8：Enter 提交后 input 卸载可能触发 blur 二次提交（Chrome 行为），加守卫防重复调用
    if (!renaming) return
    const name = draft.trim()
    setRenaming(false)
    if (name && name !== thread.title && onRename) onRename(name)
  }

  const cancelRename = () => {
    setRenaming(false)
  }

  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={thread.title || '新会话'}
      onClick={renaming ? undefined : onClick}
      onKeyDown={(e) => {
        if (renaming) return
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
      {renaming ? (
        <input
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            e.stopPropagation()
            if (e.key === 'Enter') commitRename()
            else if (e.key === 'Escape') cancelRename()
          }}
          onBlur={commitRename}
          aria-label="重命名会话"
          className="flex-1 min-w-0 px-1.5 py-0.5 rounded border border-mady-accent/50 bg-mady-bg-primary text-mady-ui text-mady-text-primary outline-none focus:ring-1 focus:ring-mady-accent/40"
        />
      ) : (
        <>
          <span className="truncate flex-1">{thread.title || '新会话'}</span>
          <span className="text-mady-text-tertiary text-mady-caption shrink-0">
            {thread.messageN}
          </span>
        </>
      )}
      {onRename && !renaming && (
        <button
          onClick={startRename}
          aria-label="重命名会话"
          title="重命名"
          className="opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity p-0.5 rounded hover:bg-mady-bg-hover hover:text-mady-text-primary text-mady-text-tertiary"
        >
          <Pencil size={12} />
        </button>
      )}
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
