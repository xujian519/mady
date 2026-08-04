/**
 * TrashPanel — 会话回收站（阶段 2.2）。
 *
 * 展示软删除的会话（Go 侧 .trash 目录），支持：
 * - 恢复：restoreThread → 回到主会话列表
 * - 彻底删除：purgeThread（不可恢复）
 *
 * 数据真相源：TanStack Query（threadKeys.trashed），
 * 与主列表（threadKeys.all）在恢复/删除后互相失效。
 */

import React from 'react'
import { useTrashedThreads, useRestoreThread, usePurgeThread } from '@/stores/threads'
import { Trash2, RotateCcw, XCircle, MessageSquare } from 'lucide-react'
import type { ThreadSummary } from '@/stores/chat'

/** 单条回收站会话项。 */
const TrashItem: React.FC<{ thread: ThreadSummary }> = ({ thread }) => {
  const restore = useRestoreThread()
  const purge = usePurgeThread()
  const busy = restore.isPending || purge.isPending

  const title = thread.title || '未命名会话'
  const when = thread.updatedAt
    ? new Date(thread.updatedAt).toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    : ''

  return (
    <li className="group flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-mady-bg-hover/60 transition-colors duration-150">
      <div className="flex-1 min-w-0">
        <p className="text-mady-small text-mady-text-primary truncate">{title}</p>
        <p className="text-mady-caption text-mady-text-tertiary flex items-center gap-1">
          <MessageSquare size={10} />
          {when || '未知时间'}
        </p>
      </div>
      <button
        onClick={() => restore.mutate(thread.key)}
        disabled={busy}
        title="恢复会话"
        aria-label={`恢复 ${title}`}
        className="p-1 rounded text-mady-text-secondary hover:text-mady-success hover:bg-mady-bg-hover transition-colors disabled:opacity-50"
      >
        <RotateCcw size={13} />
      </button>
      <button
        onClick={() => purge.mutate(thread.key)}
        disabled={busy}
        title="彻底删除（不可恢复）"
        aria-label={`彻底删除 ${title}`}
        className="p-1 rounded text-mady-text-secondary hover:text-mady-danger hover:bg-mady-bg-hover transition-colors disabled:opacity-50"
      >
        <XCircle size={13} />
      </button>
    </li>
  )
}

export const TrashPanel: React.FC = () => {
  const { data: trashed = [] } = useTrashedThreads()

  return (
    <div className="flex flex-col h-full">
      <div className="px-3 pt-3 pb-1 flex items-center gap-2 text-mady-caption text-mady-text-tertiary">
        <Trash2 size={11} />
        <span>回收站（{trashed.length}）</span>
      </div>
      {trashed.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-mady-text-tertiary">
          <div className="text-center px-4">
            <Trash2 size={20} className="mx-auto mb-2 opacity-50" />
            <p className="text-mady-caption">回收站为空</p>
          </div>
        </div>
      ) : (
        <nav className="flex-1 overflow-y-auto p-2 space-y-0.5">
          <ul className="space-y-0.5">
            {trashed.map((t) => (
              <TrashItem key={t.key} thread={t} />
            ))}
          </ul>
        </nav>
      )}
    </div>
  )
}
