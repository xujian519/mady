/**
 * Composer — 聊天输入组件。
 *
 * 功能：
 * - 多行文本输入（Enter 发送，Shift+Enter 换行）
 * - 发送按钮
 * - 空输入禁用
 * - 运行时禁用
 * - 自动聚焦 / 失焦
 */

import React, { useState, useRef, useEffect } from 'react'
import { useChatStore } from '@/stores/chat'
import { Send, Sparkles } from 'lucide-react'

export const Composer: React.FC = () => {
  const [text, setText] = useState('')
  const running = useChatStore((s) => s.running)
  const sendMessage = useChatStore((s) => s.sendMessage)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // 自动聚焦
  useEffect(() => {
    if (!running && inputRef.current) {
      inputRef.current.focus()
    }
  }, [running])

  const handleSend = () => {
    const trimmed = text.trim()
    if (!trimmed || running) return
    setText('')
    sendMessage(trimmed)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  // 自动调整高度
  const handleInput = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setText(e.target.value)
    const el = e.target
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`
  }

  const canSend = text.trim().length > 0 && !running

  return (
    <div className="border-t border-mady-separator bg-mady-bg-primary px-4 py-3">
      <div className="max-w-3xl mx-auto flex items-end gap-2">
        <div className="flex-1 relative">
          <textarea
            ref={inputRef}
            value={text}
            onChange={handleInput}
            onKeyDown={handleKeyDown}
            placeholder={running ? '等待回复…' : '输入消息…（Enter 发送，Shift+Enter 换行）'}
            disabled={running}
            rows={1}
            className={`
              w-full resize-none rounded-xl px-4 py-2.5 pr-10
              bg-mady-bg-secondary border border-mady-border
              text-mady-body text-mady-text-primary
              placeholder-mady-text-tertiary
              outline-none
              transition-colors duration-150
              focus:border-mady-accent focus:ring-1 focus:ring-mady-accent/30
              disabled:opacity-50
            `}
          />
        </div>

        <button
          onClick={handleSend}
          disabled={!canSend}
          className={`
            shrink-0 w-9 h-9 rounded-xl flex items-center justify-center
            transition-all duration-150
            ${canSend
              ? 'bg-mady-accent text-white hover:bg-mady-accent-hover shadow-sm'
              : 'bg-mady-bg-secondary text-mady-text-tertiary border border-mady-border'
            }
            disabled:cursor-not-allowed
          `}
          title="发送"
        >
          {running ? (
            <Sparkles size={15} className="animate-pulse" />
          ) : (
            <Send size={15} />
          )}
        </button>
      </div>
    </div>
  )
}
