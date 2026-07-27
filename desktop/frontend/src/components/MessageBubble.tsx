/**
 * MessageBubble — 消息气泡组件。
 *
 * 支持用户消息和 Agent 消息两种角色。
 * Agent 消息支持 Markdown 渲染。
 * 使用 Motion 实现淡入动画。
 */

import React from 'react'
import { motion } from 'framer-motion'
import type { Message } from '@/stores/chat'
import { Bot, User } from 'lucide-react'
import { MarkdownRenderer } from './MarkdownRenderer'

interface MessageBubbleProps {
  message: Message
  /** 是否为流式输出的最后一条（"正在思考"指示器）。 */
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
      className={`flex gap-3 px-4 py-3 ${isUser ? 'flex-row-reverse' : 'flex-row'}`}
    >
      {/* 头像 */}
      <div
        className={`
          shrink-0 w-8 h-8 rounded-full flex items-center justify-center
          ${isUser
            ? 'bg-mady-accent-soft text-mady-accent'
            : 'bg-mady-accent text-white'
          }
        `}
      >
        {isUser ? <User size={14} /> : <Bot size={14} />}
      </div>

      {/* 气泡主体 */}
      <div className={`max-w-[75%] min-w-0 ${isUser ? 'items-end' : 'items-start'}`}>
        <div
          className={`
            rounded-2xl px-4 py-2.5 text-mady-body leading-relaxed
            ${isUser
              ? 'bg-mady-accent text-white rounded-tr-md'
              : 'bg-mady-bg-secondary text-mady-text-primary rounded-tl-md border border-mady-separator'
            }
          `}
        >
          {isUser ? (
            <p className="whitespace-pre-wrap">{message.content}</p>
          ) : (
            <MarkdownRenderer content={message.content} />
          )}
        </div>

        {/* 时间戳 */}
        <div className="flex items-center gap-2 mt-1 px-1">
          <span className="text-mady-text-tertiary text-mady-caption">
            {formatTime(message.timestamp)}
          </span>
          {isStreaming && (
            <span className="flex gap-0.5">
              <span className="w-1 h-1 rounded-full bg-mady-accent animate-bounce" style={{ animationDelay: '0ms' }} />
              <span className="w-1 h-1 rounded-full bg-mady-accent animate-bounce" style={{ animationDelay: '150ms' }} />
              <span className="w-1 h-1 rounded-full bg-mady-accent animate-bounce" style={{ animationDelay: '300ms' }} />
            </span>
          )}
        </div>
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
