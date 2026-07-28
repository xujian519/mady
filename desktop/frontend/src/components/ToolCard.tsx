/**
 * ToolCard — 工具调用卡片（像素级对齐设计规范 C04）。
 *
 * 规范：
 *   圆角 8px（radius-lg），标题行 36px
 *   状态流：pending → spinner → 品牌紫 → done/fail
 *   展开/收起动画：height 250ms spring + chevron 旋转 150ms ease
 *   内容区最大 300px，超出可滚动
 *
 * 遵循 Invisible Handoff 契约：不渲染 handoff 工具。
 */

import React, { useState } from 'react'
import type { ToolCall } from '@/stores/chat'
import { Wrench, CheckCircle, XCircle, Loader2, ChevronRight } from 'lucide-react'

// ── Handoff 工具名称前缀（Invisible Handoff 红线） ──
const HANDOFF_PREFIXES = ['transfer_to_', 'handoff_to_']

/** 判断是否为 handoff 工具。双层防护：优先 invisible 字段，其次前缀匹配。 */
export function isHandoffTool(tc: ToolCall): boolean {
  if (tc.invisible) return true
  return HANDOFF_PREFIXES.some((p) => tc.name.startsWith(p))
}

// ── Component ─────────────────────────────────────

interface ToolCardProps {
  toolCall: ToolCall
}

export const ToolCard: React.FC<ToolCardProps> = ({ toolCall }) => {
  const [expanded, setExpanded] = useState(true)

  // Invisible Handoff
  if (isHandoffTool(toolCall)) return null

  const isRunning = toolCall.status === 'running'
  const isError = toolCall.status === 'error'
  const isDone = toolCall.status === 'completed'

  const statusIcon = isRunning
    ? <Loader2 size={14} className="animate-spin text-mady-accent" />
    : isError
      ? <XCircle size={14} className="text-mady-danger" />
      : <CheckCircle size={14} className="text-mady-success" />

  const cardBorder = isRunning
    ? 'border-mady-accent/30'
    : isError
      ? 'border-mady-danger/30'
      : 'border-mady-border'

  const cardBg = isRunning
    ? 'bg-mady-accent-soft/50'
    : isError
      ? 'bg-mady-danger/5'
      : 'bg-mady-bg-secondary'

  return (
    <div
      className={`
        rounded-lg border max-w-[85%] ${cardBorder} ${cardBg}
        transition-colors duration-150
      `}
    >
      {/* 头部：标题行（固定 36px） */}
      <div
        className="flex items-center gap-2 h-9 px-2.5 cursor-pointer select-none"
        onClick={() => (isDone || isError) && setExpanded(!expanded)}
      >
        <Wrench size={14} className="text-mady-text-secondary shrink-0" />

        <span className="text-mady-small font-mono font-medium text-mady-text-primary truncate flex-1">
          {toolCall.name}
        </span>

        {/* 状态指示器 */}
        <div className="flex items-center gap-1">
          {statusIcon}
          {(isDone || isError) && (
            <button
              onClick={(e) => { e.stopPropagation(); setExpanded(!expanded) }}
              className="p-0.5 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors duration-150"
              aria-label={expanded ? '收起' : '展开'}
            >
              <div
                className="transition-transform duration-150 ease-in-out"
                style={{ transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)' }}
              >
                <ChevronRight size={12} />
              </div>
            </button>
          )}
        </div>
      </div>

      {/* 展开内容区 */}
      {expanded && (
        <div className="border-t border-mady-border/50 px-3 py-2 space-y-2 max-h-[300px] overflow-y-auto">
          {toolCall.args && (
            <div className="text-mady-small">
              <span className="text-mady-text-tertiary text-mady-caption">参数</span>
              <pre className="mt-0.5 bg-mady-bg-primary/50 rounded-md p-2 font-mono text-mady-text-secondary whitespace-pre-wrap break-words border border-mady-border/30">
                {toolCall.args}
              </pre>
            </div>
          )}

          {isDone && toolCall.result && (
            <div className="text-mady-small">
              <span className="text-mady-text-tertiary text-mady-caption">结果</span>
              <pre className="mt-0.5 bg-mady-bg-primary/50 rounded-md p-2 font-mono text-mady-text-secondary whitespace-pre-wrap break-words border border-mady-border/30">
                {toolCall.result}
              </pre>
            </div>
          )}

          {isError && (
            <div className="flex items-center gap-1.5 text-mady-danger text-mady-ui">
              <XCircle size={12} />
              <span>{toolCall.result || '工具调用失败'}</span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
