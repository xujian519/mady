/**
 * MessageBubble — 消息气泡组件（像素级对齐设计规范 C02/C03）。
 *
 * UserBubble (C02) 规范：
 *   不对称圆角 12px 4px 12px 12px，max-width 85%，右对齐
 *   背景 rgba(88,86,214,0.12) → --color-mady-bg-bubble-user
 *
 * AgentBlock (C03) 规范：
 *   不对称圆角 12px 12px 12px 4px，max-width 92%，左对齐
 *   流式时左侧 2px 品牌紫边框 + 光标 step 闪烁 1s
 */

import React from 'react'
import { motion } from 'framer-motion'
import type { Message } from '@/stores/chat'
import { MarkdownRenderer } from './MarkdownRenderer'

interface MessageBubbleProps {
  message: Message
  /** 是否为流式输出的最后一条。 */
  isStreaming?: boolean
}

export const MessageBubble: React.FC<MessageBubbleProps> = ({
  message,
  isStreaming = false,
}) => {
  const isUser = message.role === 'user'

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: 'easeOut' }}
      className={`flex ${isUser ? 'justify-end' : 'justify-start'} px-4 py-1.5`}
    >
      <div
        className={`
          min-w-0
          ${isUser
            ? 'max-w-[85%] px-3.5 py-2.5'
            : 'max-w-[92%] px-3.5 py-2.5'
          }
          ${isUser
            ? 'rounded-[12px_4px_12px_12px] bg-mady-bg-bubble-user'
            : 'rounded-[12px_12px_12px_4px] bg-mady-bg-bubble-agent'
          }
          text-mady-body leading-relaxed text-mady-text-primary transition-shadow duration-150
          ${isUser ? '' : isStreaming ? 'border-l-2 border-mady-accent' : 'border-l-2 border-transparent'}
        `}
      >
        {isUser ? (
          <p className="whitespace-pre-wrap">{message.content}</p>
        ) : (
          <MarkdownRenderer content={message.content} />
        )}

        {/* 流式光标（仅 Agent 流式时显示） */}
        {isStreaming && !isUser && (
          <span className="streaming-cursor" />
        )}
      </div>

      {/* 时间戳 */}
      <div className={`self-end mb-1 ${isUser ? 'mr-2' : 'ml-2'}`}>
        <span className="text-mady-text-tertiary text-mady-caption">
          {formatTime(message.timestamp)}
        </span>
      </div>
    </motion.div>
  )
}

function formatTime(ts: number): string {
  const d = new Date(ts)
  const now = new Date()
  const isToday = d.toDateString() === now.toDateString()
  const hh = d.getHours().toString().padStart(2, '0')
  const mm = d.getMinutes().toString().padStart(2, '0')
  if (isToday) return `${hh}:${mm}`
  return `${d.getMonth() + 1}/${d.getDate()} ${hh}:${mm}`
}
