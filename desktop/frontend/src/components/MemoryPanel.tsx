/**
 * MemoryPanel — 记忆面板（阶段 4）。
 *
 * 展示 memory 三层系统（user/session/long_term）中的记忆：
 * - 列表：内容 + 层标签 + 更新时间 + 重要性 + 删除
 * - 搜索：语义检索（RecallMemories）
 * - 添加：手动写入长期记忆
 *
 * 数据真相源：TanStack Query（stores/memories.ts）。
 */

import React, { useState } from 'react'
import { useMemories, useMemorySearch, useRememberMemory, useForgetMemory } from '@/stores/memories'
import { Brain, Search, Plus, X, Trash2 } from 'lucide-react'
import type { MemoryEntry } from '@/lib/backend'

/** 层标签中文名。 */
const LAYER_LABEL: Record<string, string> = {
  user: '用户',
  session: '会话',
  long_term: '长期',
}

const MemoryRow: React.FC<{ entry: MemoryEntry }> = ({ entry }) => {
  const forget = useForgetMemory()
  const time = entry.updated_at
    ? new Date(entry.updated_at).toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    : ''
  return (
    <li className="group px-2 py-1.5 rounded-md hover:bg-mady-bg-hover/60 transition-colors duration-150">
      <div className="flex items-start gap-2">
        <p className="flex-1 min-w-0 text-mady-small text-mady-text-primary leading-relaxed break-words">
          {entry.content}
        </p>
        <button
          onClick={() => forget.mutate(entry.id)}
          disabled={forget.isPending}
          title="删除记忆"
          aria-label="删除记忆"
          className="shrink-0 p-1 rounded text-mady-text-tertiary opacity-0 group-hover:opacity-100 hover:text-mady-danger hover:bg-mady-bg-hover transition-all disabled:opacity-50"
        >
          <Trash2 size={12} />
        </button>
      </div>
      <div className="flex items-center gap-2 mt-0.5 text-mady-caption text-mady-text-tertiary">
        <span className="px-1 rounded bg-mady-bg-hover">{LAYER_LABEL[entry.layer] ?? entry.layer}</span>
        {entry.importance > 0 && <span>重要性 {(entry.importance * 100).toFixed(0)}%</span>}
        {time && <span>{time}</span>}
      </div>
    </li>
  )
}

export const MemoryPanel: React.FC = () => {
  const { data: memories = [] } = useMemories()
  const [query, setQuery] = useState('')
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState('')
  const remember = useRememberMemory()
  const search = useMemorySearch(query)

  const searching = query.trim().length > 0
  const results = search.data ?? []

  const handleAdd = () => {
    const content = draft.trim()
    if (!content) return
    remember.mutate(content, {
      onSuccess: () => {
        setDraft('')
        setAdding(false)
      },
    })
  }

  return (
    <div className="flex flex-col h-full">
      {/* 搜索 + 添加 */}
      <div className="p-3 pb-1 flex items-center gap-2">
        <div className="relative flex-1">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-mady-text-tertiary" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="语义搜索记忆…"
            className="w-full pl-7 pr-3 py-1.5 rounded-md bg-mady-bg-primary border border-mady-border text-mady-ui text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-1 focus:ring-mady-accent/30 transition-all duration-150"
          />
        </div>
        <button
          onClick={() => setAdding((v) => !v)}
          title={adding ? '收起' : '添加记忆'}
          aria-label={adding ? '收起' : '添加记忆'}
          className={`p-1.5 rounded-md transition-colors duration-150 ${
            adding ? 'text-mady-accent bg-mady-bg-hover' : 'text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover'
          }`}
        >
          {adding ? <X size={14} /> : <Plus size={14} />}
        </button>
      </div>

      {/* 手动添加输入 */}
      {adding && (
        <div className="px-3 pb-2">
          <div className="flex items-end gap-2">
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={2}
              placeholder="记录一条长期记忆…"
              className="flex-1 resize-none rounded-md bg-mady-bg-primary border border-mady-border px-2.5 py-1.5 text-mady-small text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent"
            />
            <button
              onClick={handleAdd}
              disabled={!draft.trim() || remember.isPending}
              className="shrink-0 px-2.5 py-1.5 rounded-md bg-mady-accent text-white text-mady-caption hover:bg-mady-accent-hover disabled:opacity-50 transition-colors"
            >
              保存
            </button>
          </div>
        </div>
      )}

      {/* 列表 / 搜索结果 */}
      <div className="px-2 pb-1 flex items-center gap-1.5 text-mady-caption text-mady-text-tertiary">
        <Brain size={11} />
        <span>
          {searching ? `检索结果（${results.length}）` : `记忆（${memories.length}）`}
        </span>
      </div>
      <nav className="flex-1 overflow-y-auto p-2 space-y-0.5">
        {(searching ? results.map((r) => r.entry) : memories).length === 0 ? (
          <p className="text-mady-text-tertiary text-mady-caption text-center pt-8">
            {searching ? '无匹配记忆' : '暂无记忆'}
          </p>
        ) : (
          <ul className="space-y-0.5">
            {(searching ? results.map((r) => r.entry) : memories).map((m) => (
              <MemoryRow key={m.id} entry={m} />
            ))}
          </ul>
        )}
      </nav>
    </div>
  )
}
