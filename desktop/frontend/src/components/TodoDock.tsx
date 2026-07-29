/**
 * TodoDock — 底部待办坞。
 *
 * 显示 agentcore/tasklist 的实时任务列表，通过 AGUI 事件驱动更新。
 * 位置：消息列表下方、AgentFooter 上方，作为可折叠底部面板。
 *
 * 交互：
 *   - 默认收起：只显示摘要行「N 个任务 · M 进行中」
 *   - 点击展开：列表视图，状态徽章 + 标题 + 进度文案
 *   - 空状态：无任务时隐藏
 */

import React, { useState } from 'react'
import { useChatStore } from '@/stores/chat'
import type { TaskItem } from '@/stores/chat'
import { CheckCircle2, Circle, ChevronUp, ChevronDown, ListTodo } from 'lucide-react'

/** 优先级对应的图标色和排序值。 */
const PRIORITY_CONFIG: Record<string, { className: string; order: number }> = {
  urgent:    { className: 'text-mady-danger', order: 4 },
  high:      { className: 'text-mady-warning', order: 3 },
  normal:    { className: 'text-mady-text-tertiary', order: 2 },
  low:       { className: 'text-mady-text-quaternary', order: 1 },
}

const STATUS_LABEL: Record<string, { label: string; icon: React.ReactNode }> = {
  pending:     { label: '待处理', icon: <Circle size={10} className="text-mady-text-tertiary" /> },
  in_progress: { label: '进行中', icon: <div className="w-2.5 h-2.5 rounded-full bg-mady-accent animate-pulse" /> },
  completed:   { label: '完成',   icon: <CheckCircle2 size={10} className="text-mady-success" /> },
}

export const TodoDock: React.FC = () => {
  const tasks = useChatStore((s) => s.tasks)
  const [expanded, setExpanded] = useState(false)

  // 空状态：无任务时隐藏
  if (tasks.length === 0) return null

  const inProgress = tasks.filter((t) => t.status === 'in_progress').length
  const completed = tasks.filter((t) => t.status === 'completed').length

  // 排序：urgency > in_progress > pending > completed
  const sorted = [...tasks].sort((a, b) => {
    const statusOrder = (s: string) => (s === 'in_progress' ? 3 : s === 'pending' ? 2 : s === 'completed' ? 1 : 0)
    const pa = PRIORITY_CONFIG[a.priority]?.order ?? 0
    const pb = PRIORITY_CONFIG[b.priority]?.order ?? 0
    if (pa !== pb) return pb - pa
    return statusOrder(b.status) - statusOrder(a.status)
  })

  return (
    <div className="border-t border-mady-separator bg-mady-bg-secondary/80">
      {/* 折叠摘要行 */}
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-4 h-7 text-mady-caption text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors duration-100 select-none"
      >
        <ListTodo size={11} className="shrink-0 text-mady-accent" />
        <span className="font-medium">{tasks.length}</span>
        <span>个任务</span>
        {inProgress > 0 && (
          <>
            <span className="text-mady-text-tertiary">·</span>
            <span className="text-mady-accent">{inProgress}</span>
            <span>进行中</span>
          </>
        )}
        {completed > 0 && (
          <>
            <span className="text-mady-text-tertiary">·</span>
            <span className="text-mady-success">{completed}</span>
            <span>已完成</span>
          </>
        )}
        <span className="ml-auto">
          {expanded ? <ChevronDown size={11} /> : <ChevronUp size={11} />}
        </span>
      </button>

      {/* 展开列表 */}
      {expanded && (
        <div className="max-h-[200px] overflow-y-auto border-t border-mady-separator/50">
          {sorted.map((task) => (
            <TodoRow key={task.id} task={task} />
          ))}
        </div>
      )}
    </div>
  )
}

/** 单行任务渲染。 */
const TodoRow: React.FC<{ task: TaskItem }> = ({ task }) => {
  const statusCfg = STATUS_LABEL[task.status] ?? STATUS_LABEL.pending
  const priorityCfg = PRIORITY_CONFIG[task.priority]

  return (
    <div className="flex items-center gap-2 px-4 h-8 text-mady-caption hover:bg-mady-bg-hover transition-colors duration-75">
      {/* 状态图标 */}
      <span className="shrink-0">{statusCfg.icon}</span>

      {/* 优先级指示点 */}
      {priorityCfg && (
        <span className={`shrink-0 w-1.5 h-1.5 rounded-full ${priorityCfg.className}`} />
      )}

      {/* 标题 */}
      <span className="flex-1 min-w-0 truncate text-mady-text-primary">
        {task.activeForm || task.subject}
      </span>

      {/* 状态标签 */}
      <span className="shrink-0 text-mady-text-tertiary">{statusCfg.label}</span>
    </div>
  )
}
