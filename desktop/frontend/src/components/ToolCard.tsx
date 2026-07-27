/**
 * ToolCard — 工具调用卡片。
 *
 * 在消息流中渲染工具调用状态。
 * 遵循 Invisible Handoff 契约：不渲染 handoff 工具。
 *
 * 过滤逻辑（双层防护）：
 *   1. 优先检查 toolCall.invisible 字段（Go 端 AGUI 事件携带）
 *   2. 其次检查名称前缀黑名单（防御性 fallback，与 Go handoff.go 同步）
 *
 * 状态流：
 *   running → completed / error
 *
 * 收展行为：
 *   - 默认展开（显示参数和结果）
 *   - 完成后可折叠
 */

import React, { useState } from 'react'
import type { ToolCall } from '@/stores/chat'
import { Wrench, CheckCircle, XCircle, Loader2, ChevronDown, ChevronRight } from 'lucide-react'

// ── Handoff 工具名称前缀（Invisible Handoff 红线） ──
// 与 Go 端 handoff.go 的 isHandoffAllowed 保持同步。
// 新增 handoff 工具时须同时更新此前缀列表。
const HANDOFF_PREFIXES = ['transfer_to_', 'handoff_to_']

/** 判断是否为 handoff 工具。双层防护：优先 invisible 字段，其次前缀匹配。 */
function isHandoffTool(tc: ToolCall): boolean {
  if (tc.invisible) return true
  return HANDOFF_PREFIXES.some((p) => tc.name.startsWith(p))
}

// ── Component ─────────────────────────────────────

interface ToolCardProps {
  toolCall: ToolCall
}

export const ToolCard: React.FC<ToolCardProps> = ({ toolCall }) => {
  const [expanded, setExpanded] = useState(true)

  // Invisible Handoff: 过滤 handoff 工具
  if (isHandoffTool(toolCall)) {
    return null
  }

  const isRunning = toolCall.status === 'running'
  const isError = toolCall.status === 'error'
  const isDone = toolCall.status === 'completed'

  const statusIcon = isRunning
    ? <Loader2 size={14} className="animate-spin text-mady-accent" />
    : isError
      ? <XCircle size={14} className="text-mady-danger" />
      : <CheckCircle size={14} className="text-mady-success" />

  const statusBg = isRunning
    ? 'border-mady-accent/30 bg-mady-accent-soft/50'
    : isError
      ? 'border-mady-danger/30 bg-mady-danger/5'
      : 'border-mady-border bg-mady-bg-secondary'

  return (
    <div
      className={`
        rounded-xl border px-3 py-2.5 transition-colors duration-200
        max-w-[75%] ${statusBg}
      `}
    >
      {/* 头部：图标 + 工具名 + 状态 + 折叠按钮 */}
      <div className="flex items-center gap-2">
        <Wrench size={13} className="text-mady-text-secondary shrink-0" />

        <span className="text-mady-ui font-mono text-mady-text-primary truncate flex-1">
          {toolCall.name}
        </span>

        {statusIcon}

        {(isDone || isError) && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="p-0.5 rounded hover:bg-mady-bg-primary text-mady-text-tertiary"
          >
            {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          </button>
        )}
      </div>

      {/* 展开内容：参数 + 结果 */}
      {expanded && (
        <div className="mt-2 space-y-1.5 text-mady-small">
          {toolCall.args && (
            <div>
              <span className="text-mady-text-tertiary text-mady-caption">参数</span>
              <pre className="mt-0.5 bg-mady-bg-primary/50 rounded-lg p-2 font-mono text-mady-text-secondary whitespace-pre-wrap break-words">
                {toolCall.args}
              </pre>
            </div>
          )}

          {(isDone && toolCall.result) && (
            <div>
              <span className="text-mady-text-tertiary text-mady-caption">结果</span>
              <pre className="mt-0.5 bg-mady-bg-primary/50 rounded-lg p-2 font-mono text-mady-text-secondary whitespace-pre-wrap break-words">
                {toolCall.result}
              </pre>
            </div>
          )}

          {isError && (
            <div className="text-mady-danger text-mady-ui">
              {toolCall.result || '工具调用失败'}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
